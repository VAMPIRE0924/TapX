package config

import (
	"strings"
	"testing"

	"tapx/internal/model"
)

func TestValidateAllowsBareRawUDP(t *testing.T) {
	cfg := RuntimeConfig{
		Listeners: []model.Listener{{
			ID:        "listener-udp",
			Enabled:   true,
			Name:      "bare udp",
			BindPort:  4000,
			Transport: model.TransportUDP,
		}},
	}

	if err := ValidateForSave(cfg); err != nil {
		t.Fatalf("ValidateForSave() error = %v", err)
	}
	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.Listeners) != 1 {
		t.Fatalf("runtime listeners = %d, want 1", len(runtime.Listeners))
	}
	if runtime.Listeners[0].Binding.VKeyValue != "" {
		t.Fatalf("bare listener got vKey %q", runtime.Listeners[0].Binding.VKeyValue)
	}
}

func TestGenerateRuntimeResolvesVKeyRouteAndAddressLimit(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Name:    "tun0",
			Type:    model.DeviceTUN,
			IfName:  "tun0",
			MTU:     1500,
		}},
		VKeys: []model.VKey{{
			ID:      "vk1",
			Enabled: true,
			Name:    "raw key",
			Value:   "secret",
		}},
		Addresses: []model.AddressLimit{{
			ID:        "addr1",
			Enabled:   true,
			Name:      "allowed tun ip",
			DeviceID:  "tun0",
			IPv4CIDRs: []string{"10.30.0.2/32"},
		}},
		Routes: []model.Route{{
			ID:        "route1",
			Enabled:   true,
			VKeyID:    "vk1",
			DeviceID:  "tun0",
			AddressID: "addr1",
		}},
		Listeners: []model.Listener{{
			ID:        "listener-udp",
			Enabled:   true,
			Name:      "udp with route",
			BindPort:  4000,
			Transport: model.TransportUDP,
			Binding: model.Binding{
				RouteID: "route1",
			},
		}},
	}

	if err := ValidateForSave(cfg); err != nil {
		t.Fatalf("ValidateForSave() error = %v", err)
	}
	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	got := runtime.Listeners[0].Binding
	if got.VKeyValue != "secret" {
		t.Fatalf("runtime vKey = %q, want secret", got.VKeyValue)
	}
	if got.DeviceID != "tun0" {
		t.Fatalf("runtime device = %q, want tun0", got.DeviceID)
	}
	if got.AddressID != "addr1" {
		t.Fatalf("runtime address = %q, want addr1", got.AddressID)
	}
	if len(runtime.UDPPipes) != 1 {
		t.Fatalf("runtime udp pipes = %d, want 1", len(runtime.UDPPipes))
	}
	pipe := runtime.UDPPipes[0]
	if pipe.DeviceID != "tun0" {
		t.Fatalf("pipe device = %q, want tun0", pipe.DeviceID)
	}
	if pipe.Binding.VKeyValue != "secret" {
		t.Fatalf("pipe vKey = %q, want secret", pipe.Binding.VKeyValue)
	}
	if pipe.FrameKind != model.DeviceTUN {
		t.Fatalf("pipe frame kind = %q, want tun", pipe.FrameKind)
	}
	if pipe.MaxFrameSize != 1500 {
		t.Fatalf("pipe max frame size = %d, want 1500", pipe.MaxFrameSize)
	}
	if len(pipe.AddressGuard.IPv4CIDRs) != 1 || pipe.AddressGuard.IPv4CIDRs[0] != "10.30.0.2/32" {
		t.Fatalf("pipe address guard = %+v, want 10.30.0.2/32", pipe.AddressGuard)
	}
}

