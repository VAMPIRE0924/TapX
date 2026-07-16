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
	"time"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/netapply"
	"tapx/internal/pathmtu"
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
	pathPreparer   rawUDPPathPreparer
	runtimeDevice  config.RuntimeDevice
	startupCancel  context.CancelFunc
	startupDone    chan struct{}
}

func startUDPPipe(pipe config.RuntimeUDPPipe, device config.RuntimeDevice) (*UDPPipeHandle, error) {
	return startUDPPipeWithCache(pipe, device, nil)
}

func startUDPPipeWithCache(pipe config.RuntimeUDPPipe, device config.RuntimeDevice, pathCache *pathmtu.Cache) (*UDPPipeHandle, error) {
	handle, err := prepareUDPPipeHandle(pipe, device, pathCache)
	if err != nil {
		return nil, err
	}
	frameKind, err := fastpath.FrameKindFromDevice(device.Type)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	peerMode, err := fastpath.PeerModeFromModel(pipe.PeerMode)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	peer, peerMode, err := peerForPipe(pipe, peerMode)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}
	addressGuard, err := fastpathAddressGuard(pipe.AddressGuard)
	if err != nil {
		_ = handle.Close()
		return nil, err
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
	handle.LocalAddr = local
	handle.udpFD = udpFD
	if pipe.LinkAutoOptimize && pipe.MaxDatagramPayload <= pathmtu.SegmentHeaderSize && pipe.EndpointKind == "listener" {
		probeConn, err := duplicateUDPConn(udpFD)
		if err != nil {
			_ = handle.Close()
			return nil, fmt.Errorf("core: prepare UDP path probe socket %s: %w", pipe.EndpointID, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		handle.startupCancel = cancel
		handle.startupDone = done
		go func() {
			defer close(done)
			var preparedPipe config.RuntimeUDPPipe
			var confirmedPeer netip.AddrPort
			for {
				var prepareErr error
				preparedPipe, confirmedPeer, prepareErr = handle.pathPreparer.prepare(ctx, pipe, device, probeConn, peer)
				if prepareErr == nil {
					break
				}
				handle.replaceErr(prepareErr)
				retry := time.NewTimer(time.Second)
				select {
				case <-retry.C:
				case <-ctx.Done():
					if !retry.Stop() {
						<-retry.C
					}
					_ = probeConn.Close()
					return
				}
			}
			closeErr := probeConn.Close()
			if closeErr != nil {
				handle.setErr(fmt.Errorf("core: close UDP path probe socket %s: %w", pipe.EndpointID, closeErr))
				return
			}
			if ctx.Err() != nil {
				return
			}
			if clampErr := handle.netApply.SetMSSClamp(preparedPipe.TCPMSSIPv4, preparedPipe.TCPMSSIPv6); clampErr != nil {
				handle.setErr(fmt.Errorf("core: apply confirmed UDP MSS for %s: %w", pipe.EndpointID, clampErr))
				return
			}
			worker, counters, startErr := startRawUDPWorker(handle.device, preparedPipe, frameKind, addressGuard, udpFD, fastpath.UDPPeerFixed, confirmedPeer)
			if startErr != nil {
				handle.setErr(startErr)
				return
			}
			handle.mu.Lock()
			handle.lastErr = nil
			handle.Pipe = preparedPipe
			handle.RemoteAddr = confirmedPeer
			handle.acceptedRemote = confirmedPeer
			handle.worker = worker
			handle.counter = counters
			handle.mu.Unlock()
		}()
		return handle, nil
	}
	if pipe.LinkAutoOptimize && pipe.MaxDatagramPayload <= pathmtu.SegmentHeaderSize {
		probeConn, err := duplicateUDPConn(udpFD)
		if err != nil {
			_ = handle.Close()
			return nil, fmt.Errorf("core: prepare UDP path probe socket %s: %w", pipe.EndpointID, err)
		}
		probeTimeout := time.Duration(pipe.ConnectTimeout) * time.Second
		if probeTimeout <= 0 {
			probeTimeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		preparedPipe, confirmedPeer, prepareErr := defaultRawUDPPathPreparer(pathCache).prepare(ctx, pipe, device, probeConn, peer)
		cancel()
		closeErr := probeConn.Close()
		if prepareErr != nil {
			_ = handle.Close()
			return nil, prepareErr
		}
		if closeErr != nil {
			_ = handle.Close()
			return nil, fmt.Errorf("core: close UDP path probe socket %s: %w", pipe.EndpointID, closeErr)
		}
		pipe = preparedPipe
		peer = confirmedPeer
		peerMode = fastpath.UDPPeerFixed
		handle.Pipe = pipe
		if err := handle.netApply.SetMSSClamp(pipe.TCPMSSIPv4, pipe.TCPMSSIPv6); err != nil {
			_ = handle.Close()
			return nil, fmt.Errorf("core: apply confirmed UDP MSS for %s: %w", pipe.EndpointID, err)
		}
	}

	worker, counters, err := startRawUDPWorker(handle.device, pipe, frameKind, addressGuard, udpFD, peerMode, peer)
	if err != nil {
		_ = handle.Close()
		return nil, err
	}

	handle.RemoteAddr = peer
	handle.acceptedRemote = peer
	handle.worker = worker
	handle.counter = counters
	return handle, nil
}

func prepareUDPPipeHandle(pipe config.RuntimeUDPPipe, device config.RuntimeDevice, pathCache *pathmtu.Cache) (*UDPPipeHandle, error) {
	if !pipe.LinkAutoOptimize && pipe.MaxDatagramPayload != 0 {
		return nil, fmt.Errorf("core: udp pipe %s has a datagram path plan while automatic link optimization is disabled", pipe.EndpointID)
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
		Type:             device.Type,
		IfName:           tunDevice.Name(),
		MTU:              device.MTU,
		MSSClamp:         device.MSSClamp,
		LinkAutoOptimize: device.LinkAutoOptimize,
		IPv4CIDR:         device.IPv4CIDR,
		IPv6CIDR:         device.IPv6CIDR,
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
		Pipe:          pipe,
		DeviceName:    tunDevice.Name(),
		device:        tunDevice,
		netApply:      netHandle,
		udpFD:         -1,
		pathPreparer:  defaultRawUDPPathPreparer(pathCache),
		runtimeDevice: device,
	}
	return handle, nil
}

func startRawUDPWorker(device tuntap.Device, pipe config.RuntimeUDPPipe, frameKind fastpath.FrameKind, addressGuard fastpath.AddressGuard, udpFD int, peerMode fastpath.UDPPeerMode, peer netip.AddrPort) (*fastpath.Worker, *fastpath.Counters, error) {
	if err := prepareRawUDPWorkerSocket(udpFD, pipe, peer); err != nil {
		return nil, nil, err
	}
	counters := fastpath.NewCounters()
	networkToDeviceRate, deviceToNetworkRate := userTrafficRates(pipe.EndpointKind, pipe.Binding)
	worker, err := fastpath.StartUDPPipe(fastpath.UDPConfig{
		TUNFD:                  device.FD(),
		UDPFD:                  udpFD,
		FrameKind:              frameKind,
		MaxFrameSize:           uint32(pipe.MaxFrameSize),
		MaxDatagramPayload:     uint32(pipe.MaxDatagramPayload),
		PeerMode:               peerMode,
		AddressGuardRemote:     pipe.AddressGuardRemote,
		Peer:                   peer,
		DeviceToNetworkRateBPS: deviceToNetworkRate,
		NetworkToDeviceRateBPS: networkToDeviceRate,
		VKey:                   []byte(pipe.Binding.VKeyValue),
		AddressGuard:           addressGuard,
		Counters:               counters,
	})
	if err != nil {
		counters.Close()
		return nil, nil, err
	}
	return worker, counters, nil
}

func prepareRawUDPWorkerSocket(fd int, pipe config.RuntimeUDPPipe, peer netip.AddrPort) error {
	if !pipe.LinkAutoOptimize {
		return nil
	}
	if !peer.IsValid() {
		return fmt.Errorf("core: automatic UDP path adaptation for %s requires a confirmed peer", pipe.EndpointID)
	}
	// A connected UDP socket may leave or reorder its SO_REUSEPORT listener
	// group. Dispatch sockets must keep their kernel BPF socket indexes stable;
	// the fastpath already pins and validates their peer explicitly.
	if pipe.DispatchGroup == "" {
		if err := connectUDPSocket(fd, peer); err != nil {
			return fmt.Errorf("core: connect confirmed UDP peer for %s: %w", pipe.EndpointID, err)
		}
	}
	if err := enableUDPPathErrorQueue(fd, peer.Addr().Is6()); err != nil {
		return fmt.Errorf("core: enable UDP path error queue for %s: %w", pipe.EndpointID, err)
	}
	return nil
}

func connectUDPSocket(fd int, peer netip.AddrPort) error {
	addr := peer.Addr().Unmap()
	if addr.Is4() {
		raw := addr.As4()
		return unix.Connect(fd, &unix.SockaddrInet4{Port: int(peer.Port()), Addr: raw})
	}
	if addr.Is6() {
		raw := addr.As16()
		return unix.Connect(fd, &unix.SockaddrInet6{Port: int(peer.Port()), Addr: raw})
	}
	return fmt.Errorf("peer address %s is not IPv4 or IPv6", peer.Addr())
}

func enableUDPPathErrorQueue(fd int, ipv6 bool) error {
	if ipv6 {
		return unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_RECVERR, 1)
	}
	return unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR, 1)
}

func (h *UDPPipeHandle) Close() error {
	var firstErr error
	if h.startupCancel != nil {
		h.startupCancel()
		h.startupCancel = nil
	}
	if h.startupDone != nil {
		<-h.startupDone
		h.startupDone = nil
	}
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
	h.mu.Lock()
	dtlsEnabled := h.Pipe.DTLS.Enabled
	worker := h.worker
	h.mu.Unlock()
	if dtlsEnabled {
		return h.dtlsCounters()
	}
	if worker == nil {
		return fastpath.CountersSnapshot{}
	}
	return worker.Counters()
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

func (h *UDPPipeHandle) replaceErr(err error) {
	h.mu.Lock()
	h.lastErr = err
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
	return openUDPSocketWithHook(pipe, peer, nil)
}

func openUDPSocketWithHook(pipe config.RuntimeUDPPipe, peer netip.AddrPort, beforeBind func(int) error) (int, netip.AddrPort, error) {
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
	if err := configureUDPSocket(fd, pipe, family); err != nil {
		_ = unix.Close(fd)
		return -1, netip.AddrPort{}, err
	}
	if beforeBind != nil {
		if err := beforeBind(fd); err != nil {
			_ = unix.Close(fd)
			return -1, netip.AddrPort{}, err
		}
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

func configureUDPSocket(fd int, pipe config.RuntimeUDPPipe, family int) error {
	if pipe.LinkAutoOptimize {
		switch family {
		case unix.AF_INET:
			if err := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO); err != nil {
				return fmt.Errorf("core: enable IPv4 path MTU discovery: %w", err)
			}
		case unix.AF_INET6:
			if err := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_MTU_DISCOVER, unix.IPV6_PMTUDISC_DO); err != nil {
				return fmt.Errorf("core: enable IPv6 path MTU discovery: %w", err)
			}
		default:
			return fmt.Errorf("core: unsupported UDP address family %d", family)
		}
	}
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
