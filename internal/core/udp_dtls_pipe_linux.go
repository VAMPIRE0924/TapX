//go:build linux

package core

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/pion/dtls/v3"

	"tapx/internal/fastpath"
	"tapx/internal/model"
)

func (h *UDPPipeHandle) startDTLS(frameKind fastpath.FrameKind, guard fastpath.AddressGuard, peer netip.AddrPort) error {
	switch h.Pipe.EndpointKind {
	case "listener":
		return h.startDTLSListener(frameKind, guard)
	case "connector":
		if !peer.IsValid() {
			return fmt.Errorf("core: dtls udp connector %s requires remote or fixed peer", h.Pipe.EndpointID)
		}
		return h.startDTLSConnector(frameKind, guard, peer)
	default:
		return fmt.Errorf("core: udp dtls pipe %s has unsupported endpoint kind %q", h.Pipe.EndpointID, h.Pipe.EndpointKind)
	}
}

func (h *UDPPipeHandle) startDTLSListener(frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	dtlsConfig, err := rawUDPServerDTLSConfig(h.Pipe.DTLS)
	if err != nil {
		return err
	}
	packetConn, local, err := openUDPPacketConn(h.Pipe, netip.AddrPort{})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.dtlsCancel = cancel
	h.dtlsPacketConn = packetConn
	h.LocalAddr = local
	done := make(chan struct{})
	h.dtlsDone = done
	go func() {
		defer close(done)
		h.acceptAndRunDTLS(ctx, packetConn, dtlsConfig, frameKind, guard)
		h.mu.Lock()
		if h.dtlsDone == done {
			h.dtlsDone = nil
		}
		h.mu.Unlock()
	}()
	return nil
}

func (h *UDPPipeHandle) acceptAndRunDTLS(ctx context.Context, packetConn net.PacketConn, dtlsConfig *dtls.Config, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	buffered, remote, err := acceptFirstDTLSPacket(ctx, packetConn)
	if err != nil {
		h.setDTLSErr(ctx, err)
		return
	}
	remoteAddr, err := addrPortFromNetAddr(remote)
	if err != nil {
		h.setErr(err)
		return
	}
	h.mu.Lock()
	h.acceptedRemote = remoteAddr
	h.RemoteAddr = remoteAddr
	h.mu.Unlock()
	conn, err := dtls.Server(buffered, remote, dtlsConfig)
	if err != nil {
		h.setDTLSErr(ctx, fmt.Errorf("core: dtls server handshake %s: %w", h.Pipe.EndpointID, err))
		return
	}
	h.mu.Lock()
	h.dtlsConn = conn
	h.mu.Unlock()
	h.runDTLSBridge(ctx, conn, frameKind, guard)
}

func (h *UDPPipeHandle) startDTLSConnector(frameKind fastpath.FrameKind, guard fastpath.AddressGuard, peer netip.AddrPort) error {
	dtlsConfig, err := rawUDPClientDTLSConfig(h.Pipe.DTLS, peer.Addr().String())
	if err != nil {
		return err
	}
	packetConn, local, err := openUDPPacketConn(h.Pipe, peer)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.dtlsCancel = cancel
	h.dtlsPacketConn = packetConn
	h.LocalAddr = local
	h.RemoteAddr = peer
	remote := udpAddrFromAddrPort(peer)
	conn, err := dtls.Client(packetConn, remote, dtlsConfig)
	if err != nil {
		cancel()
		_ = packetConn.Close()
		return fmt.Errorf("core: dtls client handshake %s: %w", h.Pipe.EndpointID, err)
	}
	h.mu.Lock()
	h.dtlsConn = conn
	done := make(chan struct{})
	h.dtlsDone = done
	h.mu.Unlock()
	go func() {
		defer close(done)
		h.runDTLSBridge(ctx, conn, frameKind, guard)
		h.mu.Lock()
		if h.dtlsConn == conn {
			h.dtlsConn = nil
		}
		if h.dtlsDone == done {
			h.dtlsDone = nil
		}
		h.mu.Unlock()
	}()
	return nil
}

func (h *UDPPipeHandle) runDTLSBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	defer conn.Close()
	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2)
	go func() { errc <- h.dtlsDeviceToConn(bridgeCtx, conn, frameKind, guard) }()
	go func() { errc <- h.dtlsConnToDevice(bridgeCtx, conn, frameKind, guard) }()
	err := <-errc
	cancel()
	_ = conn.Close()
	err2 := <-errc
	h.setDTLSErr(ctx, err)
	h.setDTLSErr(ctx, err2)
}

func (h *UDPPipeHandle) dtlsDeviceToConn(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	vkey := []byte(h.Pipe.Binding.VKeyValue)
	vkeyHeaderSize, err := rawVKeyHeaderSize(vkey)
	if err != nil {
		return err
	}
	maxFrame := maxPositive(h.Pipe.MaxFrameSize, 1500)
	buf := make([]byte, maxFrame)
	wire := make([]byte, maxFrame+vkeyHeaderSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := readDeviceFrame(ctx, h.device, buf)
		if err != nil {
			return err
		}
		frame := buf[:n]
		if !xrayFrameAllowed(frameKind, frame, guard) {
			h.dtlsCounter.dropsGuard.Add(1)
			continue
		}
		payload := frame
		if vkeyHeaderSize > 0 {
			payload = wire[:vkeyHeaderSize+n]
			writeRawVKeyHeader(payload[:vkeyHeaderSize], vkey)
			copy(payload[vkeyHeaderSize:], frame)
		}
		if _, err := conn.Write(payload); err != nil {
			h.dtlsCounter.dropsIO.Add(1)
			return err
		}
		h.dtlsCounter.txPackets.Add(1)
		h.dtlsCounter.txBytes.Add(uint64(n))
	}
}

