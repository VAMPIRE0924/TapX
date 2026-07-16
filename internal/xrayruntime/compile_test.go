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
			SendThrough:          "192.0.2.10",
			TargetStrategy:       "ForceIPv4",
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
	if generatedOutbound["sendThrough"] != "192.0.2.10" || generatedOutbound["targetStrategy"] != "ForceIPv4" {
		t.Fatalf("generated outbound = %+v, want sendThrough and targetStrategy", generatedOutbound)
	}
	if _, err := json.Marshal(compiled.Document); err != nil {
		t.Fatalf("compiled document is not JSON serializable: %v", err)
	}
}

func TestCompileExternalXrayDeviceBridges(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID: "xr1", Runtime: model.XrayExternal,
			InboundProtocol: "vless", InboundSettingsJSON: `{"clients":[{"id":"00000000-0000-0000-0000-000000000001"}]}`,
			OutboundProtocol: "vless", OutboundSettingsJSON: `{"vnext":[{"address":"192.0.2.10","port":443,"users":[{"id":"00000000-0000-0000-0000-000000000001"}]}]}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID: "listener-xr", Transport: model.TransportXray, BindPort: 443, XrayProfileID: "xr1",
			ExternalBridgePort: 39002, Binding: config.RuntimeBinding{DeviceID: "tap0"},
		}},
		Connectors: []config.RuntimeEndpoint{{
			ID: "connector-xr", Transport: model.TransportXray, Remote: "192.0.2.10", Port: 443, XrayProfileID: "xr1",
			ExternalBridgePort: 39001, Binding: config.RuntimeBinding{DeviceID: "tap0"},
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	inbounds := compiled.Document["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("inbounds = %d, want public listener plus local connector bridge", len(inbounds))
	}
	bridgeInbound := inbounds[1].(map[string]any)
	if bridgeInbound["tag"] != FrameInboundTag("connector-xr") || bridgeInbound["listen"] != "127.0.0.1" || bridgeInbound["port"] != 39001 {
		t.Fatalf("connector bridge inbound = %+v", bridgeInbound)
	}
	outbounds := compiled.Document["outbounds"].([]any)
	if len(outbounds) != 2 {
		t.Fatalf("outbounds = %d, want local listener bridge plus public connector", len(outbounds))
	}
	bridgeOutbound := outbounds[0].(map[string]any)
	settings := bridgeOutbound["settings"].(map[string]any)
	if bridgeOutbound["tag"] != FrameOutboundTag("listener-xr") || settings["redirect"] != "127.0.0.1:39002" {
		t.Fatalf("listener bridge outbound = %+v", bridgeOutbound)
	}
	routing := compiled.Document["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("bridge routing rules = %+v, want two", rules)
	}
}

func TestCompileExternalXrayRoutesAuthenticatedUsersToDedicatedBridges(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID: "xr1", Runtime: model.XrayExternal, InboundProtocol: "vless", InboundSettingsJSON: `{"decryption":"none"}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID: "listener-xr", Transport: model.TransportXray, BindPort: 443, XrayProfileID: "xr1",
		}},
		Clients: []model.Client{
			{ID: "client-a", Enabled: true, Email: "a@example.test", UUID: "11111111-1111-4111-8111-111111111111", ListenerID: "listener-xr"},
			{ID: "client-b", Enabled: true, Email: "b@example.test", UUID: "22222222-2222-4222-8222-222222222222", ListenerID: "listener-xr"},
		},
		XrayPipes: []config.RuntimeXrayPipe{
			{EndpointID: "listener-xr", EndpointKind: "listener", HandlerTag: "tapx-frame-a", Runtime: model.XrayExternal, Priority: 10, Action: model.RouteActionBindDevice, ClientEmail: "a@example.test", DeviceID: "tun-a"},
			{EndpointID: "listener-xr", EndpointKind: "listener", HandlerTag: "tapx-frame-b", Runtime: model.XrayExternal, Priority: 20, Action: model.RouteActionDrop, ClientEmail: "b@example.test", RouteID: "drop-b"},
		},
		TCPPipes: []config.RuntimeTCPPipe{{
			EndpointID: "listener-xr", EndpointKind: "listener", ExternalXrayBridge: true,
			XrayBridgeTag: "tapx-frame-a", BindPort: 39011, DeviceID: "tun-a",
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	rules := compiled.Document["routing"].(map[string]any)["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("rules = %+v, want routed and dropped users", rules)
	}
	if got := rules[0].(map[string]any)["outboundTag"]; got != "tapx-frame-a" {
		t.Fatalf("first outbound tag = %v", got)
	}
	if got := rules[1].(map[string]any)["outboundTag"]; got != frameDropOutboundTag {
		t.Fatalf("second outbound tag = %v", got)
	}
	outbounds := compiled.Document["outbounds"].([]any)
	var bridge, drop bool
	for _, item := range outbounds {
		outbound := item.(map[string]any)
		switch outbound["tag"] {
		case "tapx-frame-a":
			bridge = outbound["settings"].(map[string]any)["redirect"] == "127.0.0.1:39011"
		case frameDropOutboundTag:
			drop = outbound["protocol"] == "blackhole"
		}
	}
	if !bridge || !drop {
		t.Fatalf("outbounds = %+v, want dedicated bridge and blackhole", outbounds)
	}
}

func TestCompileAutomaticXrayStreamUsesKernelPMTUD(t *testing.T) {
	runtime := automaticXrayRuntime("tcp", "", "")
	runtime.Listeners = []config.RuntimeEndpoint{{
		ID: "listener-xr", Transport: model.TransportXray, BindPort: 443, XrayProfileID: "xr1",
		Binding: config.RuntimeBinding{DeviceID: "tun0"}, ExternalBridgePort: 39002,
	}}
	runtime.Connectors = []config.RuntimeEndpoint{{
		ID: "connector-xr", Transport: model.TransportXray, Remote: "192.0.2.10", XrayProfileID: "xr1",
		Binding: config.RuntimeBinding{DeviceID: "tun0"}, ExternalBridgePort: 39001,
	}}
	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatal(err)
	}
	inboundStream := compiled.Document["inbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	outboundStream := compiled.Document["outbounds"].([]any)[1].(map[string]any)["streamSettings"].(map[string]any)
	if sockopt, ok := inboundStream["sockopt"].(map[string]any); ok {
		if _, exists := sockopt["tcpMaxSeg"]; exists {
			t.Fatalf("listener tcpMaxSeg = %v, want kernel PMTUD", sockopt["tcpMaxSeg"])
		}
	}
	if sockopt, ok := outboundStream["sockopt"].(map[string]any); ok {
		if _, exists := sockopt["tcpMaxSeg"]; exists {
			t.Fatalf("connector tcpMaxSeg = %v, want kernel PMTUD", sockopt["tcpMaxSeg"])
		}
	}
}

func TestCompileAutomaticXrayPreservesExplicitTCPAndKCPValues(t *testing.T) {
	tcpRuntime := automaticXrayRuntime("tcp", `{"tcpMaxSeg":1200}`, "")
	compiled, err := Compile(tcpRuntime)
	if err != nil {
		t.Fatal(err)
	}
	tcpStream := compiled.Document["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if got := tcpStream["sockopt"].(map[string]any)["tcpMaxSeg"]; got != float64(1200) {
		t.Fatalf("explicit tcpMaxSeg = %v, want 1200", got)
	}

	kcpRuntime := automaticXrayRuntime("mkcp", "", `{"kcpSettings":{"mtu":1100}}`)
	compiled, err = Compile(kcpRuntime)
	if err != nil {
		t.Fatal(err)
	}
	kcpStream := compiled.Document["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if got := kcpStream["kcpSettings"].(map[string]any)["mtu"]; got != float64(1100) {
		t.Fatalf("explicit mKCP MTU = %v, want 1100", got)
	}
}

func TestCompileAutomaticXrayPreservesExplicitStreamOverride(t *testing.T) {
	tcpRuntime := automaticXrayRuntime("tcp", `{"tcpMaxSeg":9000}`, "")
	compiled, err := Compile(tcpRuntime)
	if err != nil {
		t.Fatal(err)
	}
	tcpStream := compiled.Document["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if got := tcpStream["sockopt"].(map[string]any)["tcpMaxSeg"]; got != float64(9000) {
		t.Fatalf("explicit tcpMaxSeg = %v, want 9000", got)
	}

	kcpRuntime := automaticXrayRuntime("mkcp", "", `{"kcpSettings":{"mtu":9000}}`)
	compiled, err = Compile(kcpRuntime)
	if err != nil {
		t.Fatal(err)
	}
	kcpStream := compiled.Document["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if got := kcpStream["kcpSettings"].(map[string]any)["mtu"]; got != 1232 {
		t.Fatalf("clamped mKCP MTU = %v, want 1232", got)
	}
}

func TestCompileAutomaticXrayKCPAndQUICPolicies(t *testing.T) {
	kcpRuntime := automaticXrayRuntime("mkcp", "", "")
	compiled, err := Compile(kcpRuntime)
	if err != nil {
		t.Fatal(err)
	}
	kcpStream := compiled.Document["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	if got := kcpStream["kcpSettings"].(map[string]any)["mtu"]; got != 1232 {
		t.Fatalf("automatic mKCP MTU = %v, want 1232", got)
	}

	hysteriaRuntime := automaticXrayRuntime("hysteria", "", "")
	compiled, err = Compile(hysteriaRuntime)
	if err != nil {
		t.Fatal(err)
	}
	hysteriaStream := compiled.Document["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)
	quic := hysteriaStream["finalmask"].(map[string]any)["quicParams"].(map[string]any)
	if disabled, ok := quic["disablePathMTUDiscovery"].(bool); !ok || disabled {
		t.Fatalf("QUIC path MTU discovery = %v, want enabled", quic["disablePathMTUDiscovery"])
	}

	conflict := automaticXrayRuntime("hysteria", "", `{"finalmask":{"quicParams":{"disablePathMTUDiscovery":true}}}`)
	if _, err := Compile(conflict); err == nil {
		t.Fatal("automatic link optimization accepted disabled QUIC path MTU discovery")
	}
}

func automaticXrayRuntime(network, sockoptJSON, streamJSON string) *config.GeneratedRuntime {
	return &config.GeneratedRuntime{
		Devices: []config.RuntimeDevice{{ID: "tun0", MTU: 1500, LinkAutoOptimize: true}},
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID: "xr1", Runtime: model.XrayExternal, InboundProtocol: "vless", OutboundProtocol: "freedom",
			Network: network, SockoptJSON: sockoptJSON, StreamSettingsJSON: streamJSON,
		}},
		Connectors: []config.RuntimeEndpoint{{
			ID: "connector-xr", Transport: model.TransportXray, Remote: "192.0.2.10",
			XrayProfileID: "xr1", Binding: config.RuntimeBinding{DeviceID: "tun0"}, ExternalBridgePort: 39001,
		}},
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

func TestCompileEmbeddedListenerRoutesAuthenticatedUsersByPriority(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID: "xr1", Runtime: model.XrayEmbedded, InboundProtocol: "vless",
			InboundSettingsJSON: `{"decryption":"none"}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID: "listener-xr", Transport: model.TransportXray, BindPort: 18080, XrayProfileID: "xr1",
		}},
		Clients: []model.Client{
			{ID: "client-a", Enabled: true, Email: "a@example.test", UUID: "11111111-1111-4111-8111-111111111111", ListenerID: "listener-xr"},
			{ID: "client-b", Enabled: true, Email: "b@example.test", UUID: "22222222-2222-4222-8222-222222222222", ListenerID: "listener-xr"},
		},
		XrayPipes: []config.RuntimeXrayPipe{
			{EndpointID: "listener-xr", EndpointKind: "listener", HandlerTag: "tapx-frame-a", Priority: 10, Action: model.RouteActionBindDevice, ClientEmail: "a@example.test", DeviceID: "tun-a"},
			{EndpointID: "listener-xr", EndpointKind: "listener", HandlerTag: "tapx-drop-b", Priority: 20, Action: model.RouteActionDrop, ClientEmail: "b@example.test", RouteID: "drop-b"},
			{EndpointID: "listener-xr", EndpointKind: "listener", HandlerTag: FrameOutboundTag("listener-xr"), Priority: int(^uint(0) >> 1), Action: model.RouteActionBindDevice, DeviceID: "tun-default"},
		},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	rules := compiled.EmbeddedDocument["routing"].(map[string]any)["rules"].([]any)
	if len(rules) != 3 {
		t.Fatalf("rules = %+v, want two user policies plus fallback", rules)
	}
	first := rules[0].(map[string]any)
	second := rules[1].(map[string]any)
	third := rules[2].(map[string]any)
	if first["outboundTag"] != "tapx-frame-a" || first["user"].([]any)[0] != "a@example.test" {
		t.Fatalf("first rule = %+v", first)
	}
	if second["outboundTag"] != frameDropOutboundTag || second["user"].([]any)[0] != "b@example.test" {
		t.Fatalf("second rule = %+v", second)
	}
	if third["outboundTag"] != FrameOutboundTag("listener-xr") {
		t.Fatalf("fallback rule = %+v", third)
	}
	outbounds := compiled.EmbeddedDocument["outbounds"].([]any)
	foundDrop := false
	for _, item := range outbounds {
		outbound := item.(map[string]any)
		if outbound["tag"] == frameDropOutboundTag && outbound["protocol"] == "blackhole" {
			foundDrop = true
		}
	}
	if !foundDrop {
		t.Fatalf("outbounds = %+v, want TapX blackhole", outbounds)
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

func TestCompileInjectsAssignedClientsWithoutMovingProtocolFieldsToUser(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr-vless",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "vless",
			InboundSettingsJSON: `{"clients":[{"id":"11111111-1111-4111-8111-111111111111","email":"a@example.test","flow":"xtls-rprx-vision"}],"decryption":"none"}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-vless",
			Transport:     model.TransportXray,
			BindPort:      443,
			XrayProfileID: "xr-vless",
		}},
		Clients: []model.Client{{
			ID:          "client-a",
			Enabled:     true,
			Email:       "a@example.test",
			ListenerIDs: []string{"listener-vless"},
			UUID:        "11111111-1111-4111-8111-111111111111",
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	inbound := compiled.EmbeddedDocument["inbounds"].([]any)[0].(map[string]any)
	settings := inbound["settings"].(map[string]any)
	clients := settings["clients"].([]any)
	if len(clients) != 1 {
		t.Fatalf("clients = %+v, want one assigned client", clients)
	}
	client := clients[0].(map[string]any)
	if client["id"] != "11111111-1111-4111-8111-111111111111" || client["email"] != "a@example.test" || client["flow"] != "xtls-rprx-vision" {
		t.Fatalf("client = %+v, want VLESS credentials", client)
	}
}

func TestCompileInjectsInboundFallbacks(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID: "xr-vless", Runtime: model.XrayEmbedded, InboundProtocol: "vless",
			InboundSettingsJSON: `{"clients":[],"decryption":"none"}`,
			FallbacksJSON:       `[{"name":"edge.example","alpn":"h2","path":"/fallback","dest":"127.0.0.1:8080","xver":1}]`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID: "listener-vless", Transport: model.TransportXray, BindPort: 443, XrayProfileID: "xr-vless",
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	inbound := compiled.EmbeddedDocument["inbounds"].([]any)[0].(map[string]any)
	settings := inbound["settings"].(map[string]any)
	fallbacks := settings["fallbacks"].([]any)
	if len(fallbacks) != 1 {
		t.Fatalf("fallbacks = %+v, want one", fallbacks)
	}
	fallback := fallbacks[0].(map[string]any)
	if fallback["dest"] != "127.0.0.1:8080" || fallback["xver"] != float64(1) {
		t.Fatalf("fallback = %+v", fallback)
	}
}

func TestCompileKeepsWireguardPeersInProfile(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID:                  "xr-wg",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "wireguard",
			InboundSettingsJSON: `{"secretKey":"server-key","peers":[{"publicKey":"client-public-key","preSharedKey":"psk","allowedIPs":["10.0.0.2/32"]}]}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID:            "listener-wg",
			Transport:     model.TransportXray,
			BindPort:      51820,
			XrayProfileID: "xr-wg",
		}},
		Clients: []model.Client{{
			ID:         "client-wg",
			Enabled:    true,
			ListenerID: "listener-wg",
		}},
	}

	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	inbound := compiled.EmbeddedDocument["inbounds"].([]any)[0].(map[string]any)
	settings := inbound["settings"].(map[string]any)
	peers := settings["peers"].([]any)
	peer := peers[0].(map[string]any)
	if peer["publicKey"] != "client-public-key" || peer["preSharedKey"] != "psk" {
		t.Fatalf("peer = %+v, want WireGuard credentials", peer)
	}
}
