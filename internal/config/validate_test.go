package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestGenerateRuntimeIncludesEthernetHeaderInTAPFrameLimit(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tap0", Enabled: true, Type: model.DeviceTAP, IfName: "tapx0", MTU: 1500}},
		Listeners: []model.Listener{{
			ID:        "udp0",
			Enabled:   true,
			Transport: model.TransportUDP,
			BindPort:  45000,
			Binding:   model.Binding{DeviceID: "tap0"},
		}},
	}
	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.UDPPipes) != 1 {
		t.Fatalf("UDP pipes = %d, want 1", len(runtime.UDPPipes))
	}
	if runtime.UDPPipes[0].MaxFrameSize != 1522 {
		t.Fatalf("TAP max frame size = %d, want 1522", runtime.UDPPipes[0].MaxFrameSize)
	}
}

func TestGenerateRuntimeCompilesClientRateLimitsIntoBinding(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tun0", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0", MTU: 1400}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true,
			UploadRateLimit: 3_000_000, DownloadRateLimit: 5_000_000,
			Binding: model.Binding{DeviceID: "tun0"},
		}},
		Listeners: []model.Listener{{
			ID: "udp0", Enabled: true, Transport: model.TransportUDP, BindPort: 45000,
			Binding: model.Binding{ClientID: "client-a"},
		}},
	}
	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.UDPPipes) != 1 {
		t.Fatalf("UDP pipes = %d, want 1", len(runtime.UDPPipes))
	}
	binding := runtime.UDPPipes[0].Binding
	if binding.UploadRateLimit != 3_000_000 || binding.DownloadRateLimit != 5_000_000 {
		t.Fatalf("runtime rate limits = (%d, %d)", binding.UploadRateLimit, binding.DownloadRateLimit)
	}
}

func TestGenerateRuntimeResolvesVKeyRouteAndAddressLimit(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:               "tun0",
			Enabled:          true,
			Name:             "tun0",
			Type:             model.DeviceTUN,
			IfName:           "tun0",
			MTU:              1500,
			LinkAutoOptimize: true,
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
			ID:         "route1",
			Enabled:    true,
			Priority:   25,
			Action:     model.RouteActionBindDevice,
			VKeyID:     "vk1",
			ListenerID: "listener-udp",
			DeviceID:   "tun0",
			AddressID:  "addr1",
		}},
		Listeners: []model.Listener{{
			ID:        "listener-udp",
			Enabled:   true,
			Name:      "udp with route",
			BindPort:  4000,
			Transport: model.TransportUDP,
			RawUDP: model.RawUDPSettings{
				QueueSize:      2048,
				ZeroCopy:       true,
				ConnectTimeout: 7,
				IdleTimeout:    60,
			},
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
	if len(runtime.Routes) != 1 {
		t.Fatalf("runtime routes = %d, want 1", len(runtime.Routes))
	}
	route := runtime.Routes[0]
	if route.Priority != 25 || route.Action != model.RouteActionBindDevice || route.ListenerID != "listener-udp" {
		t.Fatalf("runtime route = %+v, want priority/action/listener copied", route)
	}
	if route.Binding.RouteID != "route1" || route.Binding.VKeyValue != "secret" || route.Binding.AddressID != "addr1" {
		t.Fatalf("runtime route binding = %+v, want route/vkey/address copied", route.Binding)
	}
	if len(route.AddressGuard.IPv4CIDRs) != 1 || route.AddressGuard.IPv4CIDRs[0] != "10.30.0.2/32" {
		t.Fatalf("runtime route address guard = %+v, want 10.30.0.2/32", route.AddressGuard)
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
	if !pipe.LinkAutoOptimize {
		t.Fatal("raw UDP pipe did not inherit automatic link optimization")
	}
	if pipe.QueueSize != 2048 || !pipe.ZeroCopy || pipe.ConnectTimeout != 7 || pipe.IdleTimeout != 60 {
		t.Fatalf("pipe fastpath controls = %+v, want queue/zero-copy/timeouts", pipe)
	}
	if len(pipe.AddressGuard.IPv4CIDRs) != 1 || pipe.AddressGuard.IPv4CIDRs[0] != "10.30.0.2/32" {
		t.Fatalf("pipe address guard = %+v, want 10.30.0.2/32", pipe.AddressGuard)
	}
}

func TestGenerateRuntimeSortsRoutesAndResolvesClientPolicy(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-a", MTU: 1500,
		}},
		Addresses: []model.AddressLimit{{
			ID: "addr-client", Enabled: true, DeviceID: "tun-a", ClientID: "client-a",
			IPv4CIDRs: []string{"10.44.0.2/32"},
		}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, AddressID: "addr-client",
			AllowedDeviceIDs: []string{"tun-a"},
			UploadRateLimit:  4_000_000, DownloadRateLimit: 8_000_000,
		}},
		Routes: []model.Route{
			{ID: "later", Enabled: true, Priority: 90, Action: model.RouteActionBindDevice, DeviceID: "tun-a"},
			{ID: "first", Enabled: true, Priority: 10, Action: model.RouteActionBindDevice, ClientID: "client-a", DeviceID: "tun-a"},
			{ID: "same-priority", Enabled: true, Priority: 10, Action: model.RouteActionAllow, ClientID: "client-a"},
		},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if got := []string{runtime.Routes[0].ID, runtime.Routes[1].ID, runtime.Routes[2].ID}; !reflect.DeepEqual(got, []string{"first", "same-priority", "later"}) {
		t.Fatalf("runtime route order = %v", got)
	}
	first := runtime.Routes[0]
	if first.Binding.ClientID != "client-a" || first.Binding.AddressID != "addr-client" {
		t.Fatalf("runtime client route binding = %+v", first.Binding)
	}
	if first.Binding.UploadRateLimit != 4_000_000 || first.Binding.DownloadRateLimit != 8_000_000 {
		t.Fatalf("runtime client route rates = (%d, %d)", first.Binding.UploadRateLimit, first.Binding.DownloadRateLimit)
	}
	if len(first.AddressGuard.IPv4CIDRs) != 1 || first.AddressGuard.IPv4CIDRs[0] != "10.44.0.2/32" {
		t.Fatalf("runtime client route guard = %+v", first.AddressGuard)
	}
}

