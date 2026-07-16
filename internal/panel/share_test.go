package panel

import (
	"strings"
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestBuildClientShareRawTapXLink(t *testing.T) {
	cfg := config.RuntimeConfig{
		Devices: []model.Device{{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0", MTU: 1400, IPv4CIDR: "10.30.0.1/30"}},
		VKeys:   []model.VKey{{ID: "vk-a", Enabled: true, Name: "Raw Key", Value: "secret-vkey"}},
		Addresses: []model.AddressLimit{{
			ID: "addr-a", Enabled: true, DeviceID: "tun-a", ClientID: "client-a",
			IPv4CIDRs:         []string{"10.30.0.2/32"},
			IPv4Gateway:       "10.30.0.1",
			DNS:               []string{"1.1.1.1"},
			Routes:            []string{"10.40.0.0/24"},
			AllowDefaultRoute: true,
		}},
		Connectors: []model.Connector{{
			ID: "udp-out", Enabled: true, Remote: "198.51.100.20", Port: 46000, Transport: model.TransportUDP,
		}},
		Routes: []model.Route{{
			ID: "route-a", Enabled: true, DeviceID: "tun-a", ConnectorID: "udp-out", VKeyID: "vk-a", AddressID: "addr-a", ClientID: "client-a",
		}},
		Listeners: []model.Listener{{
			ID: "udp-in", Enabled: true, BindHost: "0.0.0.0", BindPort: 46000, Transport: model.TransportUDP,
		}},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, Name: "Alice", Email: "alice@example.test", ListenerID: "udp-in",
			CredentialType: "vkey", CredentialValue: "secret-vkey", Binding: model.Binding{RouteID: "route-a"},
			AddressID: "addr-a", TrafficCap: 1024, UploadRateLimit: 3_000_000, DownloadRateLimit: 5_000_000,
		}},
	}

	share, err := BuildClientShare(cfg, "client-a")
	if err != nil {
		t.Fatalf("BuildClientShare() error = %v", err)
	}
	if share.Type != "tapx" || !strings.HasPrefix(share.Link, "raw://server.example:46000?") {
		t.Fatalf("share link = %q type=%q, want raw", share.Link, share.Type)
	}
	if !strings.Contains(share.Link, "network=udp") || !strings.Contains(share.Link, "vkey=secret-vkey") {
		t.Fatalf("raw link misses transport or vkey: %q", share.Link)
	}
	if len(share.Links) != 1 || share.Links[0] != share.Link {
		t.Fatalf("share links = %v, want primary link", share.Links)
	}
	if share.Objects.RouteID != "route-a" || share.Objects.ConnectorID != "udp-out" || share.Objects.AddressID != "addr-a" || share.Objects.VKeyID != "vk-a" {
		t.Fatalf("objects = %+v, want route/connector/address/vkey", share.Objects)
	}
	if share.Payload.VKey == nil || share.Payload.VKey.Value != "secret-vkey" {
		t.Fatalf("payload vkey = %+v", share.Payload.VKey)
	}
	if share.Payload.AddressLimit == nil || share.Payload.AddressLimit.IPv4CIDRs[0] != "10.30.0.2/32" || share.Payload.AddressLimit.IPv4Gateway != "10.30.0.1" {
		t.Fatalf("payload address = %+v", share.Payload.AddressLimit)
	}
	if !share.Payload.AddressLimit.AllowDefaultRoute || share.Payload.AddressLimit.DNS[0] != "1.1.1.1" || share.Payload.AddressLimit.Routes[0] != "10.40.0.0/24" {
		t.Fatalf("payload address static config = %+v", share.Payload.AddressLimit)
	}

	if share.Payload.Client.ID != "client-a" || share.Payload.Connector == nil || share.Payload.Connector.Remote != "198.51.100.20" {
		t.Fatalf("share payload = %+v", share.Payload)
	}
	if share.Payload.Client.UploadRateLimit != 3_000_000 || share.Payload.Client.DownloadRateLimit != 5_000_000 {
		t.Fatalf("share rate limits = (%d, %d)", share.Payload.Client.UploadRateLimit, share.Payload.Client.DownloadRateLimit)
	}
}

func TestBuildClientShareVLESSLink(t *testing.T) {
	cfg := config.RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID: "xr-vless", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "vless", Network: "tcp", Security: "none",
			InboundSettingsJSON: `{"clients":[{"id":"11111111-1111-4111-8111-111111111111","flow":"xtls-rprx-vision"}],"decryption":"none"}`,
		}},
		Listeners: []model.Listener{{
			ID: "xray-in", Enabled: true, BindHost: "203.0.113.10", BindPort: 443, Transport: model.TransportXray, XrayProfileID: "xr-vless",
		}},
		Clients: []model.Client{{
			ID: "client-x", Enabled: true, Name: "Xray User", ListenerID: "xray-in",
			CredentialType: "uuid", CredentialValue: "11111111-1111-4111-8111-111111111111",
		}},
	}

	share, err := BuildClientShare(cfg, "client-x")
	if err != nil {
		t.Fatalf("BuildClientShare() error = %v", err)
	}
	if share.Type != "xray" || !strings.HasPrefix(share.Link, "vless://11111111-1111-4111-8111-111111111111@203.0.113.10:443") {
		t.Fatalf("share link = %q type=%q, want vless", share.Link, share.Type)
	}
	if !strings.Contains(share.Link, "security=none") || !strings.Contains(share.Link, "type=tcp") {
		t.Fatalf("vless link missing query settings: %s", share.Link)
	}
	if !strings.Contains(share.Link, "flow=xtls-rprx-vision") {
		t.Fatalf("vless link did not read flow from the Xray profile: %s", share.Link)
	}
	if len(share.Links) != 1 || share.Links[0] != share.Link {
		t.Fatalf("share links = %v, want primary link", share.Links)
	}
}

