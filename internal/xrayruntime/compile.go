package xrayruntime

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/netip"
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

func FrameInboundTag(endpointID string) string {
	return "tapx-frame-in-" + endpointID
}

const frameDropOutboundTag = "tapx-frame-drop"

type CompiledConfig struct {
	Document          map[string]any
	EmbeddedDocument  map[string]any
	ExternalEndpoints []EndpointRef
	EmbeddedEndpoints []EndpointRef
}

type linkPolicy struct {
	automatic bool
	deviceMTU int
	outer     string
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
			inbound, err := compileInbound(item, profile, runtime.Clients, endpointLinkPolicy(runtime, item))
			if err != nil {
				return CompiledConfig{}, err
			}
			appendDocumentArray(compiled.EmbeddedDocument, "inbounds", inbound)
			if err := compileEmbeddedFrameRoutes(compiled.EmbeddedDocument, runtime, item); err != nil {
				return CompiledConfig{}, err
			}
			compiled.EmbeddedEndpoints = append(compiled.EmbeddedEndpoints, ref)
			continue
		}
		compiled.ExternalEndpoints = append(compiled.ExternalEndpoints, ref)
		if err := mergeProfileTopLevel(compiled.Document, profile, appliedExternalProfiles); err != nil {
			return CompiledConfig{}, err
		}
		inbound, err := compileInbound(item, profile, runtime.Clients, endpointLinkPolicy(runtime, item))
		if err != nil {
			return CompiledConfig{}, err
		}
		appendDocumentArray(compiled.Document, "inbounds", inbound)
		if err := compileExternalFrameRoutes(compiled.Document, runtime, item); err != nil {
			return CompiledConfig{}, err
		}
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
			outbound, err := compileOutbound(item, profile, endpointLinkPolicy(runtime, item))
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
		outbound, err := compileOutbound(item, profile, endpointLinkPolicy(runtime, item))
		if err != nil {
			return CompiledConfig{}, err
		}
		appendDocumentArray(compiled.Document, "outbounds", outbound)
		if item.Binding.DeviceID != "" {
			if item.ExternalBridgePort == 0 {
				return CompiledConfig{}, fmt.Errorf("xray: external connector %s requires an allocated local frame bridge port", item.ID)
			}
			appendDocumentArray(compiled.Document, "inbounds", externalFrameInbound(item.ID, item.ExternalBridgePort))
			prependRoutingRule(compiled.Document, map[string]any{
				"type":        "field",
				"inboundTag":  []any{FrameInboundTag(item.ID)},
				"outboundTag": item.ID,
			})
		}
	}

	sortEndpointRefs(compiled.ExternalEndpoints)
	sortEndpointRefs(compiled.EmbeddedEndpoints)
	return compiled, nil
}

func compileEmbeddedFrameRoutes(doc map[string]any, runtime *config.GeneratedRuntime, endpoint config.RuntimeEndpoint) error {
	policies := make([]config.RuntimeXrayPipe, 0)
	for _, pipe := range runtime.XrayPipes {
		if pipe.EndpointKind == "listener" && pipe.EndpointID == endpoint.ID {
			policies = append(policies, pipe)
		}
	}
	if len(policies) == 0 {
		if endpoint.Binding.DeviceID != "" {
			prependRoutingRule(doc, map[string]any{
				"type":        "field",
				"inboundTag":  []any{endpoint.ID},
				"outboundTag": FrameOutboundTag(endpoint.ID),
			})
		}
		return nil
	}
	sort.SliceStable(policies, func(i, j int) bool { return policies[i].Priority < policies[j].Priority })
	rules := make([]map[string]any, 0, len(policies))
	needsDrop := false
	for _, policy := range policies {
		outboundTag := policy.HandlerTag
		if policy.Action == model.RouteActionDrop {
			outboundTag = frameDropOutboundTag
			needsDrop = true
		} else if policy.DeviceID == "" {
			return fmt.Errorf("xray: listener %s route %s has no target device", endpoint.ID, policy.RouteID)
		}
		rule := map[string]any{
			"type":        "field",
			"inboundTag":  []any{endpoint.ID},
			"outboundTag": outboundTag,
		}
		if policy.ClientEmail != "" {
			rule["user"] = []any{policy.ClientEmail}
		}
		rules = append(rules, rule)
	}
	if needsDrop {
		ensureDocumentOutbound(doc, map[string]any{
			"tag": frameDropOutboundTag, "protocol": "blackhole", "settings": map[string]any{},
		})
	}
	prependRoutingRules(doc, rules)
	return nil
}

