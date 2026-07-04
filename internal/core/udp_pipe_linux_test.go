//go:build linux

package core

import (
	"net/netip"
	"testing"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestOpenUDPSocketAppliesAdvancedSocketSettings(t *testing.T) {
	pipe := config.RuntimeUDPPipe{
		EndpointID:    "udp-listener",
		EndpointKind:  "listener",
		FrameKind:     model.DeviceTUN,
		BindHost:      "0.0.0.0",
		BindAddress:   "127.0.0.1",
		ReceiveBuffer: 8192,
		SendBuffer:    16384,
		ReuseAddr:     true,
		ReusePort:     true,
	}

	fd, local, err := openUDPSocket(pipe, netip.AddrPort{})
	if err != nil {
		t.Fatalf("openUDPSocket() error = %v", err)
	}
	defer unix.Close(fd)

	if got := local.Addr().String(); got != "127.0.0.1" {
		t.Fatalf("local addr = %s, want 127.0.0.1", got)
	}
	assertSockoptAtLeast(t, fd, unix.SO_RCVBUF, pipe.ReceiveBuffer)
	assertSockoptAtLeast(t, fd, unix.SO_SNDBUF, pipe.SendBuffer)
}

func TestOpenUDPSocketAllowsExplicitPortReuse(t *testing.T) {
	firstPipe := config.RuntimeUDPPipe{
		EndpointID:   "udp-a",
		EndpointKind: "listener",
		FrameKind:    model.DeviceTUN,
		BindAddress:  "127.0.0.1",
		ReuseAddr:    true,
		ReusePort:    true,
	}

	firstFD, firstLocal, err := openUDPSocket(firstPipe, netip.AddrPort{})
	if err != nil {
		t.Fatalf("open first udp socket: %v", err)
	}
	defer unix.Close(firstFD)
	if firstLocal.Port() == 0 {
		t.Fatalf("first local port = 0, want allocated port")
	}

	secondPipe := firstPipe
	secondPipe.EndpointID = "udp-b"
	secondPipe.BindPort = firstLocal.Port()
	secondFD, _, err := openUDPSocket(secondPipe, netip.AddrPort{})
	if err != nil {
		t.Fatalf("open second udp socket on reused port %d: %v", firstLocal.Port(), err)
	}
	defer unix.Close(secondFD)
}

func TestOpenUDPSocketRejectsInvalidBindAddress(t *testing.T) {
	_, _, err := openUDPSocket(config.RuntimeUDPPipe{
		EndpointID:   "udp-listener",
		EndpointKind: "listener",
		BindAddress:  "not-an-ip",
	}, netip.AddrPort{})
	if err == nil {
		t.Fatal("openUDPSocket() error = nil, want invalid bind address error")
	}
}

func assertSockoptAtLeast(t *testing.T, fd int, opt int, want int) {
	t.Helper()
	got, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, opt)
	if err != nil {
		t.Fatalf("getsockopt %d: %v", opt, err)
	}
	if got < want {
		t.Fatalf("getsockopt %d = %d, want at least %d", opt, got, want)
	}
}
