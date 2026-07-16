//go:build linux

package core

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"

	"github.com/pion/dtls/v3"

	"tapx/internal/fastpath"
	"tapx/internal/linkdiag"
	"tapx/internal/model"
	"tapx/internal/pathmtu"
)

const (
	dtlsVKeyHelloMagic = "TXH1"
	dtlsVKeyDiagMagic  = "TXD2"
	dtlsVKeyAckMagic   = "TXA1"
	dtlsVKeyHeaderSize = 8
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
	dtlsOptions, err := rawUDPServerDTLSOptions(h.Pipe.DTLS)
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
		h.acceptAndRunDTLS(ctx, packetConn, dtlsOptions, frameKind, guard)
		h.mu.Lock()
		if h.dtlsDone == done {
			h.dtlsDone = nil
		}
		h.mu.Unlock()
	}()
	return nil
}

func (h *UDPPipeHandle) acceptAndRunDTLS(ctx context.Context, packetConn net.PacketConn, dtlsOptions []dtls.ServerOption, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
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
	conn, err := dtls.ServerWithOptions(buffered, remote, dtlsOptions...)
	if err != nil {
		h.setDTLSErr(ctx, fmt.Errorf("core: dtls server handshake %s: %w", h.Pipe.EndpointID, err))
		return
	}
	handshakeCtx, cancelHandshake := h.dtlsHandshakeContext(ctx)
	if err := conn.HandshakeContext(handshakeCtx); err != nil {
		cancelHandshake()
		h.setDTLSErr(ctx, fmt.Errorf("core: dtls server handshake %s: %w", h.Pipe.EndpointID, err))
		_ = conn.Close()
		return
	}
	cancelHandshake()
	hello, err := receiveDTLSVKeyHello(ctx, conn, h.dtlsHandshakeTimeout())
	if err != nil {
		h.setDTLSErr(ctx, fmt.Errorf("core: receive DTLS vKey hello %s: %w", h.Pipe.EndpointID, err))
		_ = conn.Close()
		return
	}
	if hello.vkey != h.Pipe.Binding.VKeyValue {
		h.setDTLSErr(ctx, fmt.Errorf("core: DTLS vKey does not match route for %s", h.Pipe.EndpointID))
		_ = conn.Close()
		return
	}
	if hello.diagnostic {
		if err := linkdiag.ServeStream(ctx, conn, hello.vkey); err != nil {
			h.setDTLSErr(ctx, fmt.Errorf("core: serve DTLS diagnostic %s: %w", h.Pipe.EndpointID, err))
		}
		_ = conn.Close()
		return
	}
	h.mu.Lock()
	h.dtlsConn = conn
	h.mu.Unlock()
	if err := h.confirmDTLSPath(ctx, conn, remoteAddr); err != nil {
		h.setDTLSErr(ctx, err)
		_ = conn.Close()
		return
	}
	h.runDTLSBridge(ctx, conn, frameKind, guard)
}