func externalFrameOutbound(endpointID string, port uint16) map[string]any {
	return externalFrameOutboundWithTag(FrameOutboundTag(endpointID), port)
}

func externalFrameOutboundWithTag(tag string, port uint16) map[string]any {
	return map[string]any{
		"tag":      tag,
		"protocol": "freedom",
		"settings": map[string]any{
			"redirect": netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port).String(),
		},
	}
}

func compileExternalFrameRoutes(doc map[string]any, runtime *config.GeneratedRuntime, endpoint config.RuntimeEndpoint) error {
	policies := make([]config.RuntimeXrayPipe, 0)
	for _, pipe := range runtime.XrayPipes {
		if pipe.EndpointKind == "listener" && pipe.EndpointID == endpoint.ID && pipe.Runtime == model.XrayExternal {
			policies = append(policies, pipe)
		}
	}
	if len(policies) == 0 {
		if endpoint.Binding.DeviceID == "" {
			return nil
		}
		if endpoint.ExternalBridgePort == 0 {
			return fmt.Errorf("xray: external listener %s requires an allocated local frame bridge port", endpoint.ID)
		}
		appendDocumentArray(doc, "outbounds", externalFrameOutbound(endpoint.ID, endpoint.ExternalBridgePort))
		prependRoutingRule(doc, map[string]any{
			"type": "field", "inboundTag": []any{endpoint.ID}, "outboundTag": FrameOutboundTag(endpoint.ID),
		})
		return nil
	}
	sort.SliceStable(policies, func(i, j int) bool { return policies[i].Priority < policies[j].Priority })
	rules := make([]map[string]any, 0, len(policies))
	needsDrop := false
	for _, policy := range policies {
		outboundTag := policy.HandlerTag
		if policy.Action == model.RouteActionDrop {
			outboundTag = frameDropOutboundTag
			needsDrop = true
		} else {
			port := externalListenerBridgePort(runtime.TCPPipes, endpoint.ID, policy.HandlerTag)
			if port == 0 {
				return fmt.Errorf("xray: external listener %s route %s requires an allocated local frame bridge port", endpoint.ID, policy.RouteID)
			}
			ensureDocumentOutbound(doc, externalFrameOutboundWithTag(policy.HandlerTag, port))
		}
		rule := map[string]any{
			"type": "field", "inboundTag": []any{endpoint.ID}, "outboundTag": outboundTag,
		}
		if policy.ClientEmail != "" {
			rule["user"] = []any{policy.ClientEmail}
		}
		rules = append(rules, rule)
	}
	if needsDrop {
		ensureDocumentOutbound(doc, map[string]any{
			"tag": frameDropOutboundTag, "protocol": "blackhole", "settings": map[string]any{},
		})
	}
	prependRoutingRules(doc, rules)
	return nil
}

func externalListenerBridgePort(pipes []config.RuntimeTCPPipe, endpointID, tag string) uint16 {
	for _, pipe := range pipes {
		if pipe.ExternalXrayBridge && pipe.EndpointKind == "listener" && pipe.EndpointID == endpointID && pipe.XrayBridgeTag == tag {
			return pipe.BindPort
		}
	}
	return 0
}

