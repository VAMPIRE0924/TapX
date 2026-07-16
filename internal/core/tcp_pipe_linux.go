//go:build linux

package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/linkdiag"
	"tapx/internal/netapply"
	"tapx/internal/tuntap"
)

type TCPPipeHandle struct {
	Pipe       config.RuntimeTCPPipe
	LocalAddr  netip.AddrPort
	RemoteAddr netip.AddrPort
	DeviceName string

	device     tuntap.Device
	netApply   netapply.Handle
	shared     *tcpSharedDevice
	owner      bool
	listener   *net.TCPListener
	session    *tcpWorkerSession
	counter    *fastpath.Counters
	acceptDone chan struct{}

	mu              sync.Mutex
	acceptedRemote  netip.AddrPort
	lastErr         error
	tlsCancel       context.CancelFunc
	tlsDone         chan struct{}
	tlsConn         net.Conn
	tlsCounter      xrayPipeCounters
	reconnectCancel context.CancelFunc
	reconnectDone   chan struct{}
}

type tcpWorkerSession struct {
	worker      *fastpath.Worker
	file        *os.File
	fd          int
	done        chan struct{}
	monitorDone chan struct{}
	stopOnce    sync.Once
	stopErr     error
}

func (s *tcpWorkerSession) Stop() error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		if s.worker != nil {
			s.stopErr = s.worker.Stop()
			s.worker = nil
		}
		if s.file != nil {
			if err := s.file.Close(); err != nil && s.stopErr == nil {
				s.stopErr = err
			}
			s.file = nil
		}
		close(s.done)
	})
	return s.stopErr
}

type tcpSharedDevice struct {
	device   tuntap.Device
	netApply netapply.Handle

	mu     sync.Mutex
	active bool
}

func startTCPPipe(pipe config.RuntimeTCPPipe, device config.RuntimeDevice) (*TCPPipeHandle, error) {
	return startTCPPipeShared(pipe, device, nil)
}

func startTCPPipeShared(pipe config.RuntimeTCPPipe, device config.RuntimeDevice, shared *tcpSharedDevice) (*TCPPipeHandle, error) {
	frameKind, err := fastpath.FrameKindFromDevice(device.Type)
	if err != nil {
		return nil, err
	}
	lengthMode, err := fastpath.TCPLengthModeFromModel(pipe.LengthMode)
	if err != nil {
		return nil, err
	}
	addressGuard, err := fastpathAddressGuard(pipe.AddressGuard)
	if err != nil {
		return nil, err
	}

	handle, err := prepareTCPPipeHandle(pipe, device, shared)
	if err != nil {
		return nil, err
	}

	switch pipe.EndpointKind {
	case "listener":
		var err error
		if pipe.ExternalXrayBridge {
			err = handle.startGoListener(frameKind, addressGuard)
		} else if pipe.TLS.Enabled {
			err = handle.startTLSListener(frameKind, addressGuard)
		} else {
			err = handle.startListener(frameKind, lengthMode, addressGuard)
		}
		if err != nil {
			_ = handle.Close()
			return nil, err
		}
	case "connector":
		var err error
		if pipe.ExternalXrayBridge {
			err = handle.startGoConnector(frameKind, addressGuard)
		} else if pipe.TLS.Enabled {
			err = handle.startTLSConnector(frameKind, addressGuard)
		} else {
			err = handle.startConnector(frameKind, lengthMode, addressGuard)
		}
		if err != nil {
			_ = handle.Close()
			return nil, err
		}
	default:
		_ = handle.Close()
		return nil, fmt.Errorf("core: tcp pipe %s has unsupported endpoint kind %q", pipe.EndpointID, pipe.EndpointKind)
	}

	return handle, nil
}