func TestGenerateRuntimeCopiesRawUDPSocketSettings(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Type:    model.DeviceTUN,
			IfName:  "tun0",
			MTU:     1500,
		}},
		Routes: []model.Route{{
			ID:       "route1",
			Enabled:  true,
			DeviceID: "tun0",
		}},
		Listeners: []model.Listener{{
			ID:        "listener-udp",
			Enabled:   true,
			BindHost:  "0.0.0.0",
			BindPort:  4000,
			Transport: model.TransportUDP,
			RawUDP: model.RawUDPSettings{
				PeerMode:      model.UDPPeerFixed,
				FixedPeer:     "127.0.0.1:5000",
				BindInterface: "lo",
				BindAddress:   "127.0.0.1",
				ReceiveBuffer: 65536,
				SendBuffer:    131072,
				ReuseAddr:     true,
				ReusePort:     true,
				DTLS: model.RawDTLSSettings{
					Enabled:      true,
					CertFile:     "/etc/tapx/dtls/server.crt",
					KeyFile:      "/etc/tapx/dtls/server.key",
					ALPN:         []string{"tapx"},
					MTU:          1200,
					ReplayWindow: 64,
				},
			},
			Binding: model.Binding{
				RouteID: "route1",
			},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.UDPPipes) != 1 {
		t.Fatalf("runtime udp pipes = %d, want 1", len(runtime.UDPPipes))
	}
	pipe := runtime.UDPPipes[0]
	if pipe.BindInterface != "lo" {
		t.Fatalf("pipe bind interface = %q, want lo", pipe.BindInterface)
	}
	if pipe.BindAddress != "127.0.0.1" {
		t.Fatalf("pipe bind address = %q, want 127.0.0.1", pipe.BindAddress)
	}
	if pipe.ReceiveBuffer != 65536 || pipe.SendBuffer != 131072 {
		t.Fatalf("pipe buffers = %d/%d, want 65536/131072", pipe.ReceiveBuffer, pipe.SendBuffer)
	}
	if !pipe.ReuseAddr || !pipe.ReusePort {
		t.Fatalf("pipe reuse flags = %v/%v, want true/true", pipe.ReuseAddr, pipe.ReusePort)
	}
	if !pipe.DTLS.Enabled || pipe.DTLS.MTU != 1200 || pipe.DTLS.ReplayWindow != 64 {
		t.Fatalf("pipe dtls = %+v, want copied DTLS settings", pipe.DTLS)
	}
}

func TestGenerateRuntimeCopiesBridgeConfig(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tap0",
			Enabled: true,
			Type:    model.DeviceTAP,
			IfName:  "tapx0",
			MTU:     1500,
			Bridge: &model.BridgeConfig{
				Enabled: true,
				Name:    "brx0",
				IfName:  "eth1",
				MTU:     1400,
			},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.Devices) != 1 {
		t.Fatalf("runtime devices = %+v, want one", runtime.Devices)
	}
	bridge := runtime.Devices[0].Bridge
	if !bridge.Enabled || bridge.Name != "brx0" || bridge.IfName != "eth1" || bridge.MTU != 1400 {
		t.Fatalf("runtime bridge = %+v, want copied bridge", bridge)
	}
}

func TestGenerateRuntimeCopiesEnabledDeviceRoutes(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:       "tun0",
			Enabled:  true,
			Type:     model.DeviceTUN,
			IfName:   "tapx0",
			MSSClamp: 1360,
			DNS: &model.DNSConfig{
				Enabled:       true,
				Nameservers:   []string{"1.1.1.1", "2606:4700:4700::1111"},
				SearchDomains: []string{"example.com"},
				Options:       []string{"timeout:1"},
				OutputPath:    "/run/tapx/resolv/tapx0.conf",
			},
			Routes: []model.DeviceRoute{
				{
					Enabled:     true,
					Destination: "10.50.0.0/24",
					Gateway:     "10.10.0.2",
					Source:      "10.10.0.1",
					IfName:      "tapx0",
					Metric:      20,
					Table:       "100",
				},
				{
					Enabled:     false,
					Destination: "10.60.0.0/24",
				},
			},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.Devices) != 1 {
		t.Fatalf("runtime devices = %+v, want one", runtime.Devices)
	}
	if runtime.Devices[0].MSSClamp != 1360 {
		t.Fatalf("runtime mss clamp = %d, want 1360", runtime.Devices[0].MSSClamp)
	}
	dns := runtime.Devices[0].DNS
	if !dns.Enabled || len(dns.Nameservers) != 2 || dns.Nameservers[0] != "1.1.1.1" || dns.OutputPath != "/run/tapx/resolv/tapx0.conf" {
		t.Fatalf("runtime DNS = %+v, want copied DNS", dns)
	}
	routes := runtime.Devices[0].Routes
	if len(routes) != 1 {
		t.Fatalf("runtime device routes = %+v, want one enabled route", routes)
	}
	route := routes[0]
	if route.Destination != "10.50.0.0/24" || route.Gateway != "10.10.0.2" || route.Source != "10.10.0.1" || route.IfName != "tapx0" || route.Metric != 20 || route.Table != "100" {
		t.Fatalf("runtime device route = %+v, want copied route", route)
	}
}

