//go:build linux

package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/pion/dtls/v3"

	"tapx/internal/linkdiag"
)

const (
	udpDiagHeaderSize      = 32
	udpDiagVersion         = 1
	udpDiagPingRequest     = 1
	udpDiagPingResponse    = 2
	udpDiagUploadData      = 3
	udpDiagUploadFinish    = 4
	udpDiagUploadAck       = 5
	udpDiagDownloadRequest = 6
	udpDiagDownloadData    = 7
	udpDiagDownloadFinish  = 8
)

func (h *UDPPipeHandle) Diagnose(ctx context.Context, kind string, duration time.Duration) (ConnectorDiagnostic, error) {
	if h.Pipe.DTLS.Enabled {
		return h.diagnoseDTLS(ctx, kind, duration)
	}
	peer := h.RemoteAddr
	if !peer.IsValid() {
		return ConnectorDiagnostic{}, errors.New("core: UDP connector has no active remote peer")
	}
	pipe := h.Pipe
	pipe.BindPort = 0
	pipe.ReuseAddr = false
	pipe.ReusePort = false
	packetConn, _, err := openUDPPacketConn(pipe, peer)
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	defer packetConn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = packetConn.SetDeadline(deadline)
	}
	result := ConnectorDiagnostic{Kind: kind, Transport: "udp", Target: peer.String()}
	client := udpDiagnosticClient{
		conn:       packetConn,
		peer:       net.UDPAddrFromAddrPort(peer),
		credential: h.Pipe.Binding.VKeyValue,
		wireSize:   h.diagnosticWireSize(),
	}
	switch kind {
	case "channel":
		result.Delay, err = client.ping(ctx)
	case "throughput":
		result, err = client.throughput(ctx, duration)
		result.Kind = kind
		result.Transport = "udp"
		result.Target = peer.String()
	case "path-mtu":
		result.TCPMSS = h.Pipe.TCPMSSIPv4
	default:
		err = fmt.Errorf("core: unsupported UDP diagnostic %q", kind)
	}
	return result, err
}