func prepareTCPPipeHandle(pipe config.RuntimeTCPPipe, device config.RuntimeDevice, shared *tcpSharedDevice) (*TCPPipeHandle, error) {
	owner := false
	if shared == nil {
		tunDevice, err := tuntap.Open(tuntap.OpenOptions{
			Name: device.IfName, Type: device.Type, NonBlock: true,
		})
		if err != nil {
			return nil, fmt.Errorf("core: open %s %s: %w", device.Type, device.IfName, err)
		}
		netHandle, err := netapply.ApplyDevice(netapply.DeviceConfig{
			Type: device.Type, IfName: tunDevice.Name(), MTU: device.MTU, MSSClamp: device.MSSClamp,
			LinkAutoOptimize: device.LinkAutoOptimize, IPv4CIDR: device.IPv4CIDR, IPv6CIDR: device.IPv6CIDR,
			Bridge: netapply.BridgeConfig{Enabled: device.Bridge.Enabled, Name: device.Bridge.Name, IfName: device.Bridge.IfName, MTU: device.Bridge.MTU},
			Routes: netapplyRoutes(device.Routes), DNS: netapplyDNS(device.DNS),
		})
		if err != nil {
			_ = tunDevice.Close()
			return nil, fmt.Errorf("core: apply device %s: %w", tunDevice.Name(), err)
		}
		shared = &tcpSharedDevice{device: tunDevice, netApply: netHandle}
		owner = true
	}
	return &TCPPipeHandle{
		Pipe: pipe, DeviceName: shared.device.Name(), device: shared.device, netApply: shared.netApply,
		shared: shared, owner: owner, counter: fastpath.NewCounters(),
	}, nil
}

func (h *TCPPipeHandle) startListener(frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) error {
	listener, local, err := listenTCP(h.Pipe)
	if err != nil {
		return err
	}

	h.listener = listener
	h.LocalAddr = local
	done := make(chan struct{})
	h.acceptDone = done
	go h.acceptLoop(listener, done, frameKind, lengthMode, addressGuard)
	return nil
}

func (h *TCPPipeHandle) acceptLoop(listener *net.TCPListener, done chan struct{}, frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) {
	defer close(done)
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				h.setErr(fmt.Errorf("core: accept tcp %s: %w", h.Pipe.EndpointID, err))
			}
			return
		}
		h.mu.Lock()
		active := h.session != nil
		h.mu.Unlock()
		if active {
			go func() {
				_ = linkdiag.ServeStream(context.Background(), conn, h.Pipe.Binding.VKeyValue)
			}()
			continue
		}
		remote, err := tcpAddrPort(conn.RemoteAddr())
		if err != nil {
			_ = conn.Close()
			h.setErr(err)
			continue
		}
		h.mu.Lock()
		h.acceptedRemote = remote
		h.mu.Unlock()
		if err := h.startWorkerFromConn(conn, frameKind, lengthMode, addressGuard); err != nil {
			h.setErr(err)
		}
	}
}