func externalFrameInbound(endpointID string, port uint16) map[string]any {
	return map[string]any{
		"tag":      FrameInboundTag(endpointID),
		"listen":   "127.0.0.1",
		"port":     int(port),
		"protocol": "dokodemo-door",
		"settings": map[string]any{
			"address":        "tapx.frame.local",
			"port":           1,
			"network":        "tcp",
			"followRedirect": false,
		},
	}
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

func compileInbound(endpoint config.RuntimeEndpoint, profile config.RuntimeXrayProfile, clients []model.Client, policy linkPolicy) (map[string]any, error) {
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
	if err := injectInboundClients(settings, protocol, endpoint.ID, clients); err != nil {
		return nil, err
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
	if stream, err := compileStream(profile, policy); err != nil {
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

func injectInboundClients(settings map[string]any, protocol, listenerID string, clients []model.Client) error {
	selected := make([]model.Client, 0)
	for _, client := range clients {
		if client.ListenerID == listenerID || containsString(client.ListenerIDs, listenerID) {
			selected = append(selected, client)
		}
	}
	if len(selected) == 0 {
		return nil
	}

	generated := make([]any, 0, len(selected))
	for _, client := range selected {
		switch protocol {
		case "vless", "vmess":
			id := firstClientValue(client.UUID, credentialFor(client, "vless", "vmess", "uuid"))
			if id == "" {
				return fmt.Errorf("xray: listener %s client %s requires UUID for %s", listenerID, client.ID, protocol)
			}
			entry := map[string]any{"id": id, "email": firstClientValue(client.Email, client.ID)}
			generated = append(generated, entry)
		case "trojan", "shadowsocks":
			password := firstClientValue(client.Password, credentialFor(client, "trojan", "shadowsocks", "password"))
			if password == "" {
				return fmt.Errorf("xray: listener %s client %s requires password for %s", listenerID, client.ID, protocol)
			}
			generated = append(generated, map[string]any{"password": password, "email": firstClientValue(client.Email, client.ID)})
		case "hysteria":
			auth := firstClientValue(client.Auth, credentialFor(client, "hysteria"))
			if auth == "" {
				return fmt.Errorf("xray: listener %s client %s requires auth for hysteria", listenerID, client.ID)
			}
			generated = append(generated, map[string]any{"auth": auth, "email": firstClientValue(client.Email, client.ID)})
		default:
			return nil
		}
	}

	settings["clients"] = mergeClientEntries(settings["clients"], generated, clientIdentityKey(protocol))
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func credentialFor(client model.Client, types ...string) string {
	for _, value := range types {
		if client.CredentialType == value {
			return client.CredentialValue
		}
	}
	return ""
}

func firstClientValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func clientIdentityKey(protocol string) string {
	if protocol == "trojan" || protocol == "shadowsocks" {
		return "password"
	}
	if protocol == "hysteria" {
		return "auth"
	}
	return "id"
}

func mergeClientEntries(existingValue any, generated []any, identityKey string) []any {
	out := make([]any, 0)
	seen := map[string]bool{}
	if existing, ok := existingValue.([]any); ok {
		for _, item := range existing {
			entry, _ := item.(map[string]any)
			identity, _ := entry[identityKey].(string)
			if identity != "" {
				seen[identity] = true
			}
			out = append(out, item)
		}
	}
	for _, item := range generated {
		entry, _ := item.(map[string]any)
		identity, _ := entry[identityKey].(string)
		if identity != "" && seen[identity] {
			continue
		}
		if identity != "" {
			seen[identity] = true
		}
		out = append(out, item)
	}
	return out
}

func compileOutbound(endpoint config.RuntimeEndpoint, profile config.RuntimeXrayProfile, policy linkPolicy) (map[string]any, error) {
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
	if profile.SendThrough != "" {
		out["sendThrough"] = profile.SendThrough
	}
	if profile.TargetStrategy != "" {
		out["targetStrategy"] = profile.TargetStrategy
	}
	if len(settings) > 0 {
		out["settings"] = settings
	}
	if stream, err := compileStream(profile, policy); err != nil {
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

func compileStream(profile config.RuntimeXrayProfile, policy linkPolicy) (map[string]any, error) {
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
	if policy.automatic {
		if err := applyAutomaticLinkPolicy(stream, profile, policy); err != nil {
			return nil, err
		}
	}
	return stream, nil
}

func endpointLinkPolicy(runtime *config.GeneratedRuntime, endpoint config.RuntimeEndpoint) linkPolicy {
	if runtime == nil || endpoint.Binding.DeviceID == "" {
		return linkPolicy{}
	}
	for _, device := range runtime.Devices {
		if device.ID == endpoint.Binding.DeviceID {
			outer := endpoint.Remote
			if outer == "" {
				outer = endpoint.BindHost
			}
			return linkPolicy{automatic: device.LinkAutoOptimize, deviceMTU: device.MTU, outer: outer}
		}
	}
	return linkPolicy{}
}

func applyAutomaticLinkPolicy(stream map[string]any, profile config.RuntimeXrayProfile, policy linkPolicy) error {
	if policy.deviceMTU <= 0 {
		return fmt.Errorf("xray: profile %s automatic link optimization requires a device MTU", profile.ID)
	}
	network := strings.ToLower(strings.TrimSpace(profile.Network))
	if network == "" {
		network, _ = stream["network"].(string)
		network = strings.ToLower(strings.TrimSpace(network))
	}
	switch network {
	case "kcp", "mkcp":
		settings, err := nestedObject(stream, "kcpSettings", profile.ID)
		if err != nil {
			return err
		}
		// Xray's mKCP MTU is the UDP payload size. 1232 is the largest
		// payload guaranteed not to fragment on an IPv6 minimum-MTU path:
		// 1280 - 40 byte IPv6 header - 8 byte UDP header.
		mtu := min(1232, policy.deviceMTU)
		if mtu < 576 {
			return fmt.Errorf("xray: profile %s device MTU %d is too small for automatic mKCP framing", profile.ID, policy.deviceMTU)
		}
		if explicit, exists := settings["mtu"]; exists {
			value, ok := positiveJSONInteger(explicit)
			if !ok {
				return fmt.Errorf("xray: profile %s kcpSettings.mtu must be a positive integer", profile.ID)
			}
			if value <= mtu {
				stream["kcpSettings"] = settings
				return nil
			}
		}
		settings["mtu"] = mtu
		stream["kcpSettings"] = settings
	case "hysteria":
		return enableQUICPathMTUDiscovery(stream, profile.ID)
	case "xhttp", "splithttp":
		return enableQUICPathMTUDiscovery(stream, profile.ID)
	default:
		// TCP, WebSocket, gRPC and HTTP transports are byte streams. Their
		// outer sockets already use kernel PMTUD and segmentation offload, so
		// deriving TCP_MAXSEG from the inner TUN/TAP MTU only creates smaller
		// outer segments and extra CPU work. Explicit sockopt values remain
		// untouched for advanced configurations.
		return nil
	}
	return nil
}

func positiveJSONInteger(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, number > 0
	case int32:
		return int(number), number > 0
	case int64:
		if number <= 0 || uint64(number) > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(number), true
	case uint:
		if number == 0 || uint64(number) > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(number), true
	case uint32:
		if number == 0 || uint64(number) > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(number), true
	case uint64:
		if number == 0 || number > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(number), true
	case float64:
		if number <= 0 || number > float64(^uint(0)>>1) {
			return 0, false
		}
		converted := int(number)
		return converted, float64(converted) == number
	default:
		return 0, false
	}
}

func enableQUICPathMTUDiscovery(stream map[string]any, profileID string) error {
	finalMask, err := nestedObject(stream, "finalmask", profileID)
	if err != nil {
		return err
	}
	quicParams, err := nestedObject(finalMask, "quicParams", profileID)
	if err != nil {
		return err
	}
	if disabled, explicit := quicParams["disablePathMTUDiscovery"].(bool); explicit && disabled {
		return fmt.Errorf("xray: profile %s disables QUIC path MTU discovery while automatic link optimization is enabled", profileID)
	}
	quicParams["disablePathMTUDiscovery"] = false
	finalMask["quicParams"] = quicParams
	stream["finalmask"] = finalMask
	return nil
}

func nestedObject(parent map[string]any, key, profileID string) (map[string]any, error) {
	value, exists := parent[key]
	if !exists || value == nil {
		return map[string]any{}, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("xray: profile %s %s must be a JSON object", profileID, key)
	}
	return object, nil
}

func appendDocumentArray(doc map[string]any, key string, value map[string]any) {
	existing, _ := doc[key].([]any)
	doc[key] = append(existing, value)
}

func ensureDocumentOutbound(doc map[string]any, outbound map[string]any) {
	tag, _ := outbound["tag"].(string)
	existing, _ := doc["outbounds"].([]any)
	for _, item := range existing {
		object, _ := item.(map[string]any)
		if objectTag, _ := object["tag"].(string); objectTag == tag {
			return
		}
	}
	doc["outbounds"] = append(existing, outbound)
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
	prependRoutingRules(doc, []map[string]any{rule})
}

func prependRoutingRules(doc map[string]any, additions []map[string]any) {
	if len(additions) == 0 {
		return
	}
	routing, _ := doc["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
		doc["routing"] = routing
	}
	existing, _ := routing["rules"].([]any)
	rules := make([]any, 0, len(existing)+len(additions))
	for _, rule := range additions {
		rules = append(rules, maps.Clone(rule))
	}
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
