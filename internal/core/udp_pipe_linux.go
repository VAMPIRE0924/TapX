//go:build linux

package core

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
	"tapx/internal/netapply"
	"tapx/internal/tuntap"
)

type UDPPipeHandle struct {
	Pipe       config.RuntimeUDPPipe
	LocalAddr  netip.AddrPort
	RemoteAddr netip.AddrPort
	DeviceName string

	device   tuntap.Device
	netApply netapply.Handle
	udpFD    int
	worker   *fastpath.Worker
	counter  *fastpath.Counters

	mu             sync.Mutex
	acceptedRemote netip.AddrPort
	lastErr        error
	dtlsCancel     context.CancelFunc
	dtlsDone       chan struct{}
	dtlsConn       net.Conn
	dtlsPacketConn net.PacketConn
	dtlsListener   net.Listener
	dtlsCounter    xrayPipeCounters
}

func startUDPPipe(pipe config.RuntimeUDPPipe, device config.RuntimeDevice) (*UDPPipeHandle, error) {
	frameKind, err := fastpath.FrameKindFromDevice(device.Type)
	if err != nil {
		return nil, err
	}
	peerMode, err := fastpath.PeerModeFromModel(pipe.PeerMode)
	if err != nil {
		return nil, err
	}
	peer, peerMode, err := peerForPipe(pipe, peerMode)
	if err != nil {
		return nil, err
	}
	addressGuard, err := fastpathAddressGuard(pipe.AddressGuard)
	if err != nil {
		return nil, err
	}

	tunDevice, err := tuntap.Open(tuntap.OpenOptions{
		Name:     device.IfName,
		Type:     device.Type,
		NonBlock: true,
	})
	if err != nil {
		return nil, fmt.Errorf("core: open %s %s: %w", device.Type, device.IfName, err)
	}
	netHandle, err := netapply.ApplyDevice(netapply.DeviceConfig{
		Type:     device.Type,
		IfName:   tunDevice.Name(),
		MTU:      device.MTU,
		MSSClamp: device.MSSClamp,
		IPv4CIDR: device.IPv4CIDR,
		IPv6CIDR: device.IPv6CIDR,
		Bridge: netapply.BridgeConfig{
			Enabled: device.Bridge.Enabled,
			Name:    device.Bridge.Name,
			IfName:  device.Bridge.IfName,
			MTU:     device.Bridge.MTU,
		},
		Routes: netapplyRoutes(device.Routes),
		DNS:    netapplyDNS(device.DNS),
	})
	if err != nil {
		_ = tunDevice.Close()
		return nil, fmt.Errorf("core: apply device %s: %w", tunDevice.Name(), err)
	}

	handle := &UDPPipeHandle{
		Pipe:       pipe,
		DeviceName: tunDevice.Name(),
		device:     tunDevice,
		netApply:   netHandle,
		udpFD:      -1,
	}
	if pipe.DTLS.Enabled {
		if err := handle.startDTLS(frameKind, addressGuard, peer); err != nil {
			_ = handle.Close()
			return nil, err
		}
		return handle, nil
	}

	udpFD, local, err := openUDPSocket(pipe, peer)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}

	counters := fastpath.NewCounters()
	worker, err := fastpath.StartUDPPipe(fastpath.UDPConfig{
		TUNFD:        tunDevice.FD(),
		UDPFD:        udpFD,
		FrameKind:    frameKind,
		MaxFrameSize: uint32(pipe.MaxFrameSize),
		PeerMode:     peerMode,
		Peer:         peer,
		VKey:         []byte(pipe.Binding.VKeyValue),
		AddressGuard: addressGuard,
		Counters:     counters,
	})
	if err != nil {
		_ = unix.Close(udpFD)
		counters.Close()
		_ = handle.Close()
		return nil, err
	}

	handle.LocalAddr = local
	handle.udpFD = udpFD
	handle.worker = worker
	handle.counter = counters
	return handle, nil
}

