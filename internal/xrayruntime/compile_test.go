package xrayruntime

import (
	"encoding/json"
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestCompileExternalXrayConfig(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		Settings: []config.RuntimeSettings{{
			LogLevel: "warn",
		}},
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                   "xr1",
			Runtime:              model.XrayExternal,
			InboundProtocol:      "vless",
			InboundSettingsJSON:  `{"clients":[{"id":"00000000-0000-0000-0000-000000000001"}]}`,
			OutboundProtocol:     "freedom",
			OutboundSettingsJSON: `{"domainStrategy":"UseIP"}`,
			Network:              "tcp",
			Security:             "none",
			StreamSettingsJSON:   `{"tcpSettings":{"acceptProxyProtocol":false}}`,
			SniffingJSON:         `{"enabled":true,"destOverride":["http","tls"]}`,
			MuxJSON:              `{"enabled":false}`,
			SockoptJSON:          `{"tcpFastOpen":true}`,
			RoutingJSON:          `{"rules":[{"type":"field","inboundTag":["listener-xr"],"outboundTag":"connector-xr"}]}`,
			AdvancedJSON:         `{"stats":{},"outbounds":[{"tag":"direct","protocol":"freedom"}]}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-xr",
			Transport:     model.TransportXray,
			BindHost:      "127.0.0.1",
			BindPort:      1080,
			XrayProfileID: "xr1",
		}},
		Connectors: []config.RuntimeEndpoint{{
			ID:            "connector-xr",
			Transport:     model.TransportXray,
			XrayProfileID: "xr1",
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compiled.EmbeddedEndpoints) != 0 || len(compiled.ExternalEndpoints) != 2 {
		t.Fatalf("compiled endpoints = external:%+v embedded:%+v, want two external", compiled.ExternalEndpoints, compiled.EmbeddedEndpoints)
	}
	log := compiled.Document["log"].(map[string]any)
	if log["loglevel"] != "warning" {
		t.Fatalf("loglevel = %v, want warning", log["loglevel"])
	}
	inbounds := compiled.Document["inbounds"].([]any)
	if len(inbounds) != 1 {
		t.Fatalf("inbounds = %d, want 1", len(inbounds))
	}
	inbound := inbounds[0].(map[string]any)
	if inbound["tag"] != "listener-xr" || inbound["listen"] != "127.0.0.1" || inbound["protocol"] != "vless" {
		t.Fatalf("inbound = %+v, want generated listener", inbound)
	}
	stream := inbound["streamSettings"].(map[string]any)
	if stream["network"] != "tcp" || stream["security"] != "none" {
		t.Fatalf("stream = %+v, want tcp/none", stream)
	}
	if _, ok := stream["sockopt"].(map[string]any); !ok {
		t.Fatalf("stream sockopt missing: %+v", stream)
	}
	outbounds := compiled.Document["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("outbounds = %d, want one advanced plus one generated", len(outbounds))
	}
	generatedOutbound := outbounds[1].(map[string]any)
	if generatedOutbound["tag"] != "connector-xr" || generatedOutbound["protocol"] != "freedom" {
		t.Fatalf("generated outbound = %+v, want connector-xr freedom", generatedOutbound)
	}
	if _, err := json.Marshal(compiled.Document); err != nil {
		t.Fatalf("compiled document is not JSON serializable: %v", err)
	}
}

func TestCompileReportsEmbeddedEndpoints(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr1",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			AdvancedJSON:        `{"outbounds":[{"tag":"direct","protocol":"freedom"}]}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-xr",
			Transport:     model.TransportXray,
			BindHost:      "127.0.0.1",
			BindPort:      18080,
			XrayProfileID: "xr1",
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compiled.EmbeddedEndpoints) != 1 || compiled.EmbeddedEndpoints[0].ID != "listener-xr" {
		t.Fatalf("embedded endpoints = %+v, want listener-xr", compiled.EmbeddedEndpoints)
	}
	inbounds := compiled.EmbeddedDocument["inbounds"].([]any)
	if len(inbounds) != 1 {
		t.Fatalf("embedded inbounds = %d, want 1", len(inbounds))
	}
	inbound := inbounds[0].(map[string]any)
	if inbound["tag"] != "listener-xr" || inbound["protocol"] != "dokodemo-door" {
		t.Fatalf("embedded inbound = %+v, want listener-xr dokodemo-door", inbound)
	}
}

func TestCompileEmbeddedListenerRoutesBoundDeviceToFrameHandler(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr1",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			AdvancedJSON:        `{"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"rules":[{"type":"field","outboundTag":"direct"}]}}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-xr",
			Transport:     model.TransportXray,
			BindHost:      "127.0.0.1",
			BindPort:      18080,
			XrayProfileID: "xr1",
			Binding:       config.RuntimeBinding{DeviceID: "tun0"},
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	routing := compiled.EmbeddedDocument["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want frame rule plus existing rule", rules)
	}
	first := rules[0].(map[string]any)
	if first["outboundTag"] != FrameOutboundTag("listener-xr") {
		t.Fatalf("first routing rule = %+v, want frame outbound tag", first)
	}
	inboundTags := first["inboundTag"].([]any)
	if len(inboundTags) != 1 || inboundTags[0] != "listener-xr" {
		t.Fatalf("first inboundTag = %+v, want listener-xr", inboundTags)
	}
}

func TestCompileValidatesEmbeddedProfileJSONShapes(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr1",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			SockoptJSON:         `[]`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-xr",
			Transport:     model.TransportXray,
			BindPort:      18080,
			XrayProfileID: "xr1",
		}},
	}

	if _, err := Compile(runtime); err == nil {
		t.Fatal("Compile() error = nil, want embedded profile JSON shape error")
	}
}
