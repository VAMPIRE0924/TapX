//go:build linux

package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"tapx/internal/fastpath"
	"tapx/internal/model"
	"tapx/internal/rawtcp"
)

const (
	rawVKeyMagic          = "TXV1"
	rawVKeyHeaderBaseSize = 8
)

func (h *TCPPipeHandle) startTLSListener(frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	tlsConfig, err := rawTCPServerTLSConfig(h.Pipe.TLS)
	if err != nil {
		return err
	}
	listener, local, err := listenTCP(h.Pipe)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.tlsCancel = cancel
	h.listener = listener
	h.LocalAddr = local
	done := make(chan struct{})
	h.acceptDone = done
	go h.acceptTLSLoop(ctx, listener, done, frameKind, guard, tlsConfig)
	return nil
}

func (h *TCPPipeHandle) acceptTLSLoop(ctx context.Context, listener *net.TCPListener, done chan struct{}, frameKind fastpath.FrameKind, guard fastpath.AddressGuard, tlsConfig *tls.Config) {
	defer close(done)
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				h.setErr(fmt.Errorf("core: accept tls tcp %s: %w", h.Pipe.EndpointID, err))
			}
			return
		}
		if err := configureTCPConn(conn, h.Pipe); err != nil {
			_ = conn.Close()
			h.setErr(err)
			continue
		}
		tlsConn := tls.Server(conn, tlsConfig)
		var stream net.Conn = tlsConn
		if h.address.isServer() {
			handshakeCtx, stopHandshake := h.tlsHandshakeContext(ctx)
			handshakeErr := tlsConn.HandshakeContext(handshakeCtx)
			stopHandshake()
			if handshakeErr != nil {
				_ = tlsConn.Close()
				h.setErr(handshakeErr)
				continue
			}
			reader := bufio.NewReaderSize(tlsConn, 4+rawVKeyHeaderBaseSize+rawVKeyMaxSize)
			probe, probeErr := peekTCPDispatchReader(reader, h.Pipe.LengthMode, h.Pipe.MaxFrameSize)
			if probeErr != nil {
				_ = tlsConn.Close()
				h.setErr(probeErr)
				continue
			}
			if probe.Diagnostic {
				stream := &bufferedDispatchConn{Conn: tlsConn, reader: reader}
				go func() { _ = serveAddressControl(ctx, stream, probe.VKey, h.address) }()
				continue
			}
			stream = &bufferedDispatchConn{Conn: tlsConn, reader: reader}
		}
		h.mu.Lock()
		active := h.tlsDone != nil
		h.mu.Unlock()
		if active {
			go func() {
				_ = serveAddressControl(ctx, stream, h.Pipe.Binding.VKeyValue, h.address)
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
		if _, err := h.startTLSBridge(ctx, stream, frameKind, guard); err != nil {
			h.setErr(err)
		}
	}
}

