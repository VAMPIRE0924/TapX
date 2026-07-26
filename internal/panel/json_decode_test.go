package panel

import (
	"strings"
	"testing"

	"tapx/internal/config"
)

func TestStrictConfigDecodeRejectsRemovedRawFields(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		field string
	}{
		{
			name:  "udp peer mode",
			raw:   `{"Listeners":[{"ID":"listener-1","RawUDP":{"PeerMode":"learn"}}]}`,
			field: "PeerMode",
		},
		{
			name:  "tcp receive buffer",
			raw:   `{"Connectors":[{"ID":"connector-1","RawTCP":{"ReceiveBuffer":1048576}}]}`,
			field: "ReceiveBuffer",
		},
		{
			name:  "raw tls ca",
			raw:   `{"Connectors":[{"ID":"connector-1","RawTCP":{"TLS":{"CAFile":"legacy.crt"}}}]}`,
			field: "CAFile",
		},
		{
			name:  "raw dtls alpn",
			raw:   `{"Connectors":[{"ID":"connector-1","RawUDP":{"DTLS":{"ALPN":["tapx"]}}}]}`,
			field: "ALPN",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var cfg config.RuntimeConfig
			err := strictUnmarshal([]byte(test.raw), &cfg)
			if err == nil || !strings.Contains(err.Error(), `unknown field "`+test.field+`"`) {
				t.Fatalf("strictUnmarshal error = %v, want unknown %s field", err, test.field)
			}
		})
	}
}

func TestStrictConfigDecodeAcceptsCurrentRawFields(t *testing.T) {
	raw := []byte(`{
		"Listeners":[{"ID":"listener-1","RawUDP":{"Workers":0,"QueueSize":2048,"ZeroCopy":true,"DTLS":{"Enabled":true,"CertFile":"server.crt","KeyFile":"server.key"}}}],
		"Connectors":[{"ID":"connector-1","RawTCP":{"Workers":0,"QueueSize":2048,"ZeroCopy":true,"TLS":{"Enabled":true,"ServerName":"tapx.example","AllowInsecure":false}}}]
	}`)
	var cfg config.RuntimeConfig
	if err := strictUnmarshal(raw, &cfg); err != nil {
		t.Fatalf("strictUnmarshal current config: %v", err)
	}
}
