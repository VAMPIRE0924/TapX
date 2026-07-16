//go:build linux

package core

import (
	"errors"
	"net"
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

func TestOpenUDPSocketAutoOptimizeEnablesIPv4PathMTUDiscovery(t *testing.T) {
	fd, _, err := openUDPSocket(config.RuntimeUDPPipe{
		EndpointID:       "udp-auto-mtu",
		EndpointKind:     "connector",
		BindAddress:      "127.0.0.1",
		LinkAutoOptimize: true,
	}, netip.MustParseAddrPort("127.0.0.1:45000"))
	if err != nil {
		t.Fatalf("openUDPSocket() error = %v", err)
	}
	defer unix.Close(fd)

	got, err := unix.GetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_MTU_DISCOVER)
	if err != nil {
		t.Fatalf("getsockopt IP_MTU_DISCOVER: %v", err)
	}
	if got != unix.IP_PMTUDISC_DO {
		t.Fatalf("IP_MTU_DISCOVER = %d, want %d", got, unix.IP_PMTUDISC_DO)
	}
}

func TestConfigureUDPSocketAutoOptimizeEnablesIPv6PathMTUDiscovery(t *testing.T) {
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("IPv6 UDP socket unavailable: %v", err)
	}
	defer unix.Close(fd)
	if err := configureUDPSocket(fd, config.RuntimeUDPPipe{LinkAutoOptimize: true}, unix.AF_INET6); err != nil {
		t.Fatalf("configureUDPSocket() error = %v", err)
	}
	got, err := unix.GetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER)
	if err != nil {
		t.Fatalf("getsockopt IPV6_MTU_DISCOVER: %v", err)
	}
	if got != unix.IPV6_PMTUDISC_DO {
		t.Fatalf("IPV6_MTU_DISCOVER = %d, want %d", got, unix.IPV6_PMTUDISC_DO)
	}
}

func TestEnableUDPPathErrorQueueIPv4(t *testing.T) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if err := enableUDPPathErrorQueue(fd, false); err != nil {
		t.Fatalf("enableUDPPathErrorQueue() error = %v", err)
	}
	got, err := unix.GetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR)
	if err != nil {
		t.Fatalf("getsockopt IP_RECVERR: %v", err)
	}
	if got != 1 {
		t.Fatalf("IP_RECVERR = %d, want 1", got)
	}
}

func TestConnectUDPSocketPinsConfirmedPeer(t *testing.T) {
	peer := listenCoreUDP4(t)
	defer peer.Close()
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	peerAddr := peer.LocalAddr().(*net.UDPAddr).AddrPort()
	if err := connectUDPSocket(fd, peerAddr); err != nil {
		t.Fatalf("connectUDPSocket() error = %v", err)
	}
	remote, err := unix.Getpeername(fd)
	if err != nil {
		t.Fatalf("getpeername: %v", err)
	}
	got, ok := remote.(*unix.SockaddrInet4)
	if !ok || got.Port != int(peerAddr.Port()) || netip.AddrFrom4(got.Addr) != peerAddr.Addr() {
		t.Fatalf("connected peer = %#v, want %s", remote, peerAddr)
	}
}

func TestPrepareRawUDPWorkerSocketKeepsDispatchSocketUnconnected(t *testing.T) {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)

	peer := netip.MustParseAddrPort("127.0.0.1:45000")
	pipe := config.RuntimeUDPPipe{
		EndpointID: "dispatch-listener", LinkAutoOptimize: true,
		DispatchGroup: "raw-udp-listener",
	}
	if err := prepareRawUDPWorkerSocket(fd, pipe, peer); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Getpeername(fd); !errors.Is(err, unix.ENOTCONN) {
		t.Fatalf("dispatch socket peer error = %v, want ENOTCONN", err)
	}
}

func TestEnableUDPPathErrorQueueIPv6(t *testing.T) {
	fd, err := unix.Socket(unix.AF_INET6, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Skipf("IPv6 UDP socket unavailable: %v", err)
	}
	defer unix.Close(fd)
	if err := enableUDPPathErrorQueue(fd, true); err != nil {
		t.Fatalf("enableUDPPathErrorQueue() error = %v", err)
	}
	got, err := unix.GetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_RECVERR)
	if err != nil {
		t.Fatalf("getsockopt IPV6_RECVERR: %v", err)
	}
	if got != 1 {
		t.Fatalf("IPV6_RECVERR = %d, want 1", got)
	}
}

func TestStartUDPPipeRejectsUnconfirmedAutomaticOptimization(t *testing.T) {
	_, err := startUDPPipe(config.RuntimeUDPPipe{
		EndpointID: "udp-unconfirmed", LinkAutoOptimize: true,
	}, config.RuntimeDevice{})
	if err == nil {
		t.Fatal("startUDPPipe() accepted automatic optimization without a confirmed plan")
	}
}

func TestStartUDPPipeRejectsPlanWhenOptimizationIsDisabled(t *testing.T) {
	_, err := startUDPPipe(config.RuntimeUDPPipe{
		EndpointID: "udp-disabled", MaxDatagramPayload: 1400,
	}, config.RuntimeDevice{})
	if err == nil {
		t.Fatal("startUDPPipe() accepted a path plan while optimization is disabled")
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
