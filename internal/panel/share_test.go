package panel

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
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
			AddressID: "addr-a", TrafficCap: 1024,
		}},
	}

	share, err := BuildClientShare(cfg, "client-a")
	if err != nil {
		t.Fatalf("BuildClientShare() error = %v", err)
	}
	if share.Type != "tapx" || !strings.HasPrefix(share.Link, "tapx://client/gzip/") {
		t.Fatalf("share link = %q type=%q, want tapx", share.Link, share.Type)
	}
	if !strings.HasPrefix(share.QRPNG, "data:image/png;base64,") {
		t.Fatalf("qr png prefix = %q", share.QRPNG[:min(len(share.QRPNG), 32)])
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

	rawToken := strings.TrimPrefix(share.Link, "tapx://client/gzip/")
	compressed, err := base64.RawURLEncoding.DecodeString(rawToken)
	if err != nil {
		t.Fatalf("decode tapx link: %v", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("open gzip payload: %v", err)
	}
	rawJSON, err := io.ReadAll(zr)
	if closeErr := zr.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read gzip payload: %v", err)
	}
	var payload SharePayload
	if err := json.Unmarshal(rawJSON, &payload); err != nil {
		t.Fatalf("unmarshal tapx payload: %v", err)
	}
	if payload.Client.ID != "client-a" || payload.Connector == nil || payload.Connector.Remote != "198.51.100.20" {
		t.Fatalf("decoded payload = %+v", payload)
	}
}

func TestBuildClientShareVLESSLink(t *testing.T) {
	cfg := config.RuntimeConfig{
		XrayProfiles: []model.XrayProfile{{
			ID: "xr-vless", Enabled: true, Runtime: model.XrayEmbedded, InboundProtocol: "vless", Network: "tcp", Security: "none",
			InboundSettingsJSON: `{"clients":[],"decryption":"none"}`,
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
	if !strings.HasPrefix(share.QRPNG, "data:image/png;base64,") {
		t.Fatalf("qr png prefix = %q", share.QRPNG[:min(len(share.QRPNG), 32)])
	}
}