func (h *TCPPipeHandle) startTLSConnector(frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	tlsConfig, err := rawTCPClientTLSConfig(h.Pipe.TLS, h.Pipe.Remote)
	if err != nil {
		return err
	}
	if h.Pipe.ReconnectSecond <= 0 {
		ctx, cancel := context.WithCancel(context.Background())
		h.tlsCancel = cancel
		if _, err := h.connectTLS(ctx, frameKind, guard, tlsConfig); err != nil {
			cancel()
			return err
		}
		h.address.startRenewal(func(renewCtx context.Context) error {
			return h.ensureTLSAddressLease(renewCtx, tlsConfig)
		}, h.setErr)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.reconnectCancel = cancel
	h.reconnectDone = done
	sessionDone, err := h.connectTLS(ctx, frameKind, guard, tlsConfig)
	if err != nil {
		h.setErr(err)
	}
	h.address.startRenewal(func(renewCtx context.Context) error {
		return h.ensureTLSAddressLease(renewCtx, tlsConfig)
	}, h.setErr)
	go h.tlsReconnectLoop(ctx, done, sessionDone, frameKind, guard, tlsConfig)
	return nil
}

func (h *TCPPipeHandle) connectTLS(ctx context.Context, frameKind fastpath.FrameKind, guard fastpath.AddressGuard, tlsConfig *tls.Config) (<-chan struct{}, error) {
	if err := h.ensureTLSAddressLease(ctx, tlsConfig); err != nil {
		return nil, err
	}
	tcpConn, local, remote, err := dialTCP(h.Pipe)
	if err != nil {
		return nil, err
	}
	if err := configureTCPConn(tcpConn, h.Pipe); err != nil {
		_ = tcpConn.Close()
		return nil, err
	}
	tlsConn := tls.Client(tcpConn, tlsConfig)
	handshakeCtx, stopHandshake := h.tlsHandshakeContext(ctx)
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		stopHandshake()
		_ = tlsConn.Close()
		return nil, fmt.Errorf("core: tls handshake %s: %w", h.Pipe.EndpointID, err)
	}
	stopHandshake()
	h.LocalAddr = local
	h.RemoteAddr = remote
	done, err := h.startTLSBridge(ctx, tlsConn, frameKind, guard)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.lastErr = nil
	h.mu.Unlock()
	return done, nil
}

func (h *TCPPipeHandle) ensureTLSAddressLease(ctx context.Context, tlsConfig *tls.Config) error {
	if !h.address.isClient() {
		return nil
	}
	tcpConn, _, _, err := dialTCP(h.Pipe)
	if err != nil {
		return err
	}
	if err := configureTCPConn(tcpConn, h.Pipe); err != nil {
		_ = tcpConn.Close()
		return err
	}
	conn := tls.Client(tcpConn, tlsConfig)
	defer conn.Close()
	handshakeCtx, stop := h.tlsHandshakeContext(ctx)
	defer stop()
	if err := conn.HandshakeContext(handshakeCtx); err != nil {
		return fmt.Errorf("core: address control TLS handshake %s: %w", h.Pipe.EndpointID, err)
	}
	return h.address.requestLease(handshakeCtx, conn, h.Pipe.Binding.VKeyValue, addressLeaseKey(h.Pipe.Binding, h.Pipe.DeviceID, h.Pipe.EndpointID))
}

func (h *TCPPipeHandle) tlsReconnectLoop(ctx context.Context, loopDone chan struct{}, sessionDone <-chan struct{}, frameKind fastpath.FrameKind, guard fastpath.AddressGuard, tlsConfig *tls.Config) {
	defer close(loopDone)
	delay := time.Duration(h.Pipe.ReconnectSecond) * time.Second
	for {
		if sessionDone != nil {
			select {
			case <-ctx.Done():
				return
			case <-sessionDone:
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
		sessionDone, err = h.connectTLS(ctx, frameKind, guard, tlsConfig)
		if err != nil {
			h.setErr(err)
			sessionDone = nil
		}
	}
}

func (h *TCPPipeHandle) startTLSBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) (<-chan struct{}, error) {
	if _, err := rawVKeyHeaderSize([]byte(h.Pipe.Binding.VKeyValue)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !h.acquireSharedDevice() {
		_ = conn.Close()
		return nil, fmt.Errorf("core: device %s already has an active stream", h.Pipe.DeviceID)
	}
	h.mu.Lock()
	if h.tlsDone != nil {
		h.mu.Unlock()
		h.releaseSharedDevice()
		_ = conn.Close()
		return nil, fmt.Errorf("core: tls tcp pipe %s already has an active stream", h.Pipe.EndpointID)
	}
	done := make(chan struct{})
	h.tlsDone = done
	h.tlsConn = conn
	h.mu.Unlock()

	go func() {
		defer close(done)
		defer h.releaseSharedDevice()
		h.runTLSBridge(ctx, conn, frameKind, guard)
		h.mu.Lock()
		if h.tlsConn == conn {
			h.tlsConn = nil
		}
		if h.tlsDone == done {
			h.tlsDone = nil
		}
		h.mu.Unlock()
	}()
	return done, nil
}

func (h *TCPPipeHandle) runTLSBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	if tlsConn, ok := conn.(*tls.Conn); ok {
		handshakeCtx, stopHandshake := h.tlsHandshakeContext(ctx)
		err := tlsConn.HandshakeContext(handshakeCtx)
		stopHandshake()
		if err != nil {
			h.setTLSBridgeErr(ctx, fmt.Errorf("core: tls handshake %s: %w", h.Pipe.EndpointID, err))
			return
		}
	}
	conn = applyUserRateLimits(conn, h.Pipe.EndpointKind, h.Pipe.Binding)
	defer conn.Close()
	bridgeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2)
	go func() { errc <- h.tlsDeviceToConn(bridgeCtx, conn, frameKind, guard) }()
	go func() { errc <- h.tlsConnToDevice(bridgeCtx, conn, frameKind, guard) }()
	err := <-errc
	cancel()
	_ = conn.Close()
	err2 := <-errc
	h.setTLSBridgeErr(ctx, err)
	h.setTLSBridgeErr(ctx, err2)
}

