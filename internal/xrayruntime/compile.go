package xrayruntime

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"tapx/internal/config"
	"tapx/internal/model"
)

type EndpointRef struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ProfileID string `json:"profileId"`
	Runtime   string `json:"runtime"`
	Direction string `json:"direction"`
}

func FrameOutboundTag(endpointID string) string {
	return "tapx-frame-" + endpointID
}

type CompiledConfig struct {
	Document          map[string]any
	EmbeddedDocument  map[string]any
	ExternalEndpoints []EndpointRef
	EmbeddedEndpoints []EndpointRef
}

func CountEndpoints(runtime *config.GeneratedRuntime) int {
	if runtime == nil {
		return 0
	}
	count := 0
	for _, item := range runtime.Listeners {
		if item.Transport == model.TransportXray {
			count++
		}
	}
	for _, item := range runtime.Connectors {
		if item.Transport == model.TransportXray {
			count++
		}
	}
	return count
}

func Compile(runtime *config.GeneratedRuntime) (CompiledConfig, error) {
	if runtime == nil {
		return CompiledConfig{}, fmt.Errorf("xray: runtime is nil")
	}
	profiles := make(map[string]config.RuntimeXrayProfile, len(runtime.XrayProfiles))
	for _, item := range runtime.XrayProfiles {
		profiles[item.ID] = item
	}

	compiled := CompiledConfig{
		Document:         baseDocument(runtime.Settings),
		EmbeddedDocument: baseDocument(runtime.Settings),
	}
	appliedExternalProfiles := map[string]bool{}
	appliedEmbeddedProfiles := map[string]bool{}

	for _, item := range runtime.Listeners {
		if item.Transport != model.TransportXray {
			continue
		}
		profile, ok := profiles[item.XrayProfileID]
		if !ok {
			return CompiledConfig{}, fmt.Errorf("xray: listener %s references missing profile %s", item.ID, item.XrayProfileID)
		}
		ref := EndpointRef{
			ID:        item.ID,
			Kind:      "listener",
			ProfileID: profile.ID,
			Runtime:   string(profile.Runtime),
			Direction: "inbound",
		}
		if profile.Runtime == model.XrayEmbedded {
			if err := mergeProfileTopLevel(compiled.EmbeddedDocument, profile, appliedEmbeddedProfiles); err != nil {
				return CompiledConfig{}, err
			}
			inbound, err := compileInbound(item, profile)
			if err != nil {
				return CompiledConfig{}, err
			}
			appendDocumentArray(compiled.EmbeddedDocument, "inbounds", inbound)
			if item.Binding.DeviceID != "" {
				prependRoutingRule(compiled.EmbeddedDocument, map[string]any{
					"type":        "field",
					"inboundTag":  []any{item.ID},
					"outboundTag": FrameOutboundTag(item.ID),
				})
			}
			compiled.EmbeddedEndpoints = append(compiled.EmbeddedEndpoints, ref)
			continue
		}
		compiled.ExternalEndpoints = append(compiled.ExternalEndpoints, ref)
		if err := mergeProfileTopLevel(compiled.Document, profile, appliedExternalProfiles); err != nil {
			return CompiledConfig{}, err
		}
		inbound, err := compileInbound(item, profile)
		if err != nil {
			return CompiledConfig{}, err
		}
		appendDocumentArray(compiled.Document, "inbounds", inbound)
	}

	for _, item := range runtime.Connectors {
		if item.Transport != model.TransportXray {
			continue
		}
		profile, ok := profiles[item.XrayProfileID]
		if !ok {
			return CompiledConfig{}, fmt.Errorf("xray: connector %s references missing profile %s", item.ID, item.XrayProfileID)
		}
		ref := EndpointRef{
			ID:        item.ID,
			Kind:      "connector",
			ProfileID: profile.ID,
			Runtime:   string(profile.Runtime),
			Direction: "outbound",
		}
		if profile.Runtime == model.XrayEmbedded {
			if err := mergeProfileTopLevel(compiled.EmbeddedDocument, profile, appliedEmbeddedProfiles); err != nil {
				return CompiledConfig{}, err
			}
			outbound, err := compileOutbound(item, profile)
			if err != nil {
				return CompiledConfig{}, err
			}
			appendDocumentArray(compiled.EmbeddedDocument, "outbounds", outbound)
			compiled.EmbeddedEndpoints = append(compiled.EmbeddedEndpoints, ref)
			continue
		}
		compiled.ExternalEndpoints = append(compiled.ExternalEndpoints, ref)
		if err := mergeProfileTopLevel(compiled.Document, profile, appliedExternalProfiles); err != nil {
			return CompiledConfig{}, err
		}
		outbound, err := compileOutbound(item, profile)
		if err != nil {
			return CompiledConfig{}, err
		}
		appendDocumentArray(compiled.Document, "outbounds", outbound)
	}

	sortEndpointRefs(compiled.ExternalEndpoints)
	sortEndpointRefs(compiled.EmbeddedEndpoints)
	return compiled, nil
}

func baseDocument(settings []config.RuntimeSettings) map[string]any {
	doc := map[string]any{}
	if level := xrayLogLevel(settings); level != "" {
		doc["log"] = map[string]any{"loglevel": level}
	}
	return doc
}

func xrayLogLevel(settings []config.RuntimeSettings) string {
	for _, item := range settings {
		switch item.LogLevel {
		case "debug", "info", "error":
			return item.LogLevel
		case "warn":
			return "warning"
		}
	}
	return ""
}