func (h *UDPPipeHandle) startDTLSConnector(frameKind fastpath.FrameKind, guard fastpath.AddressGuard, peer netip.AddrPort) error {
	dtlsOptions, err := rawUDPClientDTLSOptions(h.Pipe.DTLS, peer.Addr().String())
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
	conn, err := dtls.ClientWithOptions(packetConn, remote, dtlsOptions...)
	if err != nil {
		cancel()
		_ = packetConn.Close()
		return fmt.Errorf("core: dtls client handshake %s: %w", h.Pipe.EndpointID, err)
	}
	handshakeCtx, cancelHandshake := h.dtlsHandshakeContext(ctx)
	if err := conn.HandshakeContext(handshakeCtx); err != nil {
		cancelHandshake()
		cancel()
		_ = conn.Close()
		_ = packetConn.Close()
		return fmt.Errorf("core: dtls client handshake %s: %w", h.Pipe.EndpointID, err)
	}
	cancelHandshake()
	if err := sendDTLSVKeyHello(ctx, conn, h.Pipe.Binding.VKeyValue, h.dtlsHandshakeTimeout()); err != nil {
		cancel()
		_ = conn.Close()
		_ = packetConn.Close()
		return fmt.Errorf("core: send DTLS vKey hello %s: %w", h.Pipe.EndpointID, err)
	}
	h.mu.Lock()
	h.dtlsConn = conn
	h.mu.Unlock()
	if err := h.confirmDTLSPath(ctx, conn, peer); err != nil {
		cancel()
		_ = conn.Close()
		_ = packetConn.Close()
		h.mu.Lock()
		if h.dtlsConn == conn {
			h.dtlsConn = nil
		}
		h.mu.Unlock()
		h.dtlsPacketConn = nil
		h.dtlsCancel = nil
		return err
	}
	done := make(chan struct{})
	h.mu.Lock()
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

func (h *UDPPipeHandle) confirmDTLSPath(parent context.Context, conn *dtls.Conn, peer netip.AddrPort) error {
	if !h.Pipe.LinkAutoOptimize || h.Pipe.MaxDatagramPayload > pathmtu.SegmentHeaderSize {
		return nil
	}
	timeout := time.Duration(h.Pipe.ConnectTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("core: complete DTLS handshake for %s: %w", h.Pipe.EndpointID, err)
	}
	overhead, err := dtlsRecordOverhead(conn)
	if err != nil {
		return fmt.Errorf("core: determine DTLS record overhead for %s: %w", h.Pipe.EndpointID, err)
	}
	pipe, err := h.pathPreparer.prepareConn(ctx, h.Pipe, h.runtimeDevice, conn, peer, overhead)
	if err != nil {
		return err
	}
	if err := h.netApply.SetMSSClamp(pipe.TCPMSSIPv4, pipe.TCPMSSIPv6); err != nil {
		return fmt.Errorf("core: apply confirmed DTLS MSS for %s: %w", h.Pipe.EndpointID, err)
	}
	h.mu.Lock()
	h.Pipe = pipe
	h.mu.Unlock()
	return nil
}

func (h *UDPPipeHandle) dtlsHandshakeTimeout() time.Duration {
	timeout := time.Duration(h.Pipe.ConnectTimeout) * time.Second
	if timeout <= 0 {
		return 10 * time.Second
	}
	return timeout
}

func (h *UDPPipeHandle) dtlsHandshakeContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, h.dtlsHandshakeTimeout())
}

func sendDTLSVKeyHello(ctx context.Context, conn net.Conn, vkey string, timeout time.Duration) error {
	return sendDTLSVKeyControl(ctx, conn, dtlsVKeyHelloMagic, vkey, timeout)
}

func sendDTLSDiagnosticHello(ctx context.Context, conn net.Conn, vkey string, timeout time.Duration) error {
	return sendDTLSVKeyControl(ctx, conn, dtlsVKeyDiagMagic, vkey, timeout)
}

func sendDTLSVKeyControl(ctx context.Context, conn net.Conn, magic, vkey string, timeout time.Duration) error {
	packet, err := encodeDTLSVKeyControl(magic, vkey)
	if err != nil {
		return err
	}
	if err := setConnContextDeadline(ctx, conn, timeout); err != nil {
		return err
	}
	defer conn.SetDeadline(time.Time{})
	if _, err := conn.Write(packet); err != nil {
		return err
	}
	response := make([]byte, len(packet))
	n, err := conn.Read(response)
	if err != nil {
		return err
	}
	want, _ := encodeDTLSVKeyControl(dtlsVKeyAckMagic, vkey)
	if n != len(want) || string(response[:n]) != string(want) {
		return errors.New("invalid DTLS vKey acknowledgement")
	}
	return nil
}

type dtlsVKeyHello struct {
	vkey       string
	diagnostic bool
}

func receiveDTLSVKeyHello(ctx context.Context, conn net.Conn, timeout time.Duration) (dtlsVKeyHello, error) {
	if err := setConnContextDeadline(ctx, conn, timeout); err != nil {
		return dtlsVKeyHello{}, err
	}
	defer conn.SetDeadline(time.Time{})
	packet := make([]byte, dtlsVKeyHeaderSize+rawVKeyMaxSize)
	n, err := conn.Read(packet)
	if err != nil {
		return dtlsVKeyHello{}, err
	}
	if n < 4 {
		return dtlsVKeyHello{}, errors.New("invalid DTLS vKey control header")
	}
	magic := string(packet[:4])
	if magic != dtlsVKeyHelloMagic && magic != dtlsVKeyDiagMagic {
		return dtlsVKeyHello{}, errors.New("invalid DTLS vKey control magic")
	}
	vkey, err := decodeDTLSVKeyControl(packet[:n], magic)
	if err != nil {
		return dtlsVKeyHello{}, err
	}
	ack, _ := encodeDTLSVKeyControl(dtlsVKeyAckMagic, vkey)
	if _, err := conn.Write(ack); err != nil {
		return dtlsVKeyHello{}, err
	}
	return dtlsVKeyHello{vkey: vkey, diagnostic: magic == dtlsVKeyDiagMagic}, nil
}

