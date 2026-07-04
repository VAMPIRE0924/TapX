//go:build linux

package core

import (
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestSupervisorStartsUDPPipeOptional(t *testing.T) {
	if os.Getenv("TAPX_TEST_TUNTAP") != "1" {
		t.Skip("set TAPX_TEST_TUNTAP=1 to create a real TUN device")
	}
	runtime := &config.GeneratedRuntime{
		Devices: []config.RuntimeDevice{{
			ID:     "tun0",
			Type:   model.DeviceTUN,
			IfName: "tapxrt%d",
			MTU:    1500,
		}},
		UDPPipes: []config.RuntimeUDPPipe{{
			EndpointID:   "udp-listener",
			EndpointKind: "listener",
			DeviceID:     "tun0",
			FrameKind:    model.DeviceTUN,
			BindHost:     "127.0.0.1",
			BindPort:     0,
			PeerMode:     model.UDPPeerLearn,
			MaxFrameSize: 1500,
		}},
	}

	s := NewSupervisor()
	if err := s.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	handles := s.UDPPipes()
	if len(handles) != 1 {
		t.Fatalf("UDPPipes() = %d, want 1", len(handles))
	}
	if !handles[0].LocalAddr.IsValid() || handles[0].LocalAddr.Port() == 0 {
		t.Fatalf("local addr = %v, want allocated UDP port", handles[0].LocalAddr)
	}
	if handles[0].DeviceName == "" {
		t.Fatal("device name is empty")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSupervisorStartsTAPUDPPipeOptional(t *testing.T) {
	if os.Getenv("TAPX_TEST_TUNTAP") != "1" {
		t.Skip("set TAPX_TEST_TUNTAP=1 to create a real TAP device")
	}
	runtime := &config.GeneratedRuntime{
		Devices: []config.RuntimeDevice{{
			ID:     "tap0",
			Type:   model.DeviceTAP,
			IfName: "tapxrt%d",
			MTU:    1500,
		}},
		UDPPipes: []config.RuntimeUDPPipe{{
			EndpointID:   "udp-tap-listener",
			EndpointKind: "listener",
			DeviceID:     "tap0",
			FrameKind:    model.DeviceTAP,
			BindHost:     "127.0.0.1",
			BindPort:     0,
			PeerMode:     model.UDPPeerLearn,
			MaxFrameSize: 1500,
			AddressGuard: config.RuntimeAddressGuard{
				MACs:      []string{"02:00:00:00:00:01"},
				IPv4CIDRs: []string{"10.0.0.0/24"},
			},
		}},
	}

	s := NewSupervisor()
	if err := s.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	handles := s.UDPPipes()
	if len(handles) != 1 {
		t.Fatalf("UDPPipes() = %d, want 1", len(handles))
	}
	if !handles[0].LocalAddr.IsValid() || handles[0].LocalAddr.Port() == 0 {
		t.Fatalf("local addr = %v, want allocated UDP port", handles[0].LocalAddr)
	}
	if handles[0].DeviceName == "" {
		t.Fatal("device name is empty")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSupervisorStartsTCPPipeListenerOptional(t *testing.T) {
	if os.Getenv("TAPX_TEST_TUNTAP") != "1" {
		t.Skip("set TAPX_TEST_TUNTAP=1 to create a real TUN device")
	}
	runtime := &config.GeneratedRuntime{
		Devices: []config.RuntimeDevice{{
			ID:     "tun0",
			Type:   model.DeviceTUN,
			IfName: "tapxrt%d",
			MTU:    1500,
		}},
		TCPPipes: []config.RuntimeTCPPipe{{
			EndpointID:      "tcp-listener",
			EndpointKind:    "listener",
			DeviceID:        "tun0",
			FrameKind:       model.DeviceTUN,
			BindHost:        "127.0.0.1",
			BindPort:        0,
			LengthMode:      model.TCPLength16,
			MaxFrameSize:    1500,
			NoDelay:         true,
			KeepAliveSecond: 30,
		}},
	}

	s := NewSupervisor()
	if err := s.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	handles := s.TCPPipes()
	if len(handles) != 1 {
		t.Fatalf("TCPPipes() = %d, want 1", len(handles))
	}
	if !handles[0].LocalAddr.IsValid() || handles[0].LocalAddr.Port() == 0 {
		t.Fatalf("local addr = %v, want allocated TCP port", handles[0].LocalAddr)
	}
	if handles[0].DeviceName == "" {
		t.Fatal("device name is empty")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestSupervisorStartsTCPPipePairOptional(t *testing.T) {
	if os.Getenv("TAPX_TEST_TUNTAP") != "1" {
		t.Skip("set TAPX_TEST_TUNTAP=1 to create a real TUN device")
	}
	port := reserveTCPPort(t)
	runtime := &config.GeneratedRuntime{
		Devices: []config.RuntimeDevice{
			{
				ID:     "tun-listener",
				Type:   model.DeviceTUN,
				IfName: "tapxrt%d",
				MTU:    1500,
			},
			{
				ID:     "tun-connector",
				Type:   model.DeviceTUN,
				IfName: "tapxrt%d",
				MTU:    1500,
			},
		},
		TCPPipes: []config.RuntimeTCPPipe{
			{
				EndpointID:      "tcp-listener",
				EndpointKind:    "listener",
				DeviceID:        "tun-listener",
				FrameKind:       model.DeviceTUN,
				BindHost:        "127.0.0.1",
				BindPort:        port,
				LengthMode:      model.TCPLength16,
				MaxFrameSize:    1500,
				NoDelay:         true,
				KeepAliveSecond: 30,
			},
			{
				EndpointID:      "tcp-connector",
				EndpointKind:    "connector",
				DeviceID:        "tun-connector",
				FrameKind:       model.DeviceTUN,
				Remote:          "127.0.0.1",
				Port:            port,
				LengthMode:      model.TCPLength16,
				MaxFrameSize:    1500,
				NoDelay:         true,
				ConnectTimeout:  2,
				KeepAliveSecond: 30,
			},
		},
	}

	s := NewSupervisor()
	if err := s.Start(runtime); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := s.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	}()

	handles := s.TCPPipes()
	if len(handles) != 2 {
		t.Fatalf("TCPPipes() = %d, want 2", len(handles))
	}
	if !handles[1].RemoteAddr.IsValid() || handles[1].RemoteAddr.Port() != port {
		t.Fatalf("connector remote addr = %v, want port %d", handles[1].RemoteAddr, port)
	}
	waitForAcceptedRemote(t, handles[0])
}

func reserveTCPPort(t *testing.T) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve tcp port: %v", err)
	}
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split tcp addr %q: %v", listener.Addr(), err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse tcp port %q: %v", portText, err)
	}
	return uint16(port)
}

func waitForAcceptedRemote(t *testing.T, handle *TCPPipeHandle) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := handle.Err(); err != nil {
			t.Fatalf("listener error: %v", err)
		}
		remote := handle.AcceptedRemoteAddr()
		if remote.IsValid() && remote.Port() != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("listener did not accept connector before timeout")
}