func (h *UDPPipeHandle) Close() error {
	var firstErr error
	if h.dtlsListener != nil {
		if err := h.dtlsListener.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.dtlsListener = nil
	}
	if h.dtlsCancel != nil {
		h.dtlsCancel()
		h.dtlsCancel = nil
	}
	h.mu.Lock()
	dtlsConn := h.dtlsConn
	dtlsDone := h.dtlsDone
	h.mu.Unlock()
	if dtlsConn != nil {
		_ = dtlsConn.Close()
	}
	if h.dtlsPacketConn != nil {
		if err := h.dtlsPacketConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.dtlsPacketConn = nil
	}
	if dtlsDone != nil {
		<-dtlsDone
	}
	if h.worker != nil {
		if err := h.worker.Stop(); err != nil {
			firstErr = err
		}
		h.worker = nil
	}
	if h.udpFD >= 0 {
		if err := unix.Close(h.udpFD); err != nil && firstErr == nil {
			firstErr = err
		}
		h.udpFD = -1
	}
	if h.device != nil {
		if h.netApply != nil {
			if err := h.netApply.Rollback(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.netApply = nil
		}
		if err := h.device.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.device = nil
	}
	if h.counter != nil {
		h.counter.Close()
		h.counter = nil
	}
	return firstErr
}

func (h *UDPPipeHandle) Counters() fastpath.CountersSnapshot {
	if h == nil {
		return fastpath.CountersSnapshot{}
	}
	if h.Pipe.DTLS.Enabled {
		return h.dtlsCounters()
	}
	if h.worker == nil {
		return fastpath.CountersSnapshot{}
	}
	return h.worker.Counters()
}

func (h *UDPPipeHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErr
}

func (h *UDPPipeHandle) AcceptedRemoteAddr() netip.AddrPort {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.acceptedRemote
}

func (h *UDPPipeHandle) setErr(err error) {
	h.mu.Lock()
	if h.lastErr == nil {
		h.lastErr = err
	}
	h.mu.Unlock()
}

func peerForPipe(pipe config.RuntimeUDPPipe, mode fastpath.UDPPeerMode) (netip.AddrPort, fastpath.UDPPeerMode, error) {
	if pipe.FixedPeer != "" {
		peer, err := netip.ParseAddrPort(pipe.FixedPeer)
		if err != nil {
			return netip.AddrPort{}, 0, fmt.Errorf("core: parse fixed peer %q: %w", pipe.FixedPeer, err)
		}
		return peer, mode, nil
	}
	if pipe.Remote != "" && pipe.Port != 0 {
		addr, err := netip.ParseAddr(pipe.Remote)
		if err != nil {
			return netip.AddrPort{}, 0, fmt.Errorf("core: parse remote %q: %w", pipe.Remote, err)
		}
		peer := netip.AddrPortFrom(addr, pipe.Port)
		if mode == fastpath.UDPPeerAny {
			mode = fastpath.UDPPeerFixed
		}
		return peer, mode, nil
	}
	return netip.AddrPort{}, mode, nil
}

func openUDPSocket(pipe config.RuntimeUDPPipe, peer netip.AddrPort) (int, netip.AddrPort, error) {
	bind := netip.IPv4Unspecified()
	if peer.IsValid() && peer.Addr().Is6() {
		bind = netip.IPv6Unspecified()
	}
	if pipe.BindHost != "" {
		addr, err := netip.ParseAddr(pipe.BindHost)
		if err != nil {
			return -1, netip.AddrPort{}, fmt.Errorf("core: parse bind host %q: %w", pipe.BindHost, err)
		}
		bind = addr.Unmap()
	}
	if pipe.BindAddress != "" {
		addr, err := netip.ParseAddr(pipe.BindAddress)
		if err != nil {
			return -1, netip.AddrPort{}, fmt.Errorf("core: parse udp bind address %q: %w", pipe.BindAddress, err)
		}
		bind = addr.Unmap()
	}

	family := unix.AF_INET
	if bind.Is6() {
		family = unix.AF_INET6
	}
	fd, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return -1, netip.AddrPort{}, fmt.Errorf("core: create udp socket: %w", err)
	}
	if err := configureUDPSocket(fd, pipe); err != nil {
		_ = unix.Close(fd)
		return -1, netip.AddrPort{}, err
	}
	if err := bindUDP(fd, bind, pipe.BindPort); err != nil {
		_ = unix.Close(fd)
		return -1, netip.AddrPort{}, err
	}
	local, err := localUDPAddr(fd, bind.Is6())
	if err != nil {
		_ = unix.Close(fd)
		return -1, netip.AddrPort{}, err
	}
	return fd, local, nil
}