func mergeProfileTopLevel(doc map[string]any, profile config.RuntimeXrayProfile, applied map[string]bool) error {
	if applied[profile.ID] {
		return nil
	}
	applied[profile.ID] = true
	if profile.AdvancedJSON != "" {
		advanced, err := decodeObject("XrayProfile", profile.ID, "AdvancedJSON", profile.AdvancedJSON)
		if err != nil {
			return err
		}
		mergeMap(doc, advanced)
	}
	for _, field := range []struct {
		name  string
		value string
		key   string
	}{
		{name: "RoutingJSON", value: profile.RoutingJSON, key: "routing"},
		{name: "DNSJSON", value: profile.DNSJSON, key: "dns"},
		{name: "PolicyJSON", value: profile.PolicyJSON, key: "policy"},
	} {
		if field.value == "" {
			continue
		}
		value, err := decodeAny("XrayProfile", profile.ID, field.name, field.value)
		if err != nil {
			return err
		}
		doc[field.key] = value
	}
	return nil
}

func compileInbound(endpoint config.RuntimeEndpoint, profile config.RuntimeXrayProfile) (map[string]any, error) {
	protocol := strings.TrimSpace(profile.InboundProtocol)
	if protocol == "" {
		return nil, fmt.Errorf("xray: listener %s profile %s has empty inbound protocol", endpoint.ID, profile.ID)
	}
	if endpoint.BindPort == 0 {
		return nil, fmt.Errorf("xray: listener %s requires bind port for external runtime", endpoint.ID)
	}
	settings, err := decodeOptionalObject("XrayProfile", profile.ID, "InboundSettingsJSON", profile.InboundSettingsJSON)
	if err != nil {
		return nil, err
	}
	if profile.FallbacksJSON != "" {
		fallbacks, err := decodeAny("XrayProfile", profile.ID, "FallbacksJSON", profile.FallbacksJSON)
		if err != nil {
			return nil, err
		}
		settings["fallbacks"] = fallbacks
	}
	out := map[string]any{
		"tag":      endpoint.ID,
		"port":     int(endpoint.BindPort),
		"protocol": protocol,
		"settings": settings,
	}
	if endpoint.BindHost != "" {
		out["listen"] = endpoint.BindHost
	}
	if stream, err := compileStream(profile); err != nil {
		return nil, err
	} else if len(stream) > 0 {
		out["streamSettings"] = stream
	}
	if sniffing, err := decodeOptionalObject("XrayProfile", profile.ID, "SniffingJSON", profile.SniffingJSON); err != nil {
		return nil, err
	} else if len(sniffing) > 0 {
		out["sniffing"] = sniffing
	}
	return out, nil
}

func compileOutbound(endpoint config.RuntimeEndpoint, profile config.RuntimeXrayProfile) (map[string]any, error) {
	protocol := strings.TrimSpace(profile.OutboundProtocol)
	if protocol == "" {
		return nil, fmt.Errorf("xray: connector %s profile %s has empty outbound protocol", endpoint.ID, profile.ID)
	}
	settings, err := decodeOptionalObject("XrayProfile", profile.ID, "OutboundSettingsJSON", profile.OutboundSettingsJSON)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"tag":      endpoint.ID,
		"protocol": protocol,
	}
	if len(settings) > 0 {
		out["settings"] = settings
	}
	if stream, err := compileStream(profile); err != nil {
		return nil, err
	} else if len(stream) > 0 {
		out["streamSettings"] = stream
	}
	if mux, err := decodeOptionalObject("XrayProfile", profile.ID, "MuxJSON", profile.MuxJSON); err != nil {
		return nil, err
	} else if len(mux) > 0 {
		out["mux"] = mux
	}
	return out, nil
}

func compileStream(profile config.RuntimeXrayProfile) (map[string]any, error) {
	stream, err := decodeOptionalObject("XrayProfile", profile.ID, "StreamSettingsJSON", profile.StreamSettingsJSON)
	if err != nil {
		return nil, err
	}
	if profile.Network != "" {
		stream["network"] = profile.Network
	}
	if profile.Security != "" {
		stream["security"] = profile.Security
	}
	if profile.SockoptJSON != "" {
		sockopt, err := decodeObject("XrayProfile", profile.ID, "SockoptJSON", profile.SockoptJSON)
		if err != nil {
			return nil, err
		}
		stream["sockopt"] = sockopt
	}
	return stream, nil
}

func appendDocumentArray(doc map[string]any, key string, value map[string]any) {
	existing, _ := doc[key].([]any)
	doc[key] = append(existing, value)
}

func decodeOptionalObject(object, id, field, value string) (map[string]any, error) {
	if strings.TrimSpace(value) == "" {
		return map[string]any{}, nil
	}
	return decodeObject(object, id, field, value)
}

func decodeObject(object, id, field, value string) (map[string]any, error) {
	decoded, err := decodeAny(object, id, field, value)
	if err != nil {
		return nil, err
	}
	out, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s[%s].%s must be a JSON object", object, id, field)
	}
	return out, nil
}

func decodeAny(object, id, field, value string) (any, error) {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil, fmt.Errorf("%s[%s].%s: %w", object, id, field, err)
	}
	return decoded, nil
}

func mergeMap(dst, src map[string]any) {
	for key, value := range src {
		srcMap, srcOK := value.(map[string]any)
		dstMap, dstOK := dst[key].(map[string]any)
		if srcOK && dstOK {
			mergeMap(dstMap, srcMap)
			continue
		}
		dst[key] = value
	}
}

func prependRoutingRule(doc map[string]any, rule map[string]any) {
	routing, _ := doc["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
		doc["routing"] = routing
	}
	ruleCopy := maps.Clone(rule)
	existing, _ := routing["rules"].([]any)
	rules := make([]any, 0, len(existing)+1)
	rules = append(rules, ruleCopy)
	rules = append(rules, existing...)
	routing["rules"] = rules
}

func sortEndpointRefs(refs []EndpointRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].ID < refs[j].ID
	})
}