func TestValidateRejectsInvalidRawUDPAddresses(t *testing.T) {
	cfg := RuntimeConfig{
		Listeners: []model.Listener{{
			ID:        "listener-udp",
			Enabled:   true,
			BindPort:  4000,
			Transport: model.TransportUDP,
			RawUDP: model.RawUDPSettings{
				PeerMode:    model.UDPPeerFixed,
				FixedPeer:   "not-an-addr",
				BindAddress: "not-an-ip",
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want address errors")
	}
	if !strings.Contains(err.Error(), "RawUDP.FixedPeer") {
		t.Fatalf("ValidateForSave() error = %v, want fixed peer message", err)
	}
	if !strings.Contains(err.Error(), "RawUDP.BindAddress") {
		t.Fatalf("ValidateForSave() error = %v, want bind address message", err)
	}
}

func TestValidateRejectsInvalidRawTCPBindAddress(t *testing.T) {
	cfg := RuntimeConfig{
		Listeners: []model.Listener{{
			ID:        "listener-tcp",
			Enabled:   true,
			BindPort:  5000,
			Transport: model.TransportTCP,
			RawTCP: model.RawTCPSettings{
				BindAddress: "not-an-ip",
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want bind address error")
	}
	if !strings.Contains(err.Error(), "RawTCP.BindAddress") {
		t.Fatalf("ValidateForSave() error = %v, want bind address message", err)
	}
}

func TestValidateAllowsApplyingRawTLSDTLS(t *testing.T) {
	cfg := RuntimeConfig{
		Listeners: []model.Listener{{
			ID:        "tcp-tls",
			Enabled:   true,
			BindPort:  4400,
			Transport: model.TransportTCP,
			RawTCP: model.RawTCPSettings{
				LengthMode: model.TCPLength16,
				TLS: model.RawTLSSettings{
					Enabled:       true,
					CertFile:      "/etc/tapx/tls/server.crt",
					KeyFile:       "/etc/tapx/tls/server.key",
					CAFile:        "/etc/tapx/tls/ca.crt",
					ServerName:    "tapx.example",
					ALPN:          []string{"tapx"},
					MinVersion:    "1.2",
					MaxVersion:    "1.3",
					AllowInsecure: false,
				},
			},
		}, {
			ID:        "udp-dtls",
			Enabled:   true,
			BindPort:  4401,
			Transport: model.TransportUDP,
			RawUDP: model.RawUDPSettings{
				PeerMode: model.UDPPeerLearn,
				DTLS: model.RawDTLSSettings{
					Enabled:    true,
					CertFile:   "/etc/tapx/dtls/server.crt",
					KeyFile:    "/etc/tapx/dtls/server.key",
					ServerName: "tapx.example",
					ALPN:       []string{"tapx"},
					MinVersion: "1.2",
					MaxVersion: "1.3",
					MTU:        1200,
				},
			},
		}},
	}

	if err := ValidateForSave(cfg); err != nil {
		t.Fatalf("ValidateForSave() error = %v", err)
	}
	if err := ValidateForApply(cfg); err != nil {
		t.Fatalf("ValidateForApply() error = %v", err)
	}
}

func TestValidateRawTLSRequiresListenerCertificatePair(t *testing.T) {
	cfg := RuntimeConfig{
		Listeners: []model.Listener{{
			ID:        "tcp-tls",
			Enabled:   true,
			BindPort:  4400,
			Transport: model.TransportTCP,
			RawTCP: model.RawTCPSettings{
				TLS: model.RawTLSSettings{Enabled: true, CertFile: "/etc/tapx/tls/server.crt"},
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want missing key problem")
	}
	if !strings.Contains(err.Error(), "RawTCP.TLS.KeyFile") {
		t.Fatalf("ValidateForSave() error = %v, want RawTCP.TLS.KeyFile problem", err)
	}
}

func TestValidateRawSecurityRejectsInvalidVersionRangeAndALPN(t *testing.T) {
	cfg := RuntimeConfig{
		Connectors: []model.Connector{{
			ID:        "tcp-tls",
			Enabled:   true,
			Remote:    "127.0.0.1",
			Port:      4400,
			Transport: model.TransportTCP,
			RawTCP: model.RawTCPSettings{
				TLS: model.RawTLSSettings{
					Enabled:    true,
					ALPN:       []string{"bad proto"},
					MinVersion: "1.3",
					MaxVersion: "1.2",
				},
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want raw TLS validation problems")
	}
	text := err.Error()
	if !strings.Contains(text, "RawTCP.TLS.ALPN[0]") || !strings.Contains(text, "RawTCP.TLS.MaxVersion") {
		t.Fatalf("ValidateForSave() error = %v, want ALPN and version problems", err)
	}
}

func TestValidateClientCredentialsAndConnectorBinding(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0"}},
		Connectors: []model.Connector{{
			ID: "conn-a", Enabled: true, Remote: "192.0.2.10", Port: 46000, Transport: model.TransportUDP,
			Binding: model.Binding{DeviceID: "tun-a"},
		}},
		Routes: []model.Route{{ID: "route-a", Enabled: true, DeviceID: "tun-a", ConnectorID: "conn-a"}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, CredentialType: "uuid", CredentialValue: "11111111-1111-4111-8111-111111111111",
			Binding: model.Binding{RouteID: "route-a", ConnectorID: "conn-a"},
		}},
	}
	if err := ValidateForApply(cfg); err != nil {
		t.Fatalf("ValidateForApply() error = %v", err)
	}
}

func TestValidateRejectsInvalidClientCredentialAndConnectorConflict(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0"}},
		Connectors: []model.Connector{
			{ID: "conn-a", Enabled: true, Remote: "192.0.2.10", Port: 46000, Transport: model.TransportUDP, Binding: model.Binding{DeviceID: "tun-a"}},
			{ID: "conn-b", Enabled: true, Remote: "192.0.2.11", Port: 46000, Transport: model.TransportUDP, Binding: model.Binding{DeviceID: "tun-a"}},
		},
		Routes: []model.Route{{ID: "route-a", Enabled: true, DeviceID: "tun-a", ConnectorID: "conn-a"}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, CredentialType: "uuid", CredentialValue: "not-a-uuid",
			Binding: model.Binding{RouteID: "route-a", ConnectorID: "conn-b"},
		}},
	}
	err := ValidateForApply(cfg)
	if err == nil {
		t.Fatalf("ValidateForApply() error = nil, want validation errors")
	}
	text := err.Error()
	if !strings.Contains(text, "CredentialValue") || !strings.Contains(text, "Binding.ConnectorID") {
		t.Fatalf("ValidateForApply() error = %v, want credential and connector conflict", err)
	}
}

func TestValidateRejectsOversizedVKey(t *testing.T) {
	cfg := RuntimeConfig{
		VKeys: []model.VKey{{
			ID:      "vk1",
			Enabled: true,
			Value:   strings.Repeat("x", 1025),
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want oversized vKey error")
	}
	if !strings.Contains(err.Error(), "1024 bytes or less") {
		t.Fatalf("ValidateForSave() error = %v, want vKey length message", err)
	}
}

func TestValidateRejectsRawVKeyOnXray(t *testing.T) {
	cfg := RuntimeConfig{
		VKeys: []model.VKey{{
			ID:      "vk1",
			Enabled: true,
			Value:   "secret",
		}},
		Listeners: []model.Listener{{
			ID:        "listener-xray",
			Enabled:   true,
			Transport: model.TransportXray,
			Binding: model.Binding{
				VKeyID: "vk1",
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want vKey/xray error")
	}
	if !strings.Contains(err.Error(), "vKey is only valid") {
		t.Fatalf("ValidateForSave() error = %v, want vKey message", err)
	}
}

func TestGenerateRuntimeIncludesXrayProfilesAndSettings(t *testing.T) {
	cfg := RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID:                  "xr1",
			Enabled:             true,
			Name:                "embedded reality",
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "vless",
			InboundSettingsJSON: `{"clients":[]}`,
			Network:             "tcp",
			Security:            "reality",
			StreamSettingsJSON:  `{"network":"tcp"}`,
		}},
		Settings: []model.Settings{{
			ID:                  "global",
			Enabled:             true,
			Name:                "global",
			LogLevel:            "info",
			StatsIntervalSecond: 5,
			OpenWrtBuildTarget:  "x86-64",
		}},
		Listeners: []model.Listener{{
			ID:            "listener-xray",
			Enabled:       true,
			BindPort:      443,
			Transport:     model.TransportXray,
			XrayProfileID: "xr1",
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.XrayProfiles) != 1 {
		t.Fatalf("runtime xray profiles = %d, want 1", len(runtime.XrayProfiles))
	}
	if runtime.Listeners[0].XrayProfileID != "xr1" {
		t.Fatalf("listener xray profile = %q, want xr1", runtime.Listeners[0].XrayProfileID)
	}
	if len(runtime.Settings) != 1 || runtime.Settings[0].OpenWrtBuildTarget != "x86-64" {
		t.Fatalf("runtime settings = %+v, want x86-64 target", runtime.Settings)
	}
}

func TestGenerateRuntimeBuildsEmbeddedXrayPipeWhenDeviceBound(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Type:    model.DeviceTUN,
			IfName:  "tapxxray0",
			MTU:     1400,
		}},
		Addresses: []model.AddressLimit{{
			ID:        "addr1",
			Enabled:   true,
			DeviceID:  "tun0",
			IPv4CIDRs: []string{"10.79.0.2/32"},
		}},
		Routes: []model.Route{{
			ID:        "route1",
			Enabled:   true,
			DeviceID:  "tun0",
			AddressID: "addr1",
		}},
		XrayProfiles: []model.XrayProfile{{
			ID:                  "xr1",
			Enabled:             true,
			Runtime:             model.XrayEmbedded,
			InboundProtocol:     "dokodemo-door",
			InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
		}},
		Listeners: []model.Listener{{
			ID:            "listener-xray",
			Enabled:       true,
			BindPort:      18080,
			Transport:     model.TransportXray,
			XrayProfileID: "xr1",
			RawTCP: model.RawTCPSettings{
				LengthMode: model.TCPLength32,
			},
			Binding: model.Binding{
				RouteID: "route1",
			},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.XrayPipes) != 1 {
		t.Fatalf("runtime xray pipes = %d, want 1", len(runtime.XrayPipes))
	}
	pipe := runtime.XrayPipes[0]
	if pipe.EndpointID != "listener-xray" || pipe.EndpointKind != "listener" {
		t.Fatalf("xray pipe endpoint = %+v, want listener-xray listener", pipe)
	}
	if pipe.DeviceID != "tun0" || pipe.FrameKind != model.DeviceTUN || pipe.MaxFrameSize != 1400 {
		t.Fatalf("xray pipe device fields = %+v, want tun0/tun/1400", pipe)
	}
	if pipe.LengthMode != model.TCPLength32 {
		t.Fatalf("xray pipe length mode = %q, want uint32", pipe.LengthMode)
	}
	if len(pipe.AddressGuard.IPv4CIDRs) != 1 || pipe.AddressGuard.IPv4CIDRs[0] != "10.79.0.2/32" {
		t.Fatalf("xray pipe address guard = %+v, want 10.79.0.2/32", pipe.AddressGuard)
	}
}

func TestGenerateRuntimeDoesNotBuildXrayPipeWithoutEmbeddedDeviceBinding(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Type:    model.DeviceTUN,
			IfName:  "tapxxray0",
		}},
		XrayProfiles: []model.XrayProfile{
			{
				ID:                  "embedded",
				Enabled:             true,
				Runtime:             model.XrayEmbedded,
				InboundProtocol:     "dokodemo-door",
				InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			},
			{
				ID:                  "external",
				Enabled:             true,
				Runtime:             model.XrayExternal,
				InboundProtocol:     "dokodemo-door",
				InboundSettingsJSON: `{"address":"127.0.0.1","port":80,"network":"tcp"}`,
			},
		},
		Listeners: []model.Listener{
			{
				ID:            "embedded-unbound",
				Enabled:       true,
				BindPort:      18080,
				Transport:     model.TransportXray,
				XrayProfileID: "embedded",
			},
			{
				ID:            "external-bound",
				Enabled:       true,
				BindPort:      18081,
				Transport:     model.TransportXray,
				XrayProfileID: "external",
				Binding:       model.Binding{DeviceID: "tun0"},
			},
		},
		Settings: []model.Settings{{
			ID:               "global",
			Enabled:          true,
			ExternalXrayPath: "/usr/bin/xray",
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.XrayPipes) != 0 {
		t.Fatalf("runtime xray pipes = %+v, want none", runtime.XrayPipes)
	}
}

func TestValidateRejectsInvalidXrayProfileComposition(t *testing.T) {
	cfg := RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID:                 "xr1",
			Enabled:            true,
			Runtime:            "bad",
			StreamSettingsJSON: `{bad-json`,
		}},
		Listeners: []model.Listener{{
			ID:            "listener-raw",
			Enabled:       true,
			BindPort:      4000,
			Transport:     model.TransportUDP,
			XrayProfileID: "xr1",
		}},
		Settings: []model.Settings{{
			ID:                 "global",
			Enabled:            true,
			LogLevel:           "verbose",
			OpenWrtBuildTarget: "mt7986",
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want xray/settings errors")
	}
	for _, want := range []string{"XrayProfile[xr1].Runtime", "StreamSettingsJSON", "Listener[listener-raw].XrayProfileID", "Settings[global].LogLevel", "OpenWrtBuildTarget"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestValidateApplyRequiresExternalXrayRuntimeFields(t *testing.T) {
	cfg := RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID:      "xr1",
			Enabled: true,
			Runtime: model.XrayExternal,
		}},
		Settings: []model.Settings{{
			ID:      "global",
			Enabled: true,
		}},
		Listeners: []model.Listener{{
			ID:            "listener-xray",
			Enabled:       true,
			Transport:     model.TransportXray,
			XrayProfileID: "xr1",
		}},
		Connectors: []model.Connector{{
			ID:            "connector-xray",
			Enabled:       true,
			Transport:     model.TransportXray,
			XrayProfileID: "xr1",
		}},
	}

	if err := ValidateForSave(cfg); err != nil {
		t.Fatalf("ValidateForSave() error = %v", err)
	}
	err := ValidateForApply(cfg)
	if err == nil {
		t.Fatal("ValidateForApply() error = nil, want external xray runtime field errors")
	}
	for _, want := range []string{"Listener[listener-xray].BindPort", "XrayProfile[xr1].InboundProtocol", "XrayProfile[xr1].OutboundProtocol", "Settings[global].ExternalXrayPath"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForApply() error = %v, want %s", err, want)
		}
	}
}

func TestValidateRejectsInvalidPanelAuthSettings(t *testing.T) {
	cfg := RuntimeConfig{
		Settings: []model.Settings{{
			ID:                "global",
			Enabled:           true,
			PanelAuthEnabled:  true,
			AdminPasswordHash: "bad",
			SessionTTLSecond:  -1,
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want panel auth errors")
	}
	for _, want := range []string{"AdminUsername", "AdminPasswordHash", "SessionTTLSecond"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestValidateRejectsMACLimitOnTUN(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Type:    model.DeviceTUN,
			IfName:  "tun0",
		}},
		Addresses: []model.AddressLimit{{
			ID:       "addr1",
			Enabled:  true,
			DeviceID: "tun0",
			MACs:     []string{"02:00:00:00:00:01"},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want tun/mac error")
	}
	if !strings.Contains(err.Error(), "MAC limits are only valid") {
		t.Fatalf("ValidateForSave() error = %v, want MAC/TUN message", err)
	}
}

func TestValidateRejectsInvalidAddressLimitStaticConfig(t *testing.T) {
	cfg := RuntimeConfig{
		Addresses: []model.AddressLimit{{
			ID:          "addr1",
			Enabled:     true,
			IPv4Gateway: "2001:db8::1",
			IPv6Gateway: "10.0.0.1",
			DNS:         []string{"bad dns"},
			Routes:      []string{"bad route"},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want address static config errors")
	}
	for _, want := range []string{"IPv4Gateway", "IPv6Gateway", "DNS", "Routes"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestValidateRejectsInvalidBridgeConfig(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Type:    model.DeviceTUN,
			IfName:  "tun0",
			Bridge:  &model.BridgeConfig{Enabled: true, MTU: -1},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want bridge errors")
	}
	for _, want := range []string{"Bridge", "Bridge.Name", "Bridge.MTU"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestValidateRejectsInvalidDeviceRoutes(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:       "tun0",
			Enabled:  true,
			Type:     model.DeviceTUN,
			IfName:   "tapx0",
			MSSClamp: 10,
			Routes: []model.DeviceRoute{
				{Enabled: true},
				{Enabled: true, Destination: "2001:db8::/64", Gateway: "10.0.0.1"},
				{Enabled: true, Destination: "10.0.0.0/24", Source: "2001:db8::1"},
				{Enabled: true, Destination: "bad", Gateway: "bad", IfName: "bad if", Metric: -1, Table: "bad table"},
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want route errors")
	}
	for _, want := range []string{"MSSClamp", "Routes[0].Destination", "Routes[1].Gateway", "Routes[2].Source", "Routes[3].Destination", "Routes[3].Gateway", "Routes[3].IfName", "Routes[3].Metric", "Routes[3].Table"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestValidateRejectsInvalidDeviceDNS(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Type:    model.DeviceTUN,
			IfName:  "tapx0",
			DNS: &model.DNSConfig{
				Enabled:       true,
				Nameservers:   []string{"bad-ip"},
				SearchDomains: []string{"bad domain"},
				Options:       []string{"bad option"},
				OutputPath:    "relative/path",
			},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want DNS errors")
	}
	for _, want := range []string{"DNS.Nameservers", "DNS.SearchDomains", "DNS.Options", "DNS.OutputPath"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestGenerateRuntimeBuildsTAPAddressGuard(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tap0",
			Enabled: true,
			Type:    model.DeviceTAP,
			IfName:  "tap0",
			MTU:     1500,
		}},
		Addresses: []model.AddressLimit{{
			ID:        "addr1",
			Enabled:   true,
			DeviceID:  "tap0",
			MACs:      []string{"02:00:00:00:00:01"},
			IPv4CIDRs: []string{"10.40.0.0/24"},
			IPv6CIDRs: []string{"2001:db8:40::/64"},
		}},
		Routes: []model.Route{{
			ID:        "route1",
			Enabled:   true,
			DeviceID:  "tap0",
			AddressID: "addr1",
		}},
		Listeners: []model.Listener{{
			ID:        "listener-udp",
			Enabled:   true,
			BindPort:  4000,
			Transport: model.TransportUDP,
			Binding: model.Binding{
				RouteID: "route1",
			},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.UDPPipes) != 1 {
		t.Fatalf("runtime udp pipes = %d, want 1", len(runtime.UDPPipes))
	}
	guard := runtime.UDPPipes[0].AddressGuard
	if len(guard.MACs) != 1 || guard.MACs[0] != "02:00:00:00:00:01" {
		t.Fatalf("guard MACs = %+v, want one MAC", guard.MACs)
	}
	if len(guard.IPv4CIDRs) != 1 || guard.IPv4CIDRs[0] != "10.40.0.0/24" {
		t.Fatalf("guard IPv4CIDRs = %+v, want 10.40.0.0/24", guard.IPv4CIDRs)
	}
	if len(guard.IPv6CIDRs) != 1 || guard.IPv6CIDRs[0] != "2001:db8:40::/64" {
		t.Fatalf("guard IPv6CIDRs = %+v, want 2001:db8:40::/64", guard.IPv6CIDRs)
	}
}

func TestGenerateRuntimeBuildsTCPPipe(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:      "tun0",
			Enabled: true,
			Name:    "tun0",
			Type:    model.DeviceTUN,
			IfName:  "tun0",
			MTU:     1400,
		}},
		Routes: []model.Route{{
			ID:       "route1",
			Enabled:  true,
			DeviceID: "tun0",
		}},
		Connectors: []model.Connector{{
			ID:        "tcp-connector",
			Enabled:   true,
			Name:      "tcp connector",
			Remote:    "127.0.0.1",
			Port:      5000,
			Transport: model.TransportTCP,
			RawTCP: model.RawTCPSettings{
				LengthMode:      model.TCPLength32,
				BindInterface:   "lo",
				BindAddress:     "127.0.0.1",
				ReceiveBuffer:   65536,
				SendBuffer:      131072,
				NoDelay:         true,
				KeepAliveSecond: 30,
				FastOpen:        true,
				ConnectTimeout:  3,
				TLS: model.RawTLSSettings{
					Enabled:       true,
					ServerName:    "tapx.example",
					ALPN:          []string{"tapx"},
					MinVersion:    "1.2",
					MaxVersion:    "1.3",
					AllowInsecure: true,
				},
			},
			Binding: model.Binding{
				RouteID: "route1",
			},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.TCPPipes) != 1 {
		t.Fatalf("runtime tcp pipes = %d, want 1", len(runtime.TCPPipes))
	}
	pipe := runtime.TCPPipes[0]
	if pipe.LengthMode != model.TCPLength32 {
		t.Fatalf("pipe length mode = %q, want uint32", pipe.LengthMode)
	}
	if pipe.BindInterface != "lo" {
		t.Fatalf("pipe bind interface = %q, want lo", pipe.BindInterface)
	}
	if pipe.BindAddress != "127.0.0.1" {
		t.Fatalf("pipe bind address = %q, want 127.0.0.1", pipe.BindAddress)
	}
	if pipe.ReceiveBuffer != 65536 || pipe.SendBuffer != 131072 {
		t.Fatalf("pipe buffers = %d/%d, want 65536/131072", pipe.ReceiveBuffer, pipe.SendBuffer)
	}
	if !pipe.NoDelay || pipe.KeepAliveSecond != 30 || !pipe.FastOpen || pipe.ConnectTimeout != 3 {
		t.Fatalf("pipe tcp flags = nodelay:%v keepalive:%d fastopen:%v timeout:%d", pipe.NoDelay, pipe.KeepAliveSecond, pipe.FastOpen, pipe.ConnectTimeout)
	}
	if !pipe.TLS.Enabled || pipe.TLS.ServerName != "tapx.example" || len(pipe.TLS.ALPN) != 1 || pipe.TLS.ALPN[0] != "tapx" {
		t.Fatalf("pipe tls = %+v, want copied TLS settings", pipe.TLS)
	}
	if pipe.MaxFrameSize != 1400 {
		t.Fatalf("pipe max frame size = %d, want 1400", pipe.MaxFrameSize)
	}
	if pipe.DeviceID != "tun0" {
		t.Fatalf("pipe device = %q, want tun0", pipe.DeviceID)
	}
}
