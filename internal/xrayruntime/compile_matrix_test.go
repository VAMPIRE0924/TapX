package xrayruntime

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	xraycore "github.com/xtls/xray-core/core"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestCurrentWebInboundProtocolsLoadInOfficialXray(t *testing.T) {
	tests := []struct {
		protocol string
		settings string
	}{
		{"vless", `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}],"decryption":"none"}`},
		{"vmess", `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}]}`},
		{"trojan", `{"clients":[{"password":"secret"}]}`},
		{"shadowsocks", `{"method":"2022-blake3-aes-256-gcm","password":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=","network":"tcp,udp","clients":[]}`},
		{"hysteria", `{"version":2,"clients":[{"auth":"secret"}]}`},
		{"http", `{"accounts":[{"user":"tapx","pass":"secret"}],"allowTransparent":false}`},
		{"mixed", `{"auth":"password","accounts":[{"user":"tapx","pass":"secret"}],"udp":false,"ip":"127.0.0.1"}`},
		{"tunnel", `{"portMap":{},"allowedNetwork":"tcp,udp","followRedirect":false}`},
		{"tun", `{"name":"xray0","mtu":1500,"gateway":[],"dns":[],"userLevel":0,"autoSystemRoutingTable":[],"autoOutboundsInterface":"auto"}`},
		{"wireguard", `{"mtu":1420,"secretKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","peers":[],"clients":[],"noKernelTun":true}`},
	}

	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			runtime := &config.GeneratedRuntime{
				XrayProfiles: []config.RuntimeXrayProfile{{
					ID: "profile", Runtime: model.XrayEmbedded,
					InboundProtocol: test.protocol, InboundSettingsJSON: test.settings,
				}},
				Listeners: []config.RuntimeEndpoint{{
					ID: "listener", Transport: model.TransportXray, BindHost: "127.0.0.1", BindPort: 18443, XrayProfileID: "profile",
				}},
			}
			compiled, err := Compile(runtime)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			loadOfficialXrayDocument(t, compiled.EmbeddedDocument)
		})
	}
}

func TestCurrentOfficialXrayRejectsRemovedMtprotoInbound(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{{
			ID: "profile", Runtime: model.XrayEmbedded,
			InboundProtocol: "mtproto", InboundSettingsJSON: `{"secret":"dd00000000000000000000000000000000"}`,
		}},
		Listeners: []config.RuntimeEndpoint{{
			ID: "listener", Transport: model.TransportXray, BindHost: "127.0.0.1", BindPort: 18443, XrayProfileID: "profile",
		}},
	}
	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	payload, err := json.Marshal(compiled.EmbeddedDocument)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := xraycore.LoadConfig("json", bytes.NewReader(payload)); err == nil {
		t.Fatal("current official Xray unexpectedly accepted the removed mtproto inbound")
	}
}