func openUDPPacketConn(pipe config.RuntimeUDPPipe, peer netip.AddrPort) (net.PacketConn, netip.AddrPort, error) {
	fd, local, err := openUDPSocket(pipe, peer)
	if err != nil {
		return nil, netip.AddrPort{}, err
	}
	file := os.NewFile(uintptr(fd), fmt.Sprintf("tapx-udp-%s", pipe.EndpointID))
	if file == nil {
		_ = unix.Close(fd)
		return nil, netip.AddrPort{}, fmt.Errorf("core: create udp file for %s", pipe.EndpointID)
	}
	packetConn, err := net.FilePacketConn(file)
	if closeErr := file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, netip.AddrPort{}, fmt.Errorf("core: convert udp socket for %s: %w", pipe.EndpointID, err)
	}
	return packetConn, local, nil
}

func configureUDPSocket(fd int, pipe config.RuntimeUDPPipe) error {
	if pipe.BindInterface != "" {
		if err := unix.SetsockoptString(fd, unix.SOL_SOCKET, unix.SO_BINDTODEVICE, pipe.BindInterface); err != nil {
			return fmt.Errorf("core: bind udp socket to interface %q: %w", pipe.BindInterface, err)
		}
	}
	if pipe.ReuseAddr {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			return fmt.Errorf("core: set udp SO_REUSEADDR: %w", err)
		}
	}
	if pipe.ReusePort {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
			return fmt.Errorf("core: set udp SO_REUSEPORT: %w", err)
		}
	}
	if pipe.ReceiveBuffer > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, pipe.ReceiveBuffer); err != nil {
			return fmt.Errorf("core: set udp receive buffer: %w", err)
		}
	}
	if pipe.SendBuffer > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, pipe.SendBuffer); err != nil {
			return fmt.Errorf("core: set udp send buffer: %w", err)
		}
	}
	return nil
}

func bindUDP(fd int, addr netip.Addr, port uint16) error {
	if addr.Is4() {
		raw := addr.As4()
		sa := &unix.SockaddrInet4{Port: int(port), Addr: raw}
		if err := unix.Bind(fd, sa); err != nil {
			return fmt.Errorf("core: bind udp %s:%d: %w", addr, port, err)
		}
		return nil
	}
	raw := addr.As16()
	sa := &unix.SockaddrInet6{Port: int(port), Addr: raw}
	if err := unix.Bind(fd, sa); err != nil {
		return fmt.Errorf("core: bind udp [%s]:%d: %w", addr, port, err)
	}
	return nil
}

func localUDPAddr(fd int, ipv6 bool) (netip.AddrPort, error) {
	sa, err := unix.Getsockname(fd)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("core: getsockname udp: %w", err)
	}
	switch addr := sa.(type) {
	case *unix.SockaddrInet4:
		return netip.AddrPortFrom(netip.AddrFrom4(addr.Addr), uint16(addr.Port)), nil
	case *unix.SockaddrInet6:
		return netip.AddrPortFrom(netip.AddrFrom16(addr.Addr), uint16(addr.Port)), nil
	default:
		return netip.AddrPort{}, fmt.Errorf("core: unsupported udp sockname %T ipv6=%s", sa, strconv.FormatBool(ipv6))
	}
}

func modelPeerMode(mode model.UDPPeerMode) model.UDPPeerMode {
	if mode == "" {
		return model.UDPPeerAny
	}
	return mode
}