func (h *UDPPipeHandle) dtlsConnToDevice(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	vkey := []byte(h.Pipe.Binding.VKeyValue)
	vkeyHeaderSize, err := rawVKeyHeaderSize(vkey)
	if err != nil {
		return err
	}
	maxFrame := maxPositive(h.Pipe.MaxFrameSize, 1500)
	wire := make([]byte, maxFrame+vkeyHeaderSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := conn.Read(wire)
		if err != nil {
			return err
		}
		frame, ok := stripRawVKeyHeader(wire[:n], vkey)
		if !ok {
			h.dtlsCounter.dropsGuard.Add(1)
			continue
		}
		if !xrayFrameAllowed(frameKind, frame, guard) {
			h.dtlsCounter.dropsGuard.Add(1)
			continue
		}
		if _, err := h.device.Write(frame); err != nil {
			h.dtlsCounter.dropsIO.Add(1)
			return err
		}
		h.dtlsCounter.rxPackets.Add(1)
		h.dtlsCounter.rxBytes.Add(uint64(len(frame)))
	}
}

func (h *UDPPipeHandle) setDTLSErr(ctx context.Context, err error) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	h.setErr(err)
}

func (h *UDPPipeHandle) dtlsCounters() fastpath.CountersSnapshot {
	return fastpath.CountersSnapshot{
		RXPackets:  h.dtlsCounter.rxPackets.Load(),
		TXPackets:  h.dtlsCounter.txPackets.Load(),
		RXBytes:    h.dtlsCounter.rxBytes.Load(),
		TXBytes:    h.dtlsCounter.txBytes.Load(),
		DropsGuard: h.dtlsCounter.dropsGuard.Load(),
		DropsIO:    h.dtlsCounter.dropsIO.Load(),
	}
}

func rawUDPServerDTLSConfig(settings model.RawDTLSSettings) (*dtls.Config, error) {
	cert, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("core: load raw udp dtls certificate: %w", err)
	}
	cfg := &dtls.Config{
		Certificates:            []tls.Certificate{cert},
		SupportedProtocols:      cleanALPN(settings.ALPN),
		MTU:                     settings.MTU,
		ReplayProtectionWindow:  settings.ReplayWindow,
		InsecureSkipVerifyHello: settings.AllowInsecure,
	}
	if settings.CAFile != "" {
		pool, err := loadCertPool(settings.CAFile)
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
		if settings.AllowInsecure {
			cfg.ClientAuth = dtls.RequestClientCert
		} else {
			cfg.ClientAuth = dtls.RequireAndVerifyClientCert
		}
	}
	return cfg, nil
}

func rawUDPClientDTLSConfig(settings model.RawDTLSSettings, remote string) (*dtls.Config, error) {
	cfg := &dtls.Config{
		ServerName:              firstNonEmpty(settings.ServerName, remote),
		InsecureSkipVerify:      settings.AllowInsecure,
		SupportedProtocols:      cleanALPN(settings.ALPN),
		MTU:                     settings.MTU,
		ReplayProtectionWindow:  settings.ReplayWindow,
		InsecureSkipVerifyHello: settings.AllowInsecure,
	}
	if settings.CAFile != "" {
		pool, err := loadCertPool(settings.CAFile)
		if err != nil {
			return nil, err
		}
		cfg.RootCAs = pool
	}
	if settings.CertFile != "" || settings.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("core: load raw udp dtls client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func acceptFirstDTLSPacket(ctx context.Context, packetConn net.PacketConn) (net.PacketConn, net.Addr, error) {
	buf := make([]byte, 8192)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		_ = packetConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, remote, err := packetConn.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return nil, nil, fmt.Errorf("core: read first dtls packet: %w", err)
		}
		_ = packetConn.SetReadDeadline(time.Time{})
		first := make([]byte, n)
		copy(first, buf[:n])
		return &firstPacketConn{PacketConn: packetConn, remote: remote, first: first}, remote, nil
	}
}

type firstPacketConn struct {
	net.PacketConn
	remote net.Addr
	first  []byte
}

func (c *firstPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if c.first != nil {
		n := copy(p, c.first)
		c.first = nil
		return n, c.remote, nil
	}
	for {
		n, addr, err := c.PacketConn.ReadFrom(p)
		if err != nil {
			return n, addr, err
		}
		if addr.String() == c.remote.String() {
			return n, addr, nil
		}
	}
}

func udpAddrFromAddrPort(addr netip.AddrPort) *net.UDPAddr {
	return &net.UDPAddr{IP: net.IP(addr.Addr().AsSlice()), Port: int(addr.Port())}
}

func addrPortFromNetAddr(addr net.Addr) (netip.AddrPort, error) {
	host, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("core: split addr %q: %w", addr, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("core: parse addr %q: %w", host, err)
	}
	port, err := net.LookupPort("udp", portText)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("core: parse port %q: %w", portText, err)
	}
	if port < 0 || port > 65535 {
		return netip.AddrPort{}, fmt.Errorf("core: port out of range %d", port)
	}
	return netip.AddrPortFrom(ip.Unmap(), uint16(port)), nil
}