func (h *TCPPipeHandle) tlsDeviceToConn(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	vkey := []byte(h.Pipe.Binding.VKeyValue)
	vkeyHeaderSize, err := rawVKeyHeaderSize(vkey)
	if err != nil {
		return err
	}
	maxFrame := maxPositive(h.Pipe.MaxFrameSize, rawtcp.DefaultMaxFrameSize)
	frameHeaderSize, err := rawtcp.FrameHeaderSize(h.Pipe.LengthMode)
	if err != nil {
		return err
	}
	wire := make([]byte, frameHeaderSize+vkeyHeaderSize+maxFrame)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := readDeviceFrame(ctx, h.device, wire[frameHeaderSize+vkeyHeaderSize:])
		if err != nil {
			return err
		}
		frame := wire[frameHeaderSize+vkeyHeaderSize : frameHeaderSize+vkeyHeaderSize+n]
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, true)) {
			h.tlsCounter.dropsGuard.Add(1)
			continue
		}
		if vkeyHeaderSize > 0 {
			writeRawVKeyHeader(wire[frameHeaderSize:frameHeaderSize+vkeyHeaderSize], vkey)
		}
		if _, err := rawtcp.EncodeFrameHeader(wire[:frameHeaderSize], h.Pipe.LengthMode, vkeyHeaderSize+n); err != nil {
			return err
		}
		if err := writeFull(conn, wire[:frameHeaderSize+vkeyHeaderSize+n]); err != nil {
			h.tlsCounter.dropsIO.Add(1)
			return err
		}
		h.tlsCounter.txPackets.Add(1)
		h.tlsCounter.txBytes.Add(uint64(n))
	}
}

func (h *TCPPipeHandle) tlsConnToDevice(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	vkey := []byte(h.Pipe.Binding.VKeyValue)
	vkeyHeaderSize, err := rawVKeyHeaderSize(vkey)
	if err != nil {
		return err
	}
	maxFrame := maxPositive(h.Pipe.MaxFrameSize, rawtcp.DefaultMaxFrameSize)
	readMax := maxFrame + vkeyHeaderSize
	wire := make([]byte, readMax)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		wireFrame, err := rawtcp.ReadFrameInto(conn, h.Pipe.LengthMode, wire, readMax)
		if err != nil {
			return err
		}
		frame, ok := stripRawVKeyHeader(wireFrame, vkey)
		if !ok {
			h.tlsCounter.dropsGuard.Add(1)
			continue
		}
		if !xrayFrameAllowed(frameKind, frame, guard, addressGuardSource(h.Pipe.AddressGuardRemote, false)) {
			h.tlsCounter.dropsGuard.Add(1)
			continue
		}
		if _, err := h.device.Write(frame); err != nil {
			h.tlsCounter.dropsIO.Add(1)
			return err
		}
		h.tlsCounter.rxPackets.Add(1)
		h.tlsCounter.rxBytes.Add(uint64(len(frame)))
	}
}