func (h *TCPPipeHandle) startConnector(frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) error {
	if h.Pipe.ReconnectSecond <= 0 {
		_, err := h.connectRawTCP(frameKind, lengthMode, addressGuard)
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.reconnectCancel = cancel
	h.reconnectDone = done
	session, err := h.connectRawTCP(frameKind, lengthMode, addressGuard)
	if err != nil {
		h.setErr(err)
	}
	go h.rawTCPReconnectLoop(ctx, done, session, frameKind, lengthMode, addressGuard)
	return nil
}

func (h *TCPPipeHandle) connectRawTCP(frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) (*tcpWorkerSession, error) {
	tcpConn, local, remote, err := dialTCP(h.Pipe)
	if err != nil {
		return nil, err
	}
	h.LocalAddr = local
	h.RemoteAddr = remote
	if err := h.startWorkerFromConn(tcpConn, frameKind, lengthMode, addressGuard); err != nil {
		return nil, err
	}
	h.mu.Lock()
	session := h.session
	h.lastErr = nil
	h.mu.Unlock()
	return session, nil

}

func (h *TCPPipeHandle) rawTCPReconnectLoop(ctx context.Context, done chan struct{}, session *tcpWorkerSession, frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) {
	defer close(done)
	delay := time.Duration(h.Pipe.ReconnectSecond) * time.Second
	for {
		if session != nil {
			select {
			case <-ctx.Done():
				return
			case <-session.monitorDone:
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
		var err error
		session, err = h.connectRawTCP(frameKind, lengthMode, addressGuard)
		if err != nil {
			h.setErr(err)
			session = nil
		}
	}
}

func (h *TCPPipeHandle) startWorkerFromConn(conn *net.TCPConn, frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) error {
	if !h.acquireSharedDevice() {
		_ = conn.Close()
		return fmt.Errorf("core: tcp device %s already has an active stream", h.Pipe.DeviceID)
	}
	releaseDevice := true
	defer func() {
		if releaseDevice {
			h.releaseSharedDevice()
		}
	}()
	if err := configureTCPConn(conn, h.Pipe); err != nil {
		_ = conn.Close()
		return err
	}
	file, err := conn.File()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("core: duplicate tcp fd for %s: %w", h.Pipe.EndpointID, err)
	}
	_ = conn.Close()
	// Fd may restore blocking mode, so capture it once before C enables O_NONBLOCK.
	fd := int(file.Fd())

	networkToDeviceRate, deviceToNetworkRate := userTrafficRates(h.Pipe.EndpointKind, h.Pipe.Binding)
	worker, err := fastpath.StartTCPPipe(fastpath.TCPConfig{
		TUNFD:                  h.device.FD(),
		TCPFD:                  fd,
		FrameKind:              frameKind,
		MaxFrameSize:           uint32(h.Pipe.MaxFrameSize),
		LengthMode:             lengthMode,
		AddressGuardRemote:     h.Pipe.AddressGuardRemote,
		DeviceToNetworkRateBPS: deviceToNetworkRate,
		NetworkToDeviceRateBPS: networkToDeviceRate,
		VKey:                   []byte(h.Pipe.Binding.VKeyValue),
		AddressGuard:           addressGuard,
		Counters:               h.counter,
	})
	if err != nil {
		_ = file.Close()
		return err
	}

	session := &tcpWorkerSession{
		worker: worker, file: file, fd: fd, done: make(chan struct{}), monitorDone: make(chan struct{}),
	}
	h.mu.Lock()
	if h.session != nil {
		h.mu.Unlock()
		_ = session.Stop()
		return fmt.Errorf("core: tcp pipe %s already has an active worker", h.Pipe.EndpointID)
	}
	h.session = session
	h.mu.Unlock()
	releaseDevice = false
	go h.monitorWorkerSession(session)
	return nil
}

func (h *TCPPipeHandle) monitorWorkerSession(session *tcpWorkerSession) {
	defer close(session.monitorDone)
	fd := session.fd
	for {
		select {
		case <-session.done:
			h.clearWorkerSession(session)
			return
		default:
		}
		pollFDs := [1]unix.PollFd{{Fd: int32(fd), Events: unix.POLLRDHUP}}
		_, err := unix.Poll(pollFDs[:], 250)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			h.setErr(fmt.Errorf("core: monitor tcp pipe %s: %w", h.Pipe.EndpointID, err))
			_ = session.Stop()
			h.clearWorkerSession(session)
			return
		}
		if pollFDs[0].Revents&(unix.POLLRDHUP|unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
			_ = session.Stop()
			h.clearWorkerSession(session)
			return
		}
	}
}

func (h *TCPPipeHandle) clearWorkerSession(session *tcpWorkerSession) {
	h.mu.Lock()
	if h.session == session {
		h.session = nil
	}
	h.mu.Unlock()
	h.releaseSharedDevice()
}

func (h *TCPPipeHandle) Close() error {
	var firstErr error
	if h.reconnectCancel != nil {
		h.reconnectCancel()
		h.reconnectCancel = nil
	}
	if h.reconnectDone != nil {
		<-h.reconnectDone
		h.reconnectDone = nil
	}
	if h.listener != nil {
		if err := h.listener.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.listener = nil
	}
	if h.acceptDone != nil {
		<-h.acceptDone
		h.acceptDone = nil
	}
	if h.tlsCancel != nil {
		h.tlsCancel()
		h.tlsCancel = nil
	}
	h.mu.Lock()
	tlsConn := h.tlsConn
	tlsDone := h.tlsDone
	h.mu.Unlock()
	if tlsConn != nil {
		_ = tlsConn.Close()
	}
	if tlsDone != nil {
		<-tlsDone
	}

	h.mu.Lock()
	session := h.session
	h.mu.Unlock()
	if session != nil {
		if err := session.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		<-session.monitorDone
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owner && h.shared != nil {
		if h.shared.netApply != nil {
			if err := h.shared.netApply.Rollback(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.shared.netApply = nil
		}
		if h.shared.device != nil {
			if err := h.shared.device.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
			h.shared.device = nil
		}
	}
	h.device = nil
	if h.counter != nil {
		h.counter.Close()
		h.counter = nil
	}
	return firstErr
}

func (h *TCPPipeHandle) acquireSharedDevice() bool {
	if h.shared == nil {
		return true
	}
	h.shared.mu.Lock()
	defer h.shared.mu.Unlock()
	if h.shared.active {
		return false
	}
	h.shared.active = true
	return true
}

func (h *TCPPipeHandle) releaseSharedDevice() {
	if h.shared == nil {
		return
	}
	h.shared.mu.Lock()
	h.shared.active = false
	h.shared.mu.Unlock()
}

func (h *TCPPipeHandle) Counters() fastpath.CountersSnapshot {
	if h == nil {
		return fastpath.CountersSnapshot{}
	}
	if h.Pipe.TLS.Enabled || h.Pipe.ExternalXrayBridge {
		return h.tlsCounters()
	}
	if h.counter == nil {
		return fastpath.CountersSnapshot{}
	}
	return h.counter.Snapshot()
}

func (h *TCPPipeHandle) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErr
}

func (h *TCPPipeHandle) AcceptedRemoteAddr() netip.AddrPort {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.acceptedRemote
}

func (h *TCPPipeHandle) setErr(err error) {
	h.mu.Lock()
	if h.lastErr == nil {
		h.lastErr = err
	}
	h.mu.Unlock()
}

func (h *TCPPipeHandle) Diagnose(ctx context.Context, kind string, duration time.Duration) (ConnectorDiagnostic, error) {
	conn, target, err := h.openDiagnosticStream(ctx)
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	defer conn.Close()
	transport := "tcp"
	if h.Pipe.ExternalXrayBridge {
		transport = "xray"
		target = net.JoinHostPort(h.Pipe.XrayRemote, strconv.Itoa(int(h.Pipe.XrayPort)))
	}
	result := ConnectorDiagnostic{Kind: kind, Transport: transport, Target: target}
	switch kind {
	case "channel":
		result.Delay, err = linkdiag.Ping(ctx, conn, h.Pipe.Binding.VKeyValue)
	case "throughput":
		measured, measureErr := linkdiag.Throughput(ctx, conn, h.Pipe.Binding.VKeyValue, duration)
		err = measureErr
		result.Delay = measured.Delay
		result.UploadBytes = measured.UploadBytes
		result.DownloadBytes = measured.DownloadBytes
		result.UploadBPS = measured.UploadBPS
		result.DownloadBPS = measured.DownloadBPS
		result.Duration = measured.Duration
	case "path-mtu":
		if h.Pipe.ExternalXrayBridge {
			result.Delay, err = linkdiag.ProbeFrame(ctx, conn, "", h.Pipe.MaxFrameSize)
			if err == nil {
				result.PathMTU = h.Pipe.DeviceMTU
			}
		} else {
			result.TCPMSS = tcpConnMSS(conn)
			if result.TCPMSS <= 0 {
				err = errors.New("core: kernel did not report a TCP MSS for the diagnostic stream")
			}
		}
	default:
		err = fmt.Errorf("core: unsupported connector diagnostic %q", kind)
	}
	return result, err
}

func (h *TCPPipeHandle) openDiagnosticStream(ctx context.Context) (net.Conn, string, error) {
	tcpConn, _, remote, err := dialTCP(h.Pipe)
	if err != nil {
		return nil, "", err
	}
	if err := configureTCPConn(tcpConn, h.Pipe); err != nil {
		_ = tcpConn.Close()
		return nil, "", err
	}
	var conn net.Conn = tcpConn
	if h.Pipe.TLS.Enabled {
		tlsConfig, err := rawTCPClientTLSConfig(h.Pipe.TLS, h.Pipe.Remote)
		if err != nil {
			_ = tcpConn.Close()
			return nil, "", err
		}
		tlsConn := tls.Client(tcpConn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = tlsConn.Close()
			return nil, "", fmt.Errorf("core: diagnostic TLS handshake %s: %w", h.Pipe.EndpointID, err)
		}
		conn = tlsConn
	}
	return conn, remote.String(), nil
}

func tcpConnMSS(conn net.Conn) int {
	var raw syscall.RawConn
	switch value := conn.(type) {
	case *net.TCPConn:
		raw, _ = value.SyscallConn()
	case *tls.Conn:
		if tcp, ok := value.NetConn().(*net.TCPConn); ok {
			raw, _ = tcp.SyscallConn()
		}
	}
	if raw == nil {
		return 0
	}
	result := 0
	_ = raw.Control(func(fd uintptr) {
		result, _ = unix.GetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_MAXSEG)
	})
	return result
}

func configureTCPConn(conn *net.TCPConn, pipe config.RuntimeTCPPipe) error {
	if pipe.NoDelay {
		if err := conn.SetNoDelay(true); err != nil {
			return fmt.Errorf("core: set tcp nodelay: %w", err)
		}
	}
	if pipe.KeepAliveSecond > 0 {
		if err := conn.SetKeepAlive(true); err != nil {
			return fmt.Errorf("core: enable tcp keepalive: %w", err)
		}
		if err := conn.SetKeepAlivePeriod(time.Duration(pipe.KeepAliveSecond) * time.Second); err != nil {
			return fmt.Errorf("core: set tcp keepalive period: %w", err)
		}
	}
	if pipe.ReceiveBuffer > 0 {
		if err := conn.SetReadBuffer(pipe.ReceiveBuffer); err != nil {
			return fmt.Errorf("core: set tcp receive buffer: %w", err)
		}
	}
	if pipe.SendBuffer > 0 {
		if err := conn.SetWriteBuffer(pipe.SendBuffer); err != nil {
			return fmt.Errorf("core: set tcp send buffer: %w", err)
		}
	}
	return nil
}

func listenTCP(pipe config.RuntimeTCPPipe) (*net.TCPListener, netip.AddrPort, error) {
	addr, err := resolveListenTCPAddr(pipe.BindHost, pipe.BindAddress, pipe.BindPort)
	if err != nil {
		return nil, netip.AddrPort{}, err
	}
	listenConfig := net.ListenConfig{Control: tcpSocketControl(pipe, true)}
	listener, err := listenConfig.Listen(context.Background(), "tcp", addr.String())
	if err != nil {
		return nil, netip.AddrPort{}, fmt.Errorf("core: listen tcp %s: %w", addr, err)
	}
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		_ = listener.Close()
		return nil, netip.AddrPort{}, fmt.Errorf("core: listen tcp %s returned %T", addr, listener)
	}
	local, err := tcpAddrPort(tcpListener.Addr())
	if err != nil {
		_ = tcpListener.Close()
		return nil, netip.AddrPort{}, err
	}
	return tcpListener, local, nil
}

