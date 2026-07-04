//go:build linux

package core

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
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
	go h.acceptOneTLS(ctx, listener, done, frameKind, guard, tlsConfig)
	return nil
}

func (h *TCPPipeHandle) acceptOneTLS(ctx context.Context, listener *net.TCPListener, done chan struct{}, frameKind fastpath.FrameKind, guard fastpath.AddressGuard, tlsConfig *tls.Config) {
	defer close(done)
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
	if err := h.startTLSBridge(ctx, tls.Server(conn, tlsConfig), frameKind, guard); err != nil {
		h.setErr(err)
	}
}

func (h *TCPPipeHandle) startTLSConnector(frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	tlsConfig, err := rawTCPClientTLSConfig(h.Pipe.TLS, h.Pipe.Remote)
	if err != nil {
		return err
	}
	tcpConn, local, remote, err := dialTCP(h.Pipe)
	if err != nil {
		return err
	}
	if err := configureTCPConn(tcpConn, h.Pipe); err != nil {
		_ = tcpConn.Close()
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.tlsCancel = cancel
	tlsConn := tls.Client(tcpConn, tlsConfig)
	handshakeCtx, stopHandshake := h.tlsHandshakeContext(ctx)
	if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
		stopHandshake()
		cancel()
		_ = tlsConn.Close()
		return fmt.Errorf("core: tls handshake %s: %w", h.Pipe.EndpointID, err)
	}
	stopHandshake()
	h.LocalAddr = local
	h.RemoteAddr = remote
	if err := h.startTLSBridge(ctx, tlsConn, frameKind, guard); err != nil {
		cancel()
		return err
	}
	return nil
}

func (h *TCPPipeHandle) startTLSBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) error {
	if _, err := rawVKeyHeaderSize([]byte(h.Pipe.Binding.VKeyValue)); err != nil {
		_ = conn.Close()
		return err
	}
	h.mu.Lock()
	if h.tlsDone != nil {
		h.mu.Unlock()
		_ = conn.Close()
		return fmt.Errorf("core: tls tcp pipe %s already has an active stream", h.Pipe.EndpointID)
	}
	done := make(chan struct{})
	h.tlsDone = done
	h.tlsConn = conn
	h.mu.Unlock()

	go func() {
		defer close(done)
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
	return nil
}

func (h *TCPPipeHandle) runTLSBridge(ctx context.Context, conn net.Conn, frameKind fastpath.FrameKind, guard fastpath.AddressGuard) {
	defer conn.Close()
	if tlsConn, ok := conn.(*tls.Conn); ok {
		handshakeCtx, stopHandshake := h.tlsHandshakeContext(ctx)
		err := tlsConn.HandshakeContext(handshakeCtx)
		stopHandshake()
		if err != nil {
			h.setTLSBridgeErr(ctx, fmt.Errorf("core: tls handshake %s: %w", h.Pipe.EndpointID, err))
			return
		}
	}
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
			h.tlsCounter.dropsGuard.Add(1)
			continue
		}
		payload := frame
		if vkeyHeaderSize > 0 {
			payload = wire[:vkeyHeaderSize+n]
			writeRawVKeyHeader(payload[:vkeyHeaderSize], vkey)
			copy(payload[vkeyHeaderSize:], frame)
		}
		if err := rawtcp.WriteFrame(conn, h.Pipe.LengthMode, payload); err != nil {
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
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		wireFrame, err := rawtcp.ReadFrame(conn, h.Pipe.LengthMode, readMax)
		if err != nil {
			return err
		}
		frame, ok := stripRawVKeyHeader(wireFrame, vkey)
		if !ok {
			h.tlsCounter.dropsGuard.Add(1)
			continue
		}
		if !xrayFrameAllowed(frameKind, frame, guard) {
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
		NextProtos:   cleanALPN(settings.ALPN),
	}
	if settings.ServerName != "" {
		cfg.ServerName = settings.ServerName
	}
	if err := applyTLSVersions(cfg, settings.MinVersion, settings.MaxVersion); err != nil {
		return nil, err
	}
	if settings.CAFile != "" {
		pool, err := loadCertPool(settings.CAFile)
		if err != nil {
			return nil, err
		}
		cfg.ClientCAs = pool
		if settings.AllowInsecure {
			cfg.ClientAuth = tls.RequestClientCert
		} else {
			cfg.ClientAuth = tls.RequireAndVerifyClientCert
		}
	}
	return cfg, nil
}

func rawTCPClientTLSConfig(settings model.RawTLSSettings, remote string) (*tls.Config, error) {
	cfg := &tls.Config{
		ServerName:         firstNonEmpty(settings.ServerName, remote),
		InsecureSkipVerify: settings.AllowInsecure,
		NextProtos:         cleanALPN(settings.ALPN),
	}
	if err := applyTLSVersions(cfg, settings.MinVersion, settings.MaxVersion); err != nil {
		return nil, err
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
			return nil, fmt.Errorf("core: load raw tcp tls client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
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

func loadCertPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("core: read CA file %q: %w", path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("core: CA file %q has no PEM certificates", path)
	}
	return pool, nil
}

func cleanALPN(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
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