func encodeDTLSVKeyControl(magic, vkey string) ([]byte, error) {
	key := []byte(vkey)
	if len(key) > rawVKeyMaxSize {
		return nil, fmt.Errorf("DTLS vKey is too long: %d", len(key))
	}
	packet := make([]byte, dtlsVKeyHeaderSize+len(key))
	copy(packet[:4], magic)
	binary.BigEndian.PutUint16(packet[4:6], uint16(len(key)))
	copy(packet[dtlsVKeyHeaderSize:], key)
	return packet, nil
}

func decodeDTLSVKeyControl(packet []byte, magic string) (string, error) {
	if len(packet) < dtlsVKeyHeaderSize || string(packet[:4]) != magic || packet[6] != 0 || packet[7] != 0 {
		return "", errors.New("invalid DTLS vKey control header")
	}
	keySize := int(binary.BigEndian.Uint16(packet[4:6]))
	if keySize > rawVKeyMaxSize || len(packet) != dtlsVKeyHeaderSize+keySize {
		return "", errors.New("invalid DTLS vKey control length")
	}
	return string(packet[dtlsVKeyHeaderSize:]), nil
}

func setConnContextDeadline(ctx context.Context, conn net.Conn, fallback time.Duration) error {
	deadline := time.Now().Add(fallback)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	return conn.SetDeadline(deadline)
}

func dtlsRecordOverhead(conn *dtls.Conn) (int, error) {
	if conn == nil {
		return 0, fmt.Errorf("DTLS connection is required")
	}
	state, ok := conn.ConnectionState()
	if !ok {
		return 0, fmt.Errorf("DTLS connection state is unavailable")
	}
	return dtlsCipherSuiteRecordOverhead(state.CipherSuiteID)
}

// The result includes the 13-byte DTLS 1.2 record header and the negotiated
// cipher expansion. CBC uses its worst possible padding so a confirmed plan
// cannot occasionally cross the path MTU for a different frame length.
func dtlsCipherSuiteRecordOverhead(id dtls.CipherSuiteID) (int, error) {
	switch id {
	case dtls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		dtls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		dtls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		dtls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM,
		dtls.TLS_PSK_WITH_AES_128_CCM,
		dtls.TLS_PSK_WITH_AES_128_GCM_SHA256:
		return 37, nil // record header + explicit nonce + 16-byte tag
	case dtls.TLS_ECDHE_ECDSA_WITH_AES_128_CCM_8,
		dtls.TLS_PSK_WITH_AES_128_CCM_8,
		dtls.TLS_PSK_WITH_AES_256_CCM_8,
		dtls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
		dtls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		dtls.TLS_PSK_WITH_CHACHA20_POLY1305_SHA256:
		return 29, nil // record header + 16 bytes of tag/nonce expansion
	case dtls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
		dtls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA:
		return 65, nil // record header + IV + SHA-1 MAC + worst padding
	case dtls.TLS_PSK_WITH_AES_128_CBC_SHA256,
		dtls.TLS_ECDHE_PSK_WITH_AES_128_CBC_SHA256:
		return 77, nil // record header + IV + SHA-256 MAC + worst padding
	default:
		return 0, fmt.Errorf("unsupported DTLS cipher suite %s", dtls.CipherSuiteName(id))
	}
}