func writeFull(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		payload = payload[n:]
	}
	return nil
}

func (h *TCPPipeHandle) tlsHandshakeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := h.Pipe.ConnectTimeout
	if timeout <= 0 {
		timeout = 10
	}
	return context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
}

func (h *TCPPipeHandle) setTLSBridgeErr(ctx context.Context, err error) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	h.setErr(err)
}

func (h *TCPPipeHandle) tlsCounters() fastpath.CountersSnapshot {
	return fastpath.CountersSnapshot{
		RXPackets:  h.tlsCounter.rxPackets.Load(),
		TXPackets:  h.tlsCounter.txPackets.Load(),
		RXBytes:    h.tlsCounter.rxBytes.Load(),
		TXBytes:    h.tlsCounter.txBytes.Load(),
		DropsGuard: h.tlsCounter.dropsGuard.Load(),
		DropsIO:    h.tlsCounter.dropsIO.Load(),
	}
}

func rawTCPServerTLSConfig(settings model.RawTLSSettings) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("core: load raw tcp tls certificate: %w", err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	if settings.ServerName != "" {
		cfg.ServerName = settings.ServerName
	}
	if err := applyTLSVersions(cfg, settings.MinVersion, settings.MaxVersion); err != nil {
		return nil, err
	}
	return cfg, nil
}

func rawTCPClientTLSConfig(settings model.RawTLSSettings, remote string) (*tls.Config, error) {
	cfg := &tls.Config{
		ServerName:         firstNonEmpty(settings.ServerName, remote),
		InsecureSkipVerify: settings.AllowInsecure,
	}
	if err := applyTLSVersions(cfg, settings.MinVersion, settings.MaxVersion); err != nil {
		return nil, err
	}
	return cfg, nil
}

func applyTLSVersions(cfg *tls.Config, minVersion string, maxVersion string) error {
	min, err := parseTLSVersion(minVersion)
	if err != nil {
		return err
	}
	max, err := parseTLSVersion(maxVersion)
	if err != nil {
		return err
	}
	cfg.MinVersion = min
	cfg.MaxVersion = max
	return nil
}

func parseTLSVersion(value string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return 0, nil
	case "1.0", "tls1.0":
		return tls.VersionTLS10, nil
	case "1.1", "tls1.1":
		return tls.VersionTLS11, nil
	case "1.2", "tls1.2":
		return tls.VersionTLS12, nil
	case "1.3", "tls1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("core: unsupported tls version %q", value)
	}
}

func rawVKeyHeaderSize(vkey []byte) (int, error) {
	if len(vkey) == 0 {
		return 0, nil
	}
	if len(vkey) > 1024 {
		return 0, fmt.Errorf("core: raw vKey length %d exceeds 1024 bytes", len(vkey))
	}
	return rawVKeyHeaderBaseSize + len(vkey), nil
}

func writeRawVKeyHeader(dst []byte, vkey []byte) {
	if len(vkey) == 0 {
		return
	}
	copy(dst[:4], rawVKeyMagic)
	binary.BigEndian.PutUint16(dst[4:6], uint16(len(vkey)))
	dst[6] = 0
	dst[7] = 0
	copy(dst[rawVKeyHeaderBaseSize:], vkey)
}

func stripRawVKeyHeader(frame []byte, vkey []byte) ([]byte, bool) {
	if len(vkey) == 0 {
		return frame, true
	}
	headerSize, err := rawVKeyHeaderSize(vkey)
	if err != nil || len(frame) < headerSize {
		return nil, false
	}
	if string(frame[:4]) != rawVKeyMagic {
		return nil, false
	}
	if binary.BigEndian.Uint16(frame[4:6]) != uint16(len(vkey)) || frame[6] != 0 || frame[7] != 0 {
		return nil, false
	}
	if !bytes.Equal(frame[rawVKeyHeaderBaseSize:headerSize], vkey) {
		return nil, false
	}
	return frame[headerSize:], true
}