func TestCurrentWebTransportPairsLoadInOfficialXray(t *testing.T) {
	tests := []struct {
		network        string
		inboundStream  string
		outboundStream string
	}{
		{"tcp", `{"tcpSettings":{"acceptProxyProtocol":false,"header":{"type":"none"}}}`, `{"tcpSettings":{"header":{"type":"none"}}}`},
		{"kcp", `{"kcpSettings":{"mtu":1350,"tti":20,"uplinkCapacity":5,"downlinkCapacity":20,"cwndMultiplier":1,"maxSendingWindow":2097152}}`, `{"kcpSettings":{"mtu":1350,"tti":20,"uplinkCapacity":5,"downlinkCapacity":20,"cwndMultiplier":1,"maxSendingWindow":2097152}}`},
		{"ws", `{"wsSettings":{"acceptProxyProtocol":false,"path":"/tapx","host":"","headers":{},"heartbeatPeriod":0}}`, `{"wsSettings":{"path":"/tapx","host":"","headers":{},"heartbeatPeriod":0}}`},
		{"grpc", `{"grpcSettings":{"serviceName":"tapx.grpc","authority":"","multiMode":false}}`, `{"grpcSettings":{"serviceName":"tapx.grpc","authority":"","multiMode":false}}`},
		{"httpupgrade", `{"httpupgradeSettings":{"acceptProxyProtocol":false,"path":"/tapx","host":"","headers":{}}}`, `{"httpupgradeSettings":{"path":"/tapx","host":"","headers":{}}}`},
		{"xhttp", `{"xhttpSettings":{"path":"/tapx","host":"","mode":"auto","xPaddingBytes":"100-1000","xPaddingObfsMode":false,"scMaxBufferedPosts":30,"serverMaxHeaderBytes":0,"headers":{},"noSSEHeader":false,"noGRPCHeader":false}}`, `{"xhttpSettings":{"path":"/tapx","mode":"auto","xPaddingBytes":"100-1000","headers":[]}}`},
	}

	for _, test := range tests {
		t.Run(test.network, func(t *testing.T) {
			inboundStream := mustMergeJSONObjects(t, test.inboundStream, `{"network":"`+test.network+`","security":"none"}`)
			outboundStream := mustMergeJSONObjects(t, test.outboundStream, `{"network":"`+test.network+`","security":"none"}`)
			runtime := &config.GeneratedRuntime{
				XrayProfiles: []config.RuntimeXrayProfile{
					{ID: "in", Runtime: model.XrayEmbedded, InboundProtocol: "vless", InboundSettingsJSON: `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}],"decryption":"none"}`, Network: test.network, Security: "none", StreamSettingsJSON: inboundStream},
					{ID: "out", Runtime: model.XrayEmbedded, OutboundProtocol: "vless", OutboundSettingsJSON: `{"address":"192.0.2.10","port":443,"id":"11111111-1111-4111-8111-111111111111","encryption":"none"}`, Network: test.network, Security: "none", StreamSettingsJSON: outboundStream},
				},
				Listeners:  []config.RuntimeEndpoint{{ID: "listener", Transport: model.TransportXray, BindHost: "127.0.0.1", BindPort: 18443, XrayProfileID: "in"}},
				Connectors: []config.RuntimeEndpoint{{ID: "connector", Transport: model.TransportXray, Remote: "192.0.2.10", Port: 443, XrayProfileID: "out"}},
			}
			compiled, err := Compile(runtime)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			loadOfficialXrayDocument(t, compiled.EmbeddedDocument)
		})
	}
}

func TestCurrentWebTLSCombinationsLoadInOfficialXray(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	protocols := []struct {
		name     string
		inbound  string
		outbound string
	}{
		{"vless", `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}],"decryption":"none"}`, `{"address":"192.0.2.10","port":443,"id":"11111111-1111-4111-8111-111111111111","encryption":"none"}`},
		{"vmess", `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}]}`, `{"vnext":[{"address":"192.0.2.10","port":443,"users":[{"id":"11111111-1111-4111-8111-111111111111","security":"auto"}]}]}`},
		{"trojan", `{"clients":[{"password":"secret"}]}`, `{"servers":[{"address":"192.0.2.10","port":443,"password":"secret"}]}`},
		{"shadowsocks", `{"method":"2022-blake3-aes-256-gcm","password":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=","network":"tcp,udp","clients":[]}`, `{"servers":[{"address":"192.0.2.10","port":443,"password":"MDEyMzQ1Njc4OWFiY2RlZg==","method":"2022-blake3-aes-128-gcm","uot":false,"UoTVersion":1}]}`},
	}
	networks := []string{"tcp", "ws", "grpc", "httpupgrade", "xhttp"}

	for _, protocol := range protocols {
		for _, network := range networks {
			t.Run(protocol.name+"/"+network, func(t *testing.T) {
				inboundStream := currentWebStreamSettings(network, true)
				inboundStream["security"] = "tls"
				inboundStream["tlsSettings"] = map[string]any{
					"serverName": "tapx.example", "minVersion": "1.2", "maxVersion": "1.3",
					"certificates": []any{map[string]any{"certificateFile": certFile, "keyFile": keyFile}},
					"alpn":         []any{"h2", "http/1.1"},
				}
				outboundStream := currentWebStreamSettings(network, false)
				outboundStream["security"] = "tls"
				outboundStream["tlsSettings"] = map[string]any{"serverName": "tapx.example"}
				runtime := pairedRuntime(t, protocol.name, protocol.inbound, protocol.outbound, network, inboundStream, outboundStream)
				compiled, err := Compile(runtime)
				if err != nil {
					t.Fatalf("Compile() error = %v", err)
				}
				loadOfficialXrayDocument(t, compiled.EmbeddedDocument)
			})
		}
	}
}