func TestBuildClientShareCreatesOneLinkPerListener(t *testing.T) {
	cfg := config.RuntimeConfig{
		Listeners: []model.Listener{
			{ID: "tcp-in", Enabled: true, BindHost: "198.51.100.10", BindPort: 41000, Transport: model.TransportTCP},
			{ID: "udp-in", Enabled: true, BindHost: "198.51.100.10", BindPort: 42000, Transport: model.TransportUDP},
		},
		Clients: []model.Client{{
			ID: "client-raw", Enabled: true, Name: "Raw User", ListenerID: "tcp-in",
			ListenerIDs: []string{"tcp-in", "udp-in"}, CredentialType: "vkey", CredentialValue: "raw-secret",
		}},
	}

	share, err := BuildClientShare(cfg, "client-raw")
	if err != nil {
		t.Fatalf("BuildClientShare() error = %v", err)
	}
	if len(share.Links) != 2 {
		t.Fatalf("share links = %v, want two", share.Links)
	}
	if !strings.HasPrefix(share.Links[0], "raw://198.51.100.10:41000?") || !strings.Contains(share.Links[0], "network=tcp") {
		t.Fatalf("tcp share link = %q", share.Links[0])
	}
	if !strings.HasPrefix(share.Links[1], "raw://198.51.100.10:42000?") || !strings.Contains(share.Links[1], "network=udp") {
		t.Fatalf("udp share link = %q", share.Links[1])
	}
	for _, link := range share.Links {
		if !strings.Contains(link, "vkey=raw-secret") {
			t.Fatalf("raw share link misses vkey: %q", link)
		}
	}
}

func TestBuildClientShareIgnoresUnrelatedInvalidObjects(t *testing.T) {
	cfg := config.RuntimeConfig{
		Listeners: []model.Listener{
			{ID: "raw-in", Enabled: true, BindHost: "203.0.113.8", BindPort: 45000, Transport: model.TransportUDP},
			{ID: "broken-xray", Enabled: true, BindPort: 45001, Transport: model.TransportXray},
		},
		Clients: []model.Client{{
			ID: "client-raw", Enabled: true, ListenerID: "raw-in", CredentialType: "raw-udp",
		}},
	}

	share, err := BuildClientShare(cfg, "client-raw")
	if err != nil {
		t.Fatalf("BuildClientShare() error = %v", err)
	}
	if !strings.HasPrefix(share.Link, "raw://203.0.113.8:45000?") {
		t.Fatalf("share link = %q", share.Link)
	}
}

func TestBuildClientShareRejectsUserLevelWireguardCredentials(t *testing.T) {
	cfg := config.RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID: "wg-profile", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "wireguard",
			InboundSettingsJSON: `{"secretKey":"dwdtCnMYpX08FsFyUbJmRd9ML4frwJkqsXf7pR25LCo=","mtu":1420}`,
		}},
		Listeners: []model.Listener{{
			ID: "wg-in", Enabled: true, BindHost: "203.0.113.20", BindPort: 51820,
			Transport: model.TransportXray, XrayProfileID: "wg-profile",
		}},
		Clients: []model.Client{{
			ID: "wg-client", Enabled: true, ListenerID: "wg-in",
		}},
	}

	if _, err := BuildClientShare(cfg, "wg-client"); err == nil || !strings.Contains(err.Error(), "Xray profile") {
		t.Fatalf("BuildClientShare() error = %v, want user-level WireGuard rejection", err)
	}
}

func TestBuildClientShareUsesCustomListenerShareAddress(t *testing.T) {
	cfg := config.RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID: "xr-vless", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "vless", Network: "tcp", Security: "none",
			InboundSettingsJSON: `{}`,
		}},
		Listeners: []model.Listener{{
			ID: "xray-in", Enabled: true, BindHost: "0.0.0.0", BindPort: 443, Transport: model.TransportXray, XrayProfileID: "xr-vless",
			ShareAddressStrategy: "custom", ShareAddress: "edge.example.com",
		}},
		Clients: []model.Client{{
			ID: "client-x", Enabled: true, ListenerID: "xray-in", CredentialType: "uuid",
			CredentialValue: "11111111-1111-4111-8111-111111111111",
		}},
	}

	share, err := BuildClientShare(cfg, "client-x")
	if err != nil {
		t.Fatalf("BuildClientShare() error = %v", err)
	}
	if !strings.Contains(share.Link, "@edge.example.com:443") {
		t.Fatalf("share link = %q, want custom share address", share.Link)
	}
	if len(share.Warnings) != 0 {
		t.Fatalf("share warnings = %v, want none", share.Warnings)
	}
}