func dialTCP(pipe config.RuntimeTCPPipe) (*net.TCPConn, netip.AddrPort, netip.AddrPort, error) {
	addr := net.JoinHostPort(pipe.Remote, strconv.Itoa(int(pipe.Port)))
	timeout := time.Duration(pipe.ConnectTimeout) * time.Second
	dialer := net.Dialer{
		Timeout: timeout,
		Control: tcpSocketControl(pipe, false),
	}
	if pipe.BindAddress != "" {
		bind, err := netip.ParseAddr(pipe.BindAddress)
		if err != nil {
			return nil, netip.AddrPort{}, netip.AddrPort{}, fmt.Errorf("core: parse tcp bind address %q: %w", pipe.BindAddress, err)
		}
		dialer.LocalAddr = &net.TCPAddr{IP: net.IP(bind.AsSlice())}
	}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, netip.AddrPort{}, netip.AddrPort{}, fmt.Errorf("core: dial tcp %s: %w", addr, err)
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return nil, netip.AddrPort{}, netip.AddrPort{}, fmt.Errorf("core: dial tcp %s returned %T", addr, conn)
	}
	local, err := tcpAddrPort(tcpConn.LocalAddr())
	if err != nil {
		_ = tcpConn.Close()
		return nil, netip.AddrPort{}, netip.AddrPort{}, err
	}
	remote, err := tcpAddrPort(tcpConn.RemoteAddr())
	if err != nil {
		_ = tcpConn.Close()
		return nil, netip.AddrPort{}, netip.AddrPort{}, err
	}
	return tcpConn, local, remote, nil
}