func TestCurrentWebHysteriaTLSLoadsInOfficialXray(t *testing.T) {
	certFile, keyFile := writeTestCertificate(t)
	inboundStream := map[string]any{
		"network": "hysteria", "security": "tls",
		"hysteriaSettings": map[string]any{"version": 2, "udpIdleTimeout": 60},
		"tlsSettings": map[string]any{
			"certificates": []any{map[string]any{"certificateFile": certFile, "keyFile": keyFile}},
			"alpn":         []any{"h3"},
		},
		"finalmask": map[string]any{"udp": []any{map[string]any{"type": "salamander", "settings": map[string]any{"password": "tapx-secret"}}}},
	}
	outboundStream := map[string]any{
		"network": "hysteria", "security": "tls",
		"hysteriaSettings": map[string]any{"version": 2, "auth": "secret", "udpIdleTimeout": 60},
		"tlsSettings":      map[string]any{"serverName": "tapx.example", "alpn": []any{"h3"}},
		"finalmask":        map[string]any{"udp": []any{map[string]any{"type": "salamander", "settings": map[string]any{"password": "tapx-secret"}}}},
	}
	runtime := pairedRuntime(t, "hysteria", `{"version":2,"clients":[{"auth":"secret"}]}`, `{"address":"192.0.2.10","port":443,"version":2}`, "hysteria", inboundStream, outboundStream)
	compiled, err := Compile(runtime)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	loadOfficialXrayDocument(t, compiled.EmbeddedDocument)
}

func TestCurrentWebRealityCombinationsLoadInOfficialXray(t *testing.T) {
	privateKey, publicKey := realityKeypair(t)
	protocols := []struct {
		name     string
		inbound  string
		outbound string
	}{
		{"vless", `{"clients":[{"id":"11111111-1111-4111-8111-111111111111"}],"decryption":"none"}`, `{"address":"192.0.2.10","port":443,"id":"11111111-1111-4111-8111-111111111111","encryption":"none"}`},
		{"trojan", `{"clients":[{"password":"secret"}]}`, `{"servers":[{"address":"192.0.2.10","port":443,"password":"secret"}]}`},
	}
	for _, protocol := range protocols {
		for _, network := range []string{"tcp", "grpc", "xhttp"} {
			t.Run(protocol.name+"/"+network, func(t *testing.T) {
				inboundStream := currentWebStreamSettings(network, true)
				inboundStream["security"] = "reality"
				inboundStream["realitySettings"] = map[string]any{
					"show": false, "xver": 0, "target": "www.cloudflare.com:443",
					"serverNames": []any{"www.cloudflare.com"}, "privateKey": privateKey,
					"shortIds": []any{"0123456789abcdef"},
				}
				outboundStream := currentWebStreamSettings(network, false)
				outboundStream["security"] = "reality"
				outboundStream["realitySettings"] = map[string]any{
					"publicKey": publicKey, "fingerprint": "chrome", "serverName": "www.cloudflare.com", "shortId": "0123456789abcdef", "spiderX": "/",
				}
				runtime := pairedRuntime(t, protocol.name, protocol.inbound, protocol.outbound, network, inboundStream, outboundStream)
				compiled, err := Compile(runtime)
				if err != nil {
					t.Fatalf("Compile() error = %v", err)
				}
				loadOfficialXrayDocument(t, compiled.EmbeddedDocument)
			})
		}
	}
}

func TestCurrentWebOutboundProtocolsLoadInOfficialXray(t *testing.T) {
	tests := []struct {
		protocol string
		settings string
	}{
		{"vmess", `{"vnext":[{"address":"192.0.2.10","port":443,"users":[{"id":"11111111-1111-4111-8111-111111111111","security":"auto"}]}]}`},
		{"vless", `{"address":"192.0.2.10","port":443,"id":"11111111-1111-4111-8111-111111111111","flow":"","encryption":"none"}`},
		{"trojan", `{"servers":[{"address":"192.0.2.10","port":443,"password":"secret"}]}`},
		{"shadowsocks", `{"servers":[{"address":"192.0.2.10","port":443,"password":"MDEyMzQ1Njc4OWFiY2RlZg==","method":"2022-blake3-aes-128-gcm","uot":false,"UoTVersion":1}]}`},
		{"socks", `{"servers":[{"address":"192.0.2.10","port":1080,"users":[{"user":"tapx","pass":"secret"}]}]}`},
		{"http", `{"servers":[{"address":"192.0.2.10","port":8080,"users":[{"user":"tapx","pass":"secret"}]}]}`},
		{"wireguard", `{"secretKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","address":["10.0.0.2/32"],"peers":[],"noKernelTun":true}`},
		{"hysteria", `{"address":"192.0.2.10","port":443,"version":2}`},
		{"freedom", `{}`},
		{"blackhole", `{}`},
		{"dns", `{}`},
		{"loopback", `{}`},
	}

	for _, test := range tests {
		t.Run(test.protocol, func(t *testing.T) {
			runtime := &config.GeneratedRuntime{
				XrayProfiles: []config.RuntimeXrayProfile{{
					ID: "profile", Runtime: model.XrayEmbedded,
					OutboundProtocol: test.protocol, OutboundSettingsJSON: test.settings,
				}},
				Connectors: []config.RuntimeEndpoint{{
					ID: "connector", Transport: model.TransportXray, Remote: "192.0.2.10", Port: 443, XrayProfileID: "profile",
				}},
			}
			compiled, err := Compile(runtime)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			loadOfficialXrayDocument(t, compiled.EmbeddedDocument)
		})
	}
}