func TestGenerateRuntimeExpandsEmbeddedXrayListenerPolicies(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{
			{ID: "tun-default", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-default", MTU: 1500},
			{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-a", MTU: 1400},
		},
		XrayProfiles: []model.XrayProfile{{
			ID: "xr1", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "vless",
		}},
		Listeners: []model.Listener{{
			ID: "listener-xr", Enabled: true, BindPort: 443, Transport: model.TransportXray,
			XrayProfileID: "xr1", Binding: model.Binding{DeviceID: "tun-default"},
		}},
		Clients: []model.Client{
			{ID: "client-a", Enabled: true, Email: "a@example.test", ListenerID: "listener-xr", AllowedDeviceIDs: []string{"tun-a"}},
			{ID: "client-b", Enabled: true, Email: "b@example.test", ListenerID: "listener-xr"},
		},
		Routes: []model.Route{
			{ID: "route-a", Enabled: true, Priority: 10, Action: model.RouteActionBindDevice, ListenerID: "listener-xr", ClientID: "client-a", DeviceID: "tun-a"},
			{ID: "route-b", Enabled: true, Priority: 20, Action: model.RouteActionDrop, ListenerID: "listener-xr", ClientID: "client-b"},
		},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.XrayPipes) != 3 {
		t.Fatalf("runtime Xray policies = %+v, want route-a/drop-b/fallback", runtime.XrayPipes)
	}
	first, second, fallback := runtime.XrayPipes[0], runtime.XrayPipes[1], runtime.XrayPipes[2]
	if first.RouteID != "route-a" || first.ClientEmail != "a@example.test" || first.DeviceID != "tun-a" || first.MaxFrameSize != 1400 {
		t.Fatalf("first Xray policy = %+v", first)
	}
	if second.RouteID != "route-b" || second.Action != model.RouteActionDrop || second.ClientEmail != "b@example.test" || second.DeviceID != "" {
		t.Fatalf("drop Xray policy = %+v", second)
	}
	if fallback.ClientEmail != "" || fallback.DeviceID != "tun-default" || fallback.HandlerTag != "tapx-frame-listener-xr" {
		t.Fatalf("fallback Xray policy = %+v", fallback)
	}
}

func TestValidateRejectsInvalidRouteActionAndPriority(t *testing.T) {
	cfg := RuntimeConfig{
		Routes: []model.Route{{
			ID:       "route-bad",
			Enabled:  true,
			Priority: -1,
			Action:   model.RouteAction("redirect"),
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want route errors")
	}
	for _, want := range []string{"Priority", "Action"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateForSave() error = %v, want %s", err, want)
		}
	}
}

func TestValidateRejectsRouteWithoutMatchOrBindDeviceTarget(t *testing.T) {
	t.Run("empty rule", func(t *testing.T) {
		err := ValidateForSave(RuntimeConfig{Routes: []model.Route{{
			ID: "route-empty", Enabled: true, Action: model.RouteActionAllow,
		}}})
		if err == nil || !strings.Contains(err.Error(), "Match") {
			t.Fatalf("ValidateForSave() error = %v, want empty route match problem", err)
		}
	})

	t.Run("bind without device", func(t *testing.T) {
		err := ValidateForSave(RuntimeConfig{
			VKeys: []model.VKey{{ID: "vkey-a", Enabled: true, Value: "secret"}},
			Routes: []model.Route{{
				ID: "route-bind", Enabled: true, Action: model.RouteActionBindDevice, VKeyID: "vkey-a",
			}},
		})
		if err == nil || !strings.Contains(err.Error(), "DeviceID") {
			t.Fatalf("ValidateForSave() error = %v, want bind-device target problem", err)
		}
	})
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
	if len(runtime.UDPDispatches) != 1 {
		t.Fatalf("runtime udp dispatches = %d, want one DTLS listener dispatcher", len(runtime.UDPDispatches))
	}
	pipe := runtime.UDPPipes[0]
	if pipe.DispatchGroup == "" {
		t.Fatal("DTLS listener pipe has no dispatch group")
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

func TestValidateAcceptsProtocolClientAndMultipleListeners(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0"}},
		Listeners: []model.Listener{
			{ID: "listener-xray", Enabled: true, BindPort: 443, Transport: model.TransportXray, XrayProfileID: "xr-a", Binding: model.Binding{DeviceID: "tun-a"}},
			{ID: "listener-raw", Enabled: true, BindPort: 4000, Transport: model.TransportUDP, Binding: model.Binding{DeviceID: "tun-a"}},
		},
		XrayProfiles: []model.XrayProfile{{ID: "xr-a", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "vless", InboundSettingsJSON: `{}`}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, CredentialType: "vless", CredentialValue: "11111111-1111-4111-8111-111111111111",
			UUID: "11111111-1111-4111-8111-111111111111", ListenerID: "listener-xray", ListenerIDs: []string{"listener-xray", "listener-raw"},
			AllowedDeviceIDs: []string{"tun-a"},
		}},
	}
	if err := ValidateForSave(cfg); err != nil {
		t.Fatalf("ValidateForSave() error = %v", err)
	}
}

func TestGenerateRuntimeFiltersExpiredListenerAndCopiesLimits(t *testing.T) {
	now := time.Now().Unix()
	cfg := RuntimeConfig{Listeners: []model.Listener{
		{
			ID: "expired", Enabled: true, BindPort: 4100, Transport: model.TransportUDP,
			ExpiresAt: now - 1,
		},
		{
			ID: "active", Enabled: true, BindPort: 4101, Transport: model.TransportUDP,
			ExpiresAt: now + 3600, TrafficCap: 2 << 30, TrafficReset: "monthly",
		},
	}}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.Listeners) != 1 || runtime.Listeners[0].ID != "active" {
		t.Fatalf("runtime listeners = %+v, want only active", runtime.Listeners)
	}
	got := runtime.Listeners[0]
	if got.ExpiresAt != now+3600 || got.TrafficCap != 2<<30 || got.TrafficReset != "monthly" {
		t.Fatalf("runtime listener limits = %+v", got)
	}
}

func TestGenerateRuntimeHonorsBuiltInKernelSwitches(t *testing.T) {
	falseValue := false
	advanced, err := json.Marshal(map[string]*bool{
		"embeddedXrayEnabled": &falseValue,
		"tapxEnabled":         &falseValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := RuntimeConfig{
		Listeners: []model.Listener{
			{ID: "raw", Enabled: true, BindPort: 4100, Transport: model.TransportUDP},
			{ID: "xray", Enabled: true, BindPort: 443, Transport: model.TransportXray, XrayProfileID: "embedded"},
		},
		XrayProfiles: []model.XrayProfile{{
			ID: "embedded", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "vless", InboundSettingsJSON: `{}`,
		}},
		Settings: []model.Settings{{ID: "settings", Enabled: true, AdvancedJSON: string(advanced)}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.Listeners) != 0 || len(runtime.UDPPipes) != 0 || len(runtime.XrayPipes) != 0 {
		t.Fatalf("disabled built-in kernels generated runtime endpoints: %+v", runtime)
	}

	cfg.Settings[0].AdvancedJSON = `{}`
	runtime, err = GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() with defaults error = %v", err)
	}
	if len(runtime.Listeners) != 2 {
		t.Fatalf("default kernel switches generated %d listeners, want 2", len(runtime.Listeners))
	}
}

func TestValidateRejectsInvalidTrafficReset(t *testing.T) {
	cfg := RuntimeConfig{Listeners: []model.Listener{{
		ID: "listener-a", Enabled: true, BindPort: 4100, Transport: model.TransportUDP,
		TrafficReset: "sometimes",
	}}}
	err := ValidateForSave(cfg)
	if err == nil || !strings.Contains(err.Error(), "TrafficReset") {
		t.Fatalf("ValidateForSave() error = %v, want TrafficReset error", err)
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
			SendThrough:         "192.0.2.10",
			TargetStrategy:      "ForceIPv4",
			Network:             "tcp",
			Security:            "reality",
			StreamSettingsJSON:  `{"network":"tcp"}`,
		}},
		Settings: []model.Settings{{
			ID:                     "global",
			Enabled:                true,
			Name:                   "global",
			ExternalXrayPath:       "/usr/local/bin/xray",
			ExternalXrayConfigFile: "/var/lib/tapx/xray.json",
			ExternalXrayWorkDir:    "/var/lib/tapx",
			ExternalXrayArgs:       "run\n-config\n{config}",
			LogLevel:               "info",
			StatsIntervalSecond:    5,
			OpenWrtBuildTarget:     "x86-64",
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
	if runtime.XrayProfiles[0].SendThrough != "192.0.2.10" || runtime.XrayProfiles[0].TargetStrategy != "ForceIPv4" {
		t.Fatalf("runtime xray profile = %+v, want outbound top-level fields", runtime.XrayProfiles[0])
	}
	if len(runtime.Settings) != 1 || runtime.Settings[0].OpenWrtBuildTarget != "x86-64" {
		t.Fatalf("runtime settings = %+v, want x86-64 target", runtime.Settings)
	}
	if runtime.Settings[0].ExternalXrayConfigFile != "/var/lib/tapx/xray.json" || runtime.Settings[0].ExternalXrayWorkDir != "/var/lib/tapx" || runtime.Settings[0].ExternalXrayArgs == "" {
		t.Fatalf("runtime external xray settings = %+v", runtime.Settings[0])
	}
}

func TestGenerateRuntimeBuildsEmbeddedXrayPipeWhenDeviceBound(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:               "tun0",
			Enabled:          true,
			Type:             model.DeviceTUN,
			IfName:           "tapxxray0",
			MTU:              1400,
			LinkAutoOptimize: true,
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
	if !pipe.LinkAutoOptimize {
		t.Fatal("Xray pipe did not inherit automatic link optimization")
	}
	if pipe.LengthMode != model.TCPLength32 {
		t.Fatalf("xray pipe length mode = %q, want uint32", pipe.LengthMode)
	}
	if len(pipe.AddressGuard.IPv4CIDRs) != 1 || pipe.AddressGuard.IPv4CIDRs[0] != "10.79.0.2/32" {
		t.Fatalf("xray pipe address guard = %+v, want 10.79.0.2/32", pipe.AddressGuard)
	}
}

func TestGenerateRuntimeBuildsExternalFrameBridgeOnlyWhenDeviceBound(t *testing.T) {
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
	if len(runtime.XrayPipes) != 1 || runtime.XrayPipes[0].Runtime != model.XrayExternal {
		t.Fatalf("runtime xray policies = %+v, want one external policy", runtime.XrayPipes)
	}
	if len(runtime.TCPPipes) != 1 {
		t.Fatalf("runtime tcp pipes = %+v, want one external xray frame bridge", runtime.TCPPipes)
	}
	bridge := runtime.TCPPipes[0]
	if !bridge.ExternalXrayBridge || bridge.EndpointID != "external-bound" || bridge.EndpointKind != "listener" {
		t.Fatalf("external xray bridge = %+v, want external-bound listener", bridge)
	}
	if bridge.BindHost != "127.0.0.1" || bridge.BindPort != 0 || bridge.DeviceID != "tun0" {
		t.Fatalf("external xray bridge address/device = %+v, want dynamic loopback/tun0", bridge)
	}
	if bridge.XrayBridgeTag != runtime.XrayPipes[0].HandlerTag {
		t.Fatalf("external xray bridge tag = %q, want %q", bridge.XrayBridgeTag, runtime.XrayPipes[0].HandlerTag)
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

func TestValidateRejectsInvalidPanelHostPathCertificateAndTimezone(t *testing.T) {
	err := ValidateForSave(RuntimeConfig{Settings: []model.Settings{{
		ID:            "global",
		Enabled:       true,
		PanelDomain:   "https://panel.example.com:2053",
		PanelBasePath: "/missing-trailing-slash",
		PanelHTTPS:    true,
		Timezone:      "Mars/Olympus",
	}}})
	if err == nil {
		t.Fatal("ValidateForSave() error = nil, want panel setting problems")
	}
	for _, want := range []string{"PanelDomain", "PanelBasePath", "PanelHTTPS", "Timezone"} {
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

func TestValidatePanelOutboundRequiresEmbeddedXrayConnector(t *testing.T) {
	base := RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{ID: "embedded", Enabled: true, Runtime: model.XrayEmbedded}},
		Connectors:   []model.Connector{{ID: "xray-a", Enabled: true, Transport: model.TransportXray, XrayProfileID: "embedded"}},
		Settings:     []model.Settings{{ID: "global", Enabled: true, PanelOutbound: "xray-a"}},
	}
	if err := ValidateForSave(base); err != nil {
		t.Fatalf("valid panel outbound rejected: %v", err)
	}

	for name, mutate := range map[string]func(*RuntimeConfig){
		"missing": func(cfg *RuntimeConfig) { cfg.Settings[0].PanelOutbound = "missing" },
		"raw": func(cfg *RuntimeConfig) {
			cfg.Connectors[0].Transport = model.TransportTCP
		},
		"external": func(cfg *RuntimeConfig) {
			cfg.XrayProfiles[0].Runtime = model.XrayExternal
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			cfg.XrayProfiles = append([]model.XrayProfile(nil), base.XrayProfiles...)
			cfg.Connectors = append([]model.Connector(nil), base.Connectors...)
			cfg.Settings = append([]model.Settings(nil), base.Settings...)
			mutate(&cfg)
			if err := ValidateForSave(cfg); err == nil || !strings.Contains(err.Error(), "PanelOutbound") {
				t.Fatalf("ValidateForSave() error = %v, want PanelOutbound error", err)
			}
		})
	}
}

func TestValidateRejectsInvalidPanelListenAndCertificatePair(t *testing.T) {
	cfg := RuntimeConfig{Settings: []model.Settings{{
		ID:            "global",
		Enabled:       true,
		PanelListen:   "panel.example.com:0",
		PanelCertFile: "relative/cert.pem",
	}}}
	err := ValidateForSave(cfg)
	if err == nil {
		t.Fatal("ValidateForSave() error = nil")
	}
	for _, want := range []string{"PanelListen", "PanelCertFile"} {
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

func TestValidateAutomaticLinkRequiresDeviceMTU(t *testing.T) {
	err := ValidateForSave(RuntimeConfig{Devices: []model.Device{{
		ID: "tun0", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0", LinkAutoOptimize: true,
	}}})
	if err == nil || !strings.Contains(err.Error(), "MTU") {
		t.Fatalf("ValidateForSave() error = %v, want automatic-link MTU error", err)
	}
}

func TestValidateRejectsDisabledXrayQUICPathMTUWithAutomaticDevice(t *testing.T) {
	err := ValidateForSave(RuntimeConfig{
		Devices: []model.Device{{
			ID: "tun0", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0", MTU: 1500, LinkAutoOptimize: true,
		}},
		XrayProfiles: []model.XrayProfile{{
			ID: "xr1", Enabled: true, Runtime: model.XrayEmbedded, Network: "hysteria",
			InboundProtocol: "vless", OutboundProtocol: "vless",
			StreamSettingsJSON: `{"finalmask":{"quicParams":{"disablePathMTUDiscovery":true}}}`,
		}},
		Connectors: []model.Connector{{
			ID: "connector-xr", Enabled: true, Transport: model.TransportXray, XrayProfileID: "xr1",
			Binding: model.Binding{DeviceID: "tun0"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot disable QUIC path MTU discovery") {
		t.Fatalf("ValidateForSave() error = %v, want QUIC PMTU conflict", err)
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
	if runtime.UDPPipes[0].AddressGuardRemote {
		t.Fatal("ordinary device route was incorrectly compiled as a remote identity guard")
	}
}

func TestListenerAddressGuardRemoteSelection(t *testing.T) {
	idx := runtimeIndex(RuntimeConfig{
		Routes: []model.Route{
			{ID: "local", Enabled: true},
			{ID: "listener", Enabled: true, ListenerID: "listener-a"},
			{ID: "vkey", Enabled: true, VKeyID: "key-a"},
			{ID: "client", Enabled: true, ClientID: "client-a"},
		},
		VKeys: []model.VKey{{ID: "key-a", Enabled: true, Value: "secret"}},
	})
	tests := []struct {
		name    string
		binding RuntimeBinding
		want    bool
	}{
		{name: "ordinary route", binding: RuntimeBinding{RouteID: "local"}},
		{name: "listener route", binding: RuntimeBinding{RouteID: "listener"}, want: true},
		{name: "other listener", binding: RuntimeBinding{RouteID: "listener"}},
		{name: "vkey route", binding: RuntimeBinding{RouteID: "vkey"}, want: true},
		{name: "client route", binding: RuntimeBinding{RouteID: "client"}, want: true},
		{name: "direct vkey", binding: RuntimeBinding{VKeyValue: "secret"}, want: true},
		{name: "direct client", binding: RuntimeBinding{ClientID: "client-a"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenerID := "listener-a"
			if tt.name == "other listener" {
				listenerID = "listener-b"
			}
			if got := idx.listenerAddressGuardRemote(listenerID, tt.binding); got != tt.want {
				t.Fatalf("listenerAddressGuardRemote() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateRuntimeBuildsRawUDPUserDispatch(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{
			{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tun-a", MTU: 1400},
			{ID: "tun-b", Enabled: true, Type: model.DeviceTUN, IfName: "tun-b", MTU: 1400},
			{ID: "tun-default", Enabled: true, Type: model.DeviceTUN, IfName: "tun-default", MTU: 1400},
		},
		VKeys: []model.VKey{
			{ID: "key-a", Enabled: true, Value: "alpha"},
			{ID: "key-b", Enabled: true, Value: "bravo"},
			{ID: "key-drop", Enabled: true, Value: "blocked"},
		},
		Clients: []model.Client{{
			ID: "client-b", Enabled: true, ListenerID: "raw-in", AllowedDeviceIDs: []string{"tun-b"},
			Binding: model.Binding{VKeyID: "key-b", DeviceID: "tun-b"},
		}},
		Routes: []model.Route{
			{ID: "route-a", Enabled: true, Priority: 10, Action: model.RouteActionBindDevice, ListenerID: "raw-in", VKeyID: "key-a", DeviceID: "tun-a"},
			{ID: "route-drop", Enabled: true, Priority: 20, Action: model.RouteActionDrop, ListenerID: "raw-in", VKeyID: "key-drop"},
		},
		Listeners: []model.Listener{{
			ID: "raw-in", Enabled: true, BindHost: "127.0.0.1", BindPort: 44000, Transport: model.TransportUDP,
			Binding: model.Binding{DeviceID: "tun-default"},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.UDPPipes) != 3 || len(runtime.UDPDispatches) != 1 {
		t.Fatalf("raw UDP runtime = pipes:%d dispatches:%d, want 3/1", len(runtime.UDPPipes), len(runtime.UDPDispatches))
	}
	dispatch := runtime.UDPDispatches[0]
	if dispatch.FallbackSocketIndex != 3 {
		t.Fatalf("fallback socket = %d, want 3", dispatch.FallbackSocketIndex)
	}
	want := map[string]uint32{"alpha": 1, "bravo": 2, "blocked": 0}
	for _, route := range dispatch.Routes {
		if want[route.VKeyValue] != route.SocketIndex {
			t.Fatalf("dispatch route %+v, want socket %d", route, want[route.VKeyValue])
		}
		delete(want, route.VKeyValue)
	}
	if len(want) != 0 {
		t.Fatalf("missing dispatch routes: %+v", want)
	}
	for index, pipe := range runtime.UDPPipes {
		if pipe.DispatchGroup != dispatch.ID || pipe.DispatchSocketIndex != uint32(index+1) || !pipe.ReusePort {
			t.Fatalf("pipe %d dispatch metadata = %+v", index, pipe)
		}
	}
}

func TestGenerateRuntimeBuildsRawTCPUserDispatch(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{
			{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tun-a", MTU: 1400},
			{ID: "tun-b", Enabled: true, Type: model.DeviceTUN, IfName: "tun-b", MTU: 1400},
			{ID: "tun-default", Enabled: true, Type: model.DeviceTUN, IfName: "tun-default", MTU: 1400},
		},
		VKeys: []model.VKey{
			{ID: "key-a", Enabled: true, Value: "alpha"},
			{ID: "key-b", Enabled: true, Value: "bravo"},
			{ID: "key-drop", Enabled: true, Value: "blocked"},
		},
		Clients: []model.Client{{
			ID: "client-b", Enabled: true, ListenerID: "raw-in", AllowedDeviceIDs: []string{"tun-b"},
			Binding: model.Binding{VKeyID: "key-b", DeviceID: "tun-b"},
		}},
		Routes: []model.Route{
			{ID: "route-a", Enabled: true, Priority: 10, Action: model.RouteActionBindDevice, ListenerID: "raw-in", VKeyID: "key-a", DeviceID: "tun-a"},
			{ID: "route-drop", Enabled: true, Priority: 20, Action: model.RouteActionDrop, ListenerID: "raw-in", VKeyID: "key-drop"},
		},
		Listeners: []model.Listener{{
			ID: "raw-in", Enabled: true, BindHost: "127.0.0.1", BindPort: 44000, Transport: model.TransportTCP,
			Binding: model.Binding{DeviceID: "tun-default"},
		}},
	}

	runtime, err := GenerateRuntime(cfg)
	if err != nil {
		t.Fatalf("GenerateRuntime() error = %v", err)
	}
	if len(runtime.TCPPipes) != 3 || len(runtime.TCPDispatches) != 1 {
		t.Fatalf("raw TCP runtime = pipes:%d dispatches:%d, want 3/1", len(runtime.TCPPipes), len(runtime.TCPDispatches))
	}
	dispatch := runtime.TCPDispatches[0]
	if dispatch.FallbackPolicyID == "" {
		t.Fatal("fallback policy is empty")
	}
	want := map[string]string{"blocked": ""}
	for _, pipe := range runtime.TCPPipes {
		if pipe.DispatchGroup != dispatch.ID || pipe.DispatchPolicyID == "" {
			t.Fatalf("pipe dispatch metadata = %+v", pipe)
		}
		if pipe.Binding.VKeyValue == "" {
			if dispatch.FallbackPolicyID != pipe.DispatchPolicyID {
				t.Fatalf("fallback policy = %q, want %q", dispatch.FallbackPolicyID, pipe.DispatchPolicyID)
			}
		} else {
			want[pipe.Binding.VKeyValue] = pipe.DispatchPolicyID
		}
	}
	for _, route := range dispatch.Routes {
		if want[route.VKeyValue] != route.PolicyID {
			t.Fatalf("dispatch route %+v, want policy %q", route, want[route.VKeyValue])
		}
		delete(want, route.VKeyValue)
	}
	if len(want) != 0 {
		t.Fatalf("missing dispatch routes: %+v", want)
	}
}

func TestGenerateRuntimeBuildsTCPPipe(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID:               "tun0",
			Enabled:          true,
			Name:             "tun0",
			Type:             model.DeviceTUN,
			IfName:           "tun0",
			MTU:              1400,
			LinkAutoOptimize: true,
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
				ReconnectSecond: 2,
				QueueSize:       4096,
				ZeroCopy:        true,
				IdleTimeout:     90,
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
	if !pipe.NoDelay || pipe.KeepAliveSecond != 30 || !pipe.FastOpen || pipe.ConnectTimeout != 3 || pipe.ReconnectSecond != 2 {
		t.Fatalf("pipe tcp flags = nodelay:%v keepalive:%d fastopen:%v timeout:%d reconnect:%d", pipe.NoDelay, pipe.KeepAliveSecond, pipe.FastOpen, pipe.ConnectTimeout, pipe.ReconnectSecond)
	}
	if pipe.QueueSize != 4096 || !pipe.ZeroCopy || pipe.IdleTimeout != 90 {
		t.Fatalf("pipe fastpath controls = %+v, want queue/zero-copy/idle timeout", pipe)
	}
	if !pipe.TLS.Enabled || pipe.TLS.ServerName != "tapx.example" || len(pipe.TLS.ALPN) != 1 || pipe.TLS.ALPN[0] != "tapx" {
		t.Fatalf("pipe tls = %+v, want copied TLS settings", pipe.TLS)
	}
	if pipe.MaxFrameSize != 1400 {
		t.Fatalf("pipe max frame size = %d, want 1400", pipe.MaxFrameSize)
	}
	if pipe.TCPMaxSeg != 1360 {
		t.Fatalf("pipe TCP maximum segment size = %d, want 1360", pipe.TCPMaxSeg)
	}
	if pipe.DeviceID != "tun0" {
		t.Fatalf("pipe device = %q, want tun0", pipe.DeviceID)
	}
}

func TestAutomaticTCPMaxSeg(t *testing.T) {
	tests := []struct {
		name   string
		device model.Device
		outer  string
		want   int
	}{
		{name: "disabled", device: model.Device{MTU: 1500}, outer: "192.0.2.1", want: 0},
		{name: "IPv4", device: model.Device{MTU: 1500, LinkAutoOptimize: true}, outer: "192.0.2.1", want: 1460},
		{name: "IPv6", device: model.Device{MTU: 1500, LinkAutoOptimize: true}, outer: "2001:db8::1", want: 1440},
		{name: "unresolved host", device: model.Device{MTU: 1500, LinkAutoOptimize: true}, outer: "tapx.example", want: 1440},
		{name: "invalid small MTU", device: model.Device{MTU: 500, LinkAutoOptimize: true}, outer: "192.0.2.1", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := automaticTCPMaxSeg(tt.device, tt.outer); got != tt.want {
				t.Fatalf("automaticTCPMaxSeg() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGenerateRuntimeInheritsExplicitClientBinding(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{
			ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-a", MTU: 1500,
		}},
		VKeys: []model.VKey{{ID: "vkey-a", Enabled: true, Value: "client-secret"}},
		Addresses: []model.AddressLimit{{
			ID: "addr-a", Enabled: true, DeviceID: "tun-a", ClientID: "client-a",
			IPv4CIDRs: []string{"10.40.0.2/32"},
		}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, ListenerIDs: []string{"listener-a"},
			AllowedDeviceIDs: []string{"tun-a"}, AddressID: "addr-a",
			Binding: model.Binding{VKeyID: "vkey-a", DeviceID: "tun-a", AddressID: "addr-a"},
		}},
		Listeners: []model.Listener{{
			ID: "listener-a", Enabled: true, BindPort: 44000, Transport: model.TransportUDP,
			Binding: model.Binding{ClientID: "client-a"},
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
	if pipe.DeviceID != "tun-a" || pipe.Binding.ClientID != "client-a" || pipe.Binding.VKeyValue != "client-secret" || pipe.Binding.AddressID != "addr-a" {
		t.Fatalf("pipe binding = %+v, device = %q", pipe.Binding, pipe.DeviceID)
	}
	if len(pipe.AddressGuard.IPv4CIDRs) != 1 || pipe.AddressGuard.IPv4CIDRs[0] != "10.40.0.2/32" {
		t.Fatalf("pipe address guard = %+v", pipe.AddressGuard)
	}
}

func TestValidateRejectsEndpointOutsideClientAllowedDevices(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{
			{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-a", MTU: 1500},
			{ID: "tun-b", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-b", MTU: 1500},
		},
		Clients: []model.Client{{ID: "client-a", Enabled: true, AllowedDeviceIDs: []string{"tun-a"}}},
		Listeners: []model.Listener{{
			ID: "listener-a", Enabled: true, BindPort: 44000, Transport: model.TransportUDP,
			Binding: model.Binding{ClientID: "client-a", DeviceID: "tun-b"},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil || !strings.Contains(err.Error(), "AllowedDeviceIDs") {
		t.Fatalf("ValidateForSave() error = %v, want client device restriction error", err)
	}
}

func TestValidateRejectsClientOnUnassignedListener(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-a", MTU: 1500}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, ListenerIDs: []string{"listener-a"},
			Binding: model.Binding{DeviceID: "tun-a"},
		}},
		Listeners: []model.Listener{
			{ID: "listener-a", Enabled: true, BindPort: 44000, Transport: model.TransportUDP, Binding: model.Binding{DeviceID: "tun-a"}},
			{ID: "listener-b", Enabled: true, BindPort: 44001, Transport: model.TransportUDP, Binding: model.Binding{ClientID: "client-a"}},
		},
	}

	err := ValidateForSave(cfg)
	if err == nil || !strings.Contains(err.Error(), "not assigned to this listener") {
		t.Fatalf("ValidateForSave() error = %v, want listener assignment error", err)
	}
}

func TestValidateRejectsEndpointAddressThatBypassesClientLimit(t *testing.T) {
	cfg := RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx-a", MTU: 1500}},
		Addresses: []model.AddressLimit{
			{ID: "addr-client", Enabled: true, DeviceID: "tun-a", ClientID: "client-a", IPv4CIDRs: []string{"10.40.0.2/32"}},
			{ID: "addr-endpoint", Enabled: true, DeviceID: "tun-a", IPv4CIDRs: []string{"10.40.0.3/32"}},
		},
		Clients: []model.Client{{ID: "client-a", Enabled: true, AddressID: "addr-client"}},
		Listeners: []model.Listener{{
			ID: "listener-a", Enabled: true, BindPort: 44000, Transport: model.TransportUDP,
			Binding: model.Binding{ClientID: "client-a", DeviceID: "tun-a", AddressID: "addr-endpoint"},
		}},
	}

	err := ValidateForSave(cfg)
	if err == nil || !strings.Contains(err.Error(), "conflicts with referenced client binding") {
		t.Fatalf("ValidateForSave() error = %v, want client address conflict", err)
	}
}
