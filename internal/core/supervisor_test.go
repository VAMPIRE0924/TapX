package core

import (
	"testing"

	"tapx/internal/config"
)

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