func tcpSocketControl(pipe config.RuntimeTCPPipe, listener bool) func(string, string, syscall.RawConn) error {
	return func(_, address string, conn syscall.RawConn) error {
		var controlErr error
		if err := conn.Control(func(fd uintptr) {
			controlErr = configureRawTCPSocket(int(fd), pipe, listener)
		}); err != nil {
			return fmt.Errorf("core: control tcp socket %s: %w", address, err)
		}
		return controlErr
	}
}

func configureRawTCPSocket(fd int, pipe config.RuntimeTCPPipe, listener bool) error {
	if pipe.BindInterface != "" {
		if err := unix.SetsockoptString(fd, unix.SOL_SOCKET, unix.SO_BINDTODEVICE, pipe.BindInterface); err != nil {
			return fmt.Errorf("core: bind tcp socket to interface %q: %w", pipe.BindInterface, err)
		}
	}
	if pipe.ReceiveBuffer > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, pipe.ReceiveBuffer); err != nil {
			return fmt.Errorf("core: set tcp socket receive buffer: %w", err)
		}
	}
	if pipe.SendBuffer > 0 {
		if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, pipe.SendBuffer); err != nil {
			return fmt.Errorf("core: set tcp socket send buffer: %w", err)
		}
	}
	if pipe.FastOpen {
		opt := unix.TCP_FASTOPEN_CONNECT
		value := 1
		if listener {
			opt = unix.TCP_FASTOPEN
			value = 16
		}
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_TCP, opt, value); err != nil {
			return fmt.Errorf("core: set tcp fast open: %w", err)
		}
	}
	if pipe.TCPMaxSeg > 0 {
		if err := unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_MAXSEG, pipe.TCPMaxSeg); err != nil {
			return fmt.Errorf("core: set tcp maximum segment size: %w", err)
		}
	}
	return nil
}

func resolveListenTCPAddr(host, bindAddress string, port uint16) (*net.TCPAddr, error) {
	if bindAddress != "" {
		addr, err := netip.ParseAddr(bindAddress)
		if err != nil {
			return nil, fmt.Errorf("core: parse tcp bind address %q: %w", bindAddress, err)
		}
		return &net.TCPAddr{IP: net.IP(addr.AsSlice()), Port: int(port)}, nil
	}
	if host == "" {
		host = "0.0.0.0"
	}
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		return nil, fmt.Errorf("core: resolve tcp listen %s:%d: %w", host, port, err)
	}
	return addr, nil
}

func tcpAddrPort(addr net.Addr) (netip.AddrPort, error) {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("core: expected tcp address, got %T", addr)
	}
	ip, ok := netip.AddrFromSlice(tcp.IP)
	if !ok {
		return netip.AddrPort{}, fmt.Errorf("core: parse tcp address %s", tcp)
	}
	if tcp.Port < 0 || tcp.Port > 65535 {
		return netip.AddrPort{}, fmt.Errorf("core: tcp port out of range %d", tcp.Port)
	}
	return netip.AddrPortFrom(ip.Unmap(), uint16(tcp.Port)), nil
}