func (h *UDPPipeHandle) diagnoseDTLS(ctx context.Context, kind string, duration time.Duration) (ConnectorDiagnostic, error) {
	peer := h.RemoteAddr
	if !peer.IsValid() {
		return ConnectorDiagnostic{}, errors.New("core: DTLS connector has no active remote peer")
	}
	pipe := h.Pipe
	pipe.BindPort = 0
	pipe.ReuseAddr = false
	pipe.ReusePort = false
	packetConn, _, err := openUDPPacketConn(pipe, peer)
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	defer packetConn.Close()
	options, err := rawUDPClientDTLSOptions(h.Pipe.DTLS, peer.Addr().String())
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	conn, err := dtls.ClientWithOptions(packetConn, udpAddrFromAddrPort(peer), options...)
	if err != nil {
		return ConnectorDiagnostic{}, fmt.Errorf("core: open DTLS diagnostic %s: %w", h.Pipe.EndpointID, err)
	}
	defer conn.Close()
	handshakeCtx, cancel := h.dtlsHandshakeContext(ctx)
	err = conn.HandshakeContext(handshakeCtx)
	cancel()
	if err != nil {
		return ConnectorDiagnostic{}, fmt.Errorf("core: DTLS diagnostic handshake %s: %w", h.Pipe.EndpointID, err)
	}
	credential := h.Pipe.Binding.VKeyValue
	if err := sendDTLSDiagnosticHello(ctx, conn, credential, h.dtlsHandshakeTimeout()); err != nil {
		return ConnectorDiagnostic{}, fmt.Errorf("core: classify DTLS diagnostic %s: %w", h.Pipe.EndpointID, err)
	}
	result := ConnectorDiagnostic{Kind: kind, Transport: "dtls", Target: peer.String()}
	switch kind {
	case "channel":
		result.Delay, err = linkdiag.Ping(ctx, conn, credential)
	case "throughput":
		recordOverhead, overheadErr := dtlsRecordOverhead(conn)
		if overheadErr != nil {
			return ConnectorDiagnostic{}, overheadErr
		}
		dtlsMTU := h.Pipe.DTLS.MTU
		if dtlsMTU <= 0 {
			dtlsMTU = 1200
		}
		chunkSize := dtlsMTU - recordOverhead
		if chunkSize < 64 {
			return ConnectorDiagnostic{}, fmt.Errorf("core: DTLS diagnostic MTU %d leaves no usable payload", dtlsMTU)
		}
		measured, measureErr := linkdiag.ThroughputWithChunkSize(ctx, conn, credential, duration, chunkSize)
		err = measureErr
		result.Delay = measured.Delay
		result.UploadBytes = measured.UploadBytes
		result.DownloadBytes = measured.DownloadBytes
		result.UploadBPS = measured.UploadBPS
		result.DownloadBPS = measured.DownloadBPS
		result.Duration = measured.Duration
	case "path-mtu":
		result.Delay, err = linkdiag.Ping(ctx, conn, credential)
		if err == nil {
			result.PathMTU = firstPositive(h.Pipe.ConfirmedPathMTU, h.Pipe.EffectiveNetworkMTU)
			result.TCPMSS = firstPositive(h.Pipe.TCPMSSIPv4, h.Pipe.TCPMSSIPv6)
			if result.PathMTU <= 0 {
				err = errors.New("core: DTLS path MTU has not been confirmed")
			}
		}
	default:
		err = fmt.Errorf("core: unsupported DTLS diagnostic %q", kind)
	}
	return result, err
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (h *UDPPipeHandle) diagnosticWireSize() int {
	if h.Pipe.MaxDatagramPayload > 0 {
		return h.Pipe.MaxDatagramPayload
	}
	return 1400
}

type udpDiagnosticClient struct {
	conn       net.PacketConn
	peer       *net.UDPAddr
	credential string
	wireSize   int
}

func (c *udpDiagnosticClient) ping(ctx context.Context) (time.Duration, error) {
	session, err := randomDiagnosticSession()
	if err != nil {
		return 0, err
	}
	request, err := c.packet(udpDiagPingRequest, session, 0, 0, 0, 0)
	if err != nil {
		return 0, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		started := time.Now()
		if _, err := c.conn.WriteTo(request, c.peer); err != nil {
			return 0, err
		}
		response, err := c.read(ctx, 800*time.Millisecond)
		if err == nil && response.op == udpDiagPingResponse && response.session == session {
			return time.Since(started), nil
		}
	}
	return 0, errors.New("core: UDP TapX diagnostic acknowledgement timed out")
}

func (c *udpDiagnosticClient) throughput(ctx context.Context, duration time.Duration) (ConnectorDiagnostic, error) {
	if duration <= 0 {
		duration = 2 * time.Second
	}
	if duration > 10*time.Second {
		duration = 10 * time.Second
	}
	session, err := randomDiagnosticSession()
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	bodySize := c.wireSize - c.vkeyHeaderSize() - udpDiagHeaderSize
	if bodySize < 64 {
		return ConnectorDiagnostic{}, fmt.Errorf("core: UDP diagnostic wire size %d is too small", c.wireSize)
	}
	uploadPacket, err := c.packet(udpDiagUploadData, session, 0, uint32(duration/time.Millisecond), 0, bodySize)
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	uploadStarted := time.Now()
	var sent uint64
	var sequence uint32
	for time.Since(uploadStarted) < duration {
		binary.BigEndian.PutUint32(uploadPacket[c.vkeyHeaderSize()+16:], sequence)
		written, writeErr := c.conn.WriteTo(uploadPacket, c.peer)
		if writeErr != nil {
			if netErr, ok := writeErr.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if errors.Is(writeErr, syscall.EINTR) || errors.Is(writeErr, syscall.EAGAIN) ||
				errors.Is(writeErr, syscall.EWOULDBLOCK) || errors.Is(writeErr, syscall.ENOBUFS) {
				continue
			}
			return ConnectorDiagnostic{}, writeErr
		}
		if written == len(uploadPacket) {
			sent += uint64(bodySize)
		}
		sequence++
	}
	finish, err := c.packet(udpDiagUploadFinish, session, sequence, uint32(duration/time.Millisecond), sent, 0)
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	var acknowledged uint64
	for attempt := 0; attempt < 3 && acknowledged == 0; attempt++ {
		if _, err := c.conn.WriteTo(finish, c.peer); err != nil {
			return ConnectorDiagnostic{}, err
		}
		response, readErr := c.read(ctx, time.Second)
		if readErr == nil && response.op == udpDiagUploadAck && response.session == session {
			acknowledged = response.value
		}
	}
	if acknowledged == 0 && sent > 0 {
		return ConnectorDiagnostic{}, errors.New("core: UDP upload diagnostic acknowledgement timed out")
	}

	downloadSession, err := randomDiagnosticSession()
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	downloadRequest, err := c.packet(udpDiagDownloadRequest, downloadSession, 0, uint32(duration/time.Millisecond), uint64(bodySize), 0)
	if err != nil {
		return ConnectorDiagnostic{}, err
	}
	if _, err := c.conn.WriteTo(downloadRequest, c.peer); err != nil {
		return ConnectorDiagnostic{}, err
	}
	downloadStarted := time.Now()
	var downloaded uint64
	for {
		response, readErr := c.read(ctx, duration+3*time.Second)
		if readErr != nil {
			return ConnectorDiagnostic{}, readErr
		}
		if response.session != downloadSession {
			continue
		}
		switch response.op {
		case udpDiagDownloadData:
			downloaded += uint64(response.bodySize)
		case udpDiagDownloadFinish:
			elapsed := time.Since(downloadStarted)
			return ConnectorDiagnostic{
				UploadBytes:   acknowledged,
				DownloadBytes: downloaded,
				UploadBPS:     diagnosticBitsPerSecond(acknowledged, duration),
				DownloadBPS:   diagnosticBitsPerSecond(downloaded, elapsed),
				Duration:      duration,
			}, nil
		}
	}
}

type udpDiagnosticMessage struct {
	op       uint8
	session  uint64
	value    uint64
	bodySize int
}

func (c *udpDiagnosticClient) read(ctx context.Context, timeout time.Duration) (udpDiagnosticMessage, error) {
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := c.conn.SetReadDeadline(deadline); err != nil {
		return udpDiagnosticMessage{}, err
	}
	buffer := make([]byte, max(c.wireSize, 2048))
	for {
		n, peer, err := c.conn.ReadFrom(buffer)
		if err != nil {
			return udpDiagnosticMessage{}, err
		}
		if !sameDiagnosticPeer(peer, c.peer) {
			continue
		}
		payload, ok := stripDiagnosticVKey(buffer[:n], c.credential)
		if !ok || len(payload) < udpDiagHeaderSize || string(payload[:4]) != "TXD1" || payload[4] != udpDiagVersion || int(binary.BigEndian.Uint16(payload[6:8])) != udpDiagHeaderSize {
			continue
		}
		return udpDiagnosticMessage{
			op: payload[5], session: binary.BigEndian.Uint64(payload[8:16]),
			value: binary.BigEndian.Uint64(payload[24:32]), bodySize: len(payload) - udpDiagHeaderSize,
		}, nil
	}
}

func (c *udpDiagnosticClient) packet(op uint8, session uint64, sequence, durationMS uint32, value uint64, bodySize int) ([]byte, error) {
	headerSize := c.vkeyHeaderSize()
	packet := make([]byte, headerSize+udpDiagHeaderSize+bodySize)
	if headerSize > 0 {
		writeRawVKeyHeader(packet[:headerSize], []byte(c.credential))
	}
	header := packet[headerSize:]
	copy(header[:4], "TXD1")
	header[4] = udpDiagVersion
	header[5] = op
	binary.BigEndian.PutUint16(header[6:8], udpDiagHeaderSize)
	binary.BigEndian.PutUint64(header[8:16], session)
	binary.BigEndian.PutUint32(header[16:20], sequence)
	binary.BigEndian.PutUint32(header[20:24], durationMS)
	binary.BigEndian.PutUint64(header[24:32], value)
	for index := udpDiagHeaderSize; index < len(header); index++ {
		header[index] = 0xa5
	}
	return packet, nil
}

func (c *udpDiagnosticClient) vkeyHeaderSize() int {
	size, _ := rawVKeyHeaderSize([]byte(c.credential))
	return size
}

func stripDiagnosticVKey(packet []byte, credential string) ([]byte, bool) {
	if credential == "" {
		return packet, true
	}
	return stripRawVKeyHeader(packet, []byte(credential))
}

func randomDiagnosticSession() (uint64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(raw[:]), nil
}

func sameDiagnosticPeer(addr net.Addr, want *net.UDPAddr) bool {
	udp, ok := addr.(*net.UDPAddr)
	if !ok {
		return false
	}
	got, ok := netip.AddrFromSlice(udp.IP)
	return ok && got.Unmap() == want.AddrPort().Addr().Unmap() && udp.Port == want.Port
}

func diagnosticBitsPerSecond(bytes uint64, elapsed time.Duration) uint64 {
	if elapsed <= 0 {
		return 0
	}
	return uint64(float64(bytes*8) / elapsed.Seconds())
}
