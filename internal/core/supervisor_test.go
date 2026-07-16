package core

import (
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestRuntimeForXraySeparatesEmbeddedAndExternalObjects(t *testing.T) {
	runtime := &config.GeneratedRuntime{
		Listeners: []config.RuntimeEndpoint{
			{ID: "embedded-listener", Transport: model.TransportXray, XrayProfileID: "embedded-profile"},
			{ID: "external-listener", Transport: model.TransportXray, XrayProfileID: "external-profile"},
			{ID: "raw-listener", Transport: model.TransportUDP},
		},
		XrayProfiles: []config.RuntimeXrayProfile{
			{ID: "embedded-profile", Runtime: model.XrayEmbedded},
			{ID: "external-profile", Runtime: model.XrayExternal},
		},
		XrayPipes: []config.RuntimeXrayPipe{
			{EndpointID: "embedded-listener", Runtime: model.XrayEmbedded},
			{EndpointID: "external-listener", Runtime: model.XrayExternal},
		},
		TCPPipes: []config.RuntimeTCPPipe{
			{EndpointID: "raw-tcp"},
			{EndpointID: "external-listener", ExternalXrayBridge: true},
		},
		UDPPipes: []config.RuntimeUDPPipe{{EndpointID: "raw-listener"}},
	}

	embedded := runtimeForXray(runtime, model.XrayEmbedded)
	if len(embedded.Listeners) != 1 || embedded.Listeners[0].ID != "embedded-listener" || len(embedded.XrayProfiles) != 1 || len(embedded.XrayPipes) != 1 {
		t.Fatalf("embedded runtime = %+v", embedded)
	}
	if len(embedded.TCPPipes) != 0 || len(embedded.UDPPipes) != 0 {
		t.Fatalf("embedded runtime retained raw/external pipes: %+v", embedded)
	}

	external := runtimeForXray(runtime, model.XrayExternal)
	if len(external.Listeners) != 1 || external.Listeners[0].ID != "external-listener" || len(external.XrayProfiles) != 1 || len(external.XrayPipes) != 1 {
		t.Fatalf("external runtime = %+v", external)
	}
	if len(external.TCPPipes) != 1 || !external.TCPPipes[0].ExternalXrayBridge || len(external.UDPPipes) != 0 {
		t.Fatalf("external runtime pipe filtering = %+v", external)
	}
}

func TestSupervisorComponentStopsAreIsolated(t *testing.T) {
	rawTCP := &TCPPipeHandle{Pipe: config.RuntimeTCPPipe{EndpointID: "raw-tcp"}}
	externalBridge := &TCPPipeHandle{Pipe: config.RuntimeTCPPipe{EndpointID: "external-xray", ExternalXrayBridge: true}}
	embeddedPipe := &XrayPipeHandle{Pipe: config.RuntimeXrayPipe{EndpointID: "embedded-xray"}}
	s := &Supervisor{
		tcpPipes:  []*TCPPipeHandle{rawTCP, externalBridge},
		xrayPipes: []*XrayPipeHandle{embeddedPipe},
	}

	if err := s.StopComponent(RuntimeComponentTapX); err != nil {
		t.Fatalf("stop TapX: %v", err)
	}
	if len(s.tcpPipes) != 1 || s.tcpPipes[0] != externalBridge {
		t.Fatalf("TapX stop changed external Xray bridges: %+v", s.tcpPipes)
	}
	if len(s.xrayPipes) != 1 || s.xrayPipes[0] != embeddedPipe {
		t.Fatalf("TapX stop changed embedded Xray pipes: %+v", s.xrayPipes)
	}

	if err := s.StopComponent(RuntimeComponentExternalXray); err != nil {
		t.Fatalf("stop external Xray: %v", err)
	}
	if len(s.tcpPipes) != 0 {
		t.Fatalf("external Xray stop retained bridge pipes: %+v", s.tcpPipes)
	}
	if len(s.xrayPipes) != 1 || s.xrayPipes[0] != embeddedPipe {
		t.Fatalf("external Xray stop changed embedded Xray pipes: %+v", s.xrayPipes)
	}

	if err := s.StopComponent(RuntimeComponentEmbeddedXray); err != nil {
		t.Fatalf("stop embedded Xray: %v", err)
	}
	if len(s.xrayPipes) != 0 {
		t.Fatalf("embedded Xray stop retained pipes: %+v", s.xrayPipes)
	}
}

func TestSupervisorCloseClientPipes(t *testing.T) {
	s := &Supervisor{
		udpPipes: []*UDPPipeHandle{
			{Pipe: config.RuntimeUDPPipe{EndpointID: "udp-a", Binding: config.RuntimeBinding{ClientID: "client-a"}}, udpFD: -1},
			{Pipe: config.RuntimeUDPPipe{EndpointID: "udp-b", Binding: config.RuntimeBinding{ClientID: "client-b"}}, udpFD: -1},
		},
		tcpPipes: []*TCPPipeHandle{
			{Pipe: config.RuntimeTCPPipe{EndpointID: "tcp-a", Binding: config.RuntimeBinding{ClientID: "client-a"}}},
		},
		xrayPipes: []*XrayPipeHandle{
			{Pipe: config.RuntimeXrayPipe{EndpointID: "xray-a", Binding: config.RuntimeBinding{ClientID: "client-a"}}},
		},
	}

	closed, err := s.CloseClientPipes("client-a")
	if err != nil {
		t.Fatalf("close client pipes: %v", err)
	}
	if closed != 3 {
		t.Fatalf("closed = %d, want 3", closed)
	}
	if len(s.udpPipes) != 1 || s.udpPipes[0].Pipe.EndpointID != "udp-b" {
		t.Fatalf("udp pipes = %+v, want only udp-b", s.udpPipes)
	}
	if len(s.tcpPipes) != 0 {
		t.Fatalf("tcp pipes = %+v, want none", s.tcpPipes)
	}
	if len(s.xrayPipes) != 0 {
		t.Fatalf("xray pipes = %+v, want none", s.xrayPipes)
	}
}

func TestSupervisorCloseEndpointPipes(t *testing.T) {
	s := &Supervisor{
		udpPipes: []*UDPPipeHandle{
			{Pipe: config.RuntimeUDPPipe{EndpointID: "listener-a", EndpointKind: "listener"}, udpFD: -1},
			{Pipe: config.RuntimeUDPPipe{EndpointID: "connector-a", EndpointKind: "connector"}, udpFD: -1},
		},
		tcpPipes: []*TCPPipeHandle{
			{Pipe: config.RuntimeTCPPipe{EndpointID: "listener-a", EndpointKind: "listener"}},
		},
		xrayPipes: []*XrayPipeHandle{
			{Pipe: config.RuntimeXrayPipe{EndpointID: "listener-a", EndpointKind: "listener"}},
		},
	}

	closed, err := s.CloseEndpointPipes("listener", "listener-a")
	if err != nil {
		t.Fatalf("close endpoint pipes: %v", err)
	}
	if closed != 3 {
		t.Fatalf("closed = %d, want 3", closed)
	}
	if len(s.udpPipes) != 1 || s.udpPipes[0].Pipe.EndpointID != "connector-a" {
		t.Fatalf("udp pipes = %+v, want only connector-a", s.udpPipes)
	}
	if len(s.tcpPipes) != 0 || len(s.xrayPipes) != 0 {
		t.Fatalf("remaining tcp/xray pipes = %d/%d, want 0/0", len(s.tcpPipes), len(s.xrayPipes))
	}
}