func loadOfficialXrayDocument(t *testing.T, document map[string]any) {
	t.Helper()
	payload, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if _, err := xraycore.LoadConfig("json", bytes.NewReader(payload)); err != nil {
		t.Fatalf("official xray rejected generated config: %v\n%s", err, payload)
	}
}

func pairedRuntime(t *testing.T, protocol, inboundSettings, outboundSettings, network string, inboundStream, outboundStream map[string]any) *config.GeneratedRuntime {
	t.Helper()
	return &config.GeneratedRuntime{
		XrayProfiles: []config.RuntimeXrayProfile{
			{ID: "in", Runtime: model.XrayEmbedded, InboundProtocol: protocol, InboundSettingsJSON: inboundSettings, Network: network, Security: stringValue(inboundStream["security"]), StreamSettingsJSON: mustJSON(t, inboundStream)},
			{ID: "out", Runtime: model.XrayEmbedded, OutboundProtocol: protocol, OutboundSettingsJSON: outboundSettings, Network: network, Security: stringValue(outboundStream["security"]), StreamSettingsJSON: mustJSON(t, outboundStream)},
		},
		Listeners:  []config.RuntimeEndpoint{{ID: "listener", Transport: model.TransportXray, BindHost: "127.0.0.1", BindPort: 18443, XrayProfileID: "in"}},
		Connectors: []config.RuntimeEndpoint{{ID: "connector", Transport: model.TransportXray, Remote: "192.0.2.10", Port: 443, XrayProfileID: "out"}},
	}
}

func currentWebStreamSettings(network string, inbound bool) map[string]any {
	stream := map[string]any{"network": network, "security": "none"}
	switch network {
	case "tcp":
		stream["tcpSettings"] = map[string]any{"header": map[string]any{"type": "none"}}
		if inbound {
			stream["tcpSettings"].(map[string]any)["acceptProxyProtocol"] = false
		}
	case "ws":
		stream["wsSettings"] = map[string]any{"path": "/tapx", "host": "", "headers": map[string]any{}, "heartbeatPeriod": 0}
		if inbound {
			stream["wsSettings"].(map[string]any)["acceptProxyProtocol"] = false
		}
	case "grpc":
		stream["grpcSettings"] = map[string]any{"serviceName": "tapx.grpc", "authority": "", "multiMode": false}
	case "httpupgrade":
		stream["httpupgradeSettings"] = map[string]any{"path": "/tapx", "host": "", "headers": map[string]any{}}
		if inbound {
			stream["httpupgradeSettings"].(map[string]any)["acceptProxyProtocol"] = false
		}
	case "xhttp":
		headers := any([]any{})
		if inbound {
			headers = map[string]any{}
		}
		stream["xhttpSettings"] = map[string]any{"path": "/tapx", "host": "", "mode": "auto", "headers": headers, "xPaddingBytes": "100-1000"}
	}
	return stream
}

func writeTestCertificate(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "tapx.example"}, DNSNames: []string{"tapx.example"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func realityKeypair(t *testing.T) (string, string) {
	t.Helper()
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(privateKey.Bytes()), base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func mustMergeJSONObjects(t *testing.T, values ...string) string {
	t.Helper()
	merged := map[string]any{}
	for _, value := range values {
		var object map[string]any
		if err := json.Unmarshal([]byte(value), &object); err != nil {
			t.Fatal(err)
		}
		for key, item := range object {
			merged[key] = item
		}
	}
	payload, err := json.Marshal(merged)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
