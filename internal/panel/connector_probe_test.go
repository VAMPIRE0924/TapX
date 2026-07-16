package panel

import (
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestConnectorProbeNetwork(t *testing.T) {
	cfg := config.RuntimeConfig{XrayProfiles: []model.XrayProfile{{ID: "xr", Network: "xhttp"}}}
	tests := []struct {
		name      string
		connector model.Connector
		want      string
	}{
		{name: "raw udp", connector: model.Connector{Transport: model.TransportUDP}, want: "udp"},
		{name: "raw dtls", connector: model.Connector{Transport: model.TransportUDP, RawUDP: model.RawUDPSettings{DTLS: model.RawDTLSSettings{Enabled: true}}}, want: "dtls"},
		{name: "raw tcp", connector: model.Connector{Transport: model.TransportTCP}, want: "tcp"},
		{name: "raw tls", connector: model.Connector{Transport: model.TransportTCP, RawTCP: model.RawTCPSettings{TLS: model.RawTLSSettings{Enabled: true}}}, want: "tls"},
		{name: "xray", connector: model.Connector{Transport: model.TransportXray, XrayProfileID: "xr"}, want: "xray/xhttp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := connectorProbeNetwork(cfg, test.connector); got != test.want {
				t.Fatalf("connectorProbeNetwork() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConnectorRuntimePipe(t *testing.T) {
	state := RuntimeState{UDPPipes: []RuntimePipeState{{EndpointID: "c1", EndpointKind: "connector", ConfirmedPathMTU: 1500}}}
	pipe, ok := connectorRuntimePipe(state, "c1")
	if !ok || pipe.ConfirmedPathMTU != 1500 {
		t.Fatalf("connectorRuntimePipe() = %+v, %v", pipe, ok)
	}
	if _, ok := connectorRuntimePipe(state, "missing"); ok {
		t.Fatal("connectorRuntimePipe(missing) found a pipe")
	}
}