func (h *UDPPipeHandle) runDTLSBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	conn = applyUserRateLimits(conn, h.Pipe.EndpointKind, h.Pipe.Binding)
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
	wireSize := maxFrame + vkeyHeaderSize
	var segmenter *pathmtu.Segmenter
	if h.Pipe.MaxDatagramPayload > 0 {
		if h.Pipe.MaxDatagramPayload <= vkeyHeaderSize {
			return fmt.Errorf("core: DTLS datagram payload %d does not fit vKey header %d", h.Pipe.MaxDatagramPayload, vkeyHeaderSize)
		}
		segmenter, err = pathmtu.NewSegmenter(h.Pipe.MaxDatagramPayload - vkeyHeaderSize)
		if err != nil {
			return fmt.Errorf("core: create DTLS segmenter: %w", err)
		}
		wireSize = max(h.Pipe.MaxDatagramPayload, maxFrame+vkeyHeaderSize+pathmtu.SegmentHeaderSize)
	}
	wire := make([]byte, wireSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := readDeviceFrame(ctx, h.device, buf)
		if err != nil {
			return err
		}
		frame := buf[:n]
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, true)) {
			h.dtlsCounter.dropsGuard.Add(1)
			continue
		}
		writePayload := func(payload []byte) error {
			if vkeyHeaderSize > 0 {
				withVKey := wire[:vkeyHeaderSize+len(payload)]
				writeRawVKeyHeader(withVKey[:vkeyHeaderSize], vkey)
				copy(withVKey[vkeyHeaderSize:], payload)
				payload = withVKey
			}
			written, err := conn.Write(payload)
			if err != nil {
				return err
			}
			if written != len(payload) {
				return io.ErrShortWrite
			}
			return nil
		}
		if segmenter != nil {
			err = segmenter.WriteFrame(frame, writePayload)
		} else {
			err = writePayload(frame)
		}
		if err != nil {
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
	wireSize := maxFrame + vkeyHeaderSize
	var reassembler *pathmtu.Reassembler
	if h.Pipe.MaxDatagramPayload > 0 {
		if h.Pipe.MaxDatagramPayload <= vkeyHeaderSize {
			return fmt.Errorf("core: DTLS datagram payload %d does not fit vKey header %d", h.Pipe.MaxDatagramPayload, vkeyHeaderSize)
		}
		reassembler, err = pathmtu.NewReassembler(maxFrame)
		if err != nil {
			return fmt.Errorf("core: create DTLS reassembler: %w", err)
		}
		wireSize = max(h.Pipe.MaxDatagramPayload, maxFrame+vkeyHeaderSize+pathmtu.SegmentHeaderSize)
	}
	wire := make([]byte, wireSize)
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
		if reassembler != nil {
			var complete bool
			frame, complete, err = reassembler.Push(frame)
			if err != nil {
				h.dtlsCounter.dropsGuard.Add(1)
				continue
			}
			if !complete {
				continue
			}
		}
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, false)) {
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

func rawUDPServerDTLSOptions(settings model.RawDTLSSettings) ([]dtls.ServerOption, error) {
	cert, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("core: load raw udp dtls certificate: %w", err)
	}
	options := []dtls.ServerOption{
		dtls.WithCertificates(cert),
		dtls.WithInsecureSkipVerifyHello(settings.AllowInsecure),
	}
	if protocols := cleanALPN(settings.ALPN); len(protocols) > 0 {
		options = append(options, dtls.WithSupportedProtocols(protocols...))
	}
	if settings.MTU > 0 {
		options = append(options, dtls.WithMTU(settings.MTU))
	}
	if settings.ReplayWindow > 0 {
		options = append(options, dtls.WithReplayProtectionWindow(settings.ReplayWindow))
	}
	if settings.CAFile != "" {
		pool, err := loadCertPool(settings.CAFile)
		if err != nil {
			return nil, err
		}
		options = append(options, dtls.WithClientCAs(pool))
		if settings.AllowInsecure {
			options = append(options, dtls.WithClientAuth(dtls.RequestClientCert))
		} else {
			options = append(options, dtls.WithClientAuth(dtls.RequireAndVerifyClientCert))
		}
	}
	return options, nil
}

func rawUDPClientDTLSOptions(settings model.RawDTLSSettings, remote string) ([]dtls.ClientOption, error) {
	options := []dtls.ClientOption{
		dtls.WithServerName(firstNonEmpty(settings.ServerName, remote)),
		dtls.WithInsecureSkipVerify(settings.AllowInsecure),
	}
	if protocols := cleanALPN(settings.ALPN); len(protocols) > 0 {
		options = append(options, dtls.WithSupportedProtocols(protocols...))
	}
	if settings.MTU > 0 {
		options = append(options, dtls.WithMTU(settings.MTU))
	}
	if settings.ReplayWindow > 0 {
		options = append(options, dtls.WithReplayProtectionWindow(settings.ReplayWindow))
	}
	if settings.CAFile != "" {
		pool, err := loadCertPool(settings.CAFile)
		if err != nil {
			return nil, err
		}
		options = append(options, dtls.WithRootCAs(pool))
	}
	if settings.CertFile != "" || settings.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("core: load raw udp dtls client certificate: %w", err)
		}
		options = append(options, dtls.WithCertificates(cert))
	}
	return options, nil
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
