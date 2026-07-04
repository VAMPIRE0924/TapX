//go:build linux

package core

import (
	"context"
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
	listener   *net.TCPListener
	tcpFile    *os.File
	worker     *fastpath.Worker
	counter    *fastpath.Counters
	acceptDone chan struct{}

	mu             sync.Mutex
	acceptedRemote netip.AddrPort
	lastErr        error
	tlsCancel      context.CancelFunc
	tlsDone        chan struct{}
	tlsConn        net.Conn
	tlsCounter     xrayPipeCounters
}

func startTCPPipe(pipe config.RuntimeTCPPipe, device config.RuntimeDevice) (*TCPPipeHandle, error) {
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

	handle := &TCPPipeHandle{
		Pipe:       pipe,
		DeviceName: tunDevice.Name(),
		device:     tunDevice,
		netApply:   netHandle,
		counter:    fastpath.NewCounters(),
	}

	switch pipe.EndpointKind {
	case "listener":
		var err error
		if pipe.TLS.Enabled {
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
		if pipe.TLS.Enabled {
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

func (h *TCPPipeHandle) startListener(frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) error {
	listener, local, err := listenTCP(h.Pipe)
	if err != nil {
		return err
	}

	h.listener = listener
	h.LocalAddr = local
	done := make(chan struct{})
	h.acceptDone = done
	go h.acceptOne(listener, done, frameKind, lengthMode, addressGuard)
	return nil
}

func (h *TCPPipeHandle) acceptOne(listener *net.TCPListener, done chan struct{}, frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) {
	defer close(done)
	conn, err := listener.AcceptTCP()
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			h.setErr(fmt.Errorf("core: accept tcp %s: %w", h.Pipe.EndpointID, err))
		}
		return
	}
	remote, err := tcpAddrPort(conn.RemoteAddr())
	if err != nil {
		_ = conn.Close()
		h.setErr(err)
		return
	}
	h.mu.Lock()
	h.acceptedRemote = remote
	h.mu.Unlock()
	if err := h.startWorkerFromConn(conn, frameKind, lengthMode, addressGuard); err != nil {
		h.setErr(err)
	}
}

func (h *TCPPipeHandle) startConnector(frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) error {
	tcpConn, local, remote, err := dialTCP(h.Pipe)
	if err != nil {
		return err
	}
	h.LocalAddr = local
	h.RemoteAddr = remote
	return h.startWorkerFromConn(tcpConn, frameKind, lengthMode, addressGuard)
}

func (h *TCPPipeHandle) startWorkerFromConn(conn *net.TCPConn, frameKind fastpath.FrameKind, lengthMode fastpath.TCPLengthMode, addressGuard fastpath.AddressGuard) error {
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

	worker, err := fastpath.StartTCPPipe(fastpath.TCPConfig{
		TUNFD:        h.device.FD(),
		TCPFD:        int(file.Fd()),
		FrameKind:    frameKind,
		MaxFrameSize: uint32(h.Pipe.MaxFrameSize),
		LengthMode:   lengthMode,
		VKey:         []byte(h.Pipe.Binding.VKeyValue),
		AddressGuard: addressGuard,
		Counters:     h.counter,
	})
	if err != nil {
		_ = file.Close()
		return err
	}

	h.mu.Lock()
	h.tcpFile = file
	h.worker = worker
	h.mu.Unlock()
	return nil
}

func (h *TCPPipeHandle) Close() error {
	var firstErr error
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
	defer h.mu.Unlock()
	if h.worker != nil {
		if err := h.worker.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.worker = nil
	}
	if h.tcpFile != nil {
		if err := h.tcpFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		h.tcpFile = nil
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

func (h *TCPPipeHandle) Counters() fastpath.CountersSnapshot {
	if h == nil {
		return fastpath.CountersSnapshot{}
	}
	if h.Pipe.TLS.Enabled {
		return h.tlsCounters()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.worker == nil {
		return fastpath.CountersSnapshot{}
	}
	return h.worker.Counters()
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
