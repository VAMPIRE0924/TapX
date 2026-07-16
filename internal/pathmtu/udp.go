package pathmtu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"
)

const MaxUDPProbePayload = 65507

type UDPExchanger struct {
	Conn       *net.UDPConn
	Peer       *net.UDPAddr
	Timeout    time.Duration
	MaxPayload int
	Prefix     []byte

	mu          sync.Mutex
	successful  map[int][]byte
	pending     [pendingProbeSlots]pendingProbe
	nextPending int
}

// Exchange sends one probe and waits for its exact response. The mutex keeps
// multiple control-plane confirmations from consuming each other's replies.
func (e *UDPExchanger) Exchange(ctx context.Context, request []byte) ([]byte, error) {
	if e == nil || e.Conn == nil {
		return nil, errors.New("UDP probe connection is required")
	}
	if e.Peer == nil || e.Peer.IP == nil || e.Peer.Port == 0 {
		return nil, errors.New("UDP probe peer is required")
	}
	if len(request) > MaxUDPProbePayload {
		return nil, fmt.Errorf("UDP probe payload %d exceeds %d bytes", len(request), MaxUDPProbePayload)
	}
	expected, err := ProbeResponseFor(request)
	if err != nil {
		return nil, fmt.Errorf("validate UDP probe request: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	response, err := e.exchangePacketLocked(ctx, request, expected)
	if err != nil {
		return nil, err
	}
	if e.successful == nil {
		e.successful = make(map[int][]byte)
	}
	e.successful[len(request)] = append([]byte(nil), request...)
	return response, nil
}

// Commit sends the exact successful request selected by ConfirmPayload and
// waits for the responder's final acknowledgement. Both peers therefore agree
// on one immutable payload limit before either activates a worker.
func (e *UDPExchanger) Commit(ctx context.Context, payloadSize, attempts int) error {
	if e == nil || e.Conn == nil {
		return errors.New("UDP probe connection is required")
	}
	if e.Peer == nil || e.Peer.IP == nil || e.Peer.Port == 0 {
		return errors.New("UDP probe peer is required")
	}
	if attempts <= 0 {
		attempts = 3
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	request := e.successful[payloadSize]
	if len(request) == 0 {
		return fmt.Errorf("no successful UDP probe is available for %d bytes", payloadSize)
	}
	commit, err := ProbeCommitFor(request)
	if err != nil {
		return err
	}
	expected, err := ProbeCommittedFor(commit)
	if err != nil {
		return err
	}
	var lastErr error
	for range attempts {
		if _, err := e.exchangePacketLocked(ctx, commit, expected); err != nil {
			lastErr = err
			continue
		}
		clear(e.successful)
		return nil
	}
	return fmt.Errorf("commit peer-confirmed UDP payload %d: %w", payloadSize, lastErr)
}

func (e *UDPExchanger) exchangePacketLocked(ctx context.Context, outgoing, expected []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	contextDeadline, hasContextDeadline := ctx.Deadline()
	if hasContextDeadline && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := e.Conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set UDP probe deadline: %w", err)
	}
	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		_ = e.Conn.SetDeadline(time.Now())
	})
	defer func() {
		if !stopCancel() {
			<-cancelDone
		}
		_ = e.Conn.SetDeadline(time.Time{})
	}()

	wireOutgoing := prefixUDPPacket(e.Prefix, outgoing)
	wireExpected := prefixUDPPacket(e.Prefix, expected)
	if len(wireOutgoing) > MaxUDPProbePayload {
		return nil, fmt.Errorf("prefixed UDP probe payload %d exceeds %d bytes", len(wireOutgoing), MaxUDPProbePayload)
	}
	if _, err := e.Conn.WriteToUDP(wireOutgoing, e.Peer); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if hasContextDeadline && !time.Now().Before(contextDeadline) {
			return nil, context.DeadlineExceeded
		}
		return nil, fmt.Errorf("send UDP probe: %w", err)
	}

	buffer := make([]byte, MaxUDPProbePayload)
	for {
		n, peer, err := e.Conn.ReadFromUDP(buffer)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if hasContextDeadline && !time.Now().Before(contextDeadline) {
					return nil, context.DeadlineExceeded
				}
				return nil, fmt.Errorf("UDP probe timed out after %s", timeout)
			}
			return nil, fmt.Errorf("receive UDP probe response: %w", err)
		}
		if !sameUDPAddr(peer, e.Peer) {
			continue
		}
		response := buffer[:n]
		if n == len(wireExpected) && bytes.Equal(response, wireExpected) {
			return append([]byte(nil), expected...), nil
		}
		if err := e.respondToPeerProbeLocked(response, peer); err != nil {
			return nil, err
		}
	}
}

// Handoff keeps answering a simultaneously probing peer after this side has
// committed its own path. This covers fixed-peer listener/listener and
// connector/connector layouts without assigning either endpoint a hidden
// initiator role.
func (e *UDPExchanger) Handoff(ctx context.Context, duration time.Duration) error {
	if e == nil || e.Conn == nil {
		return errors.New("UDP probe connection is required")
	}
	if e.Peer == nil || e.Peer.IP == nil || e.Peer.Port == 0 {
		return errors.New("UDP probe peer is required")
	}
	if duration <= 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	deadline := time.Now().Add(duration)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := e.Conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("set UDP probe handoff deadline: %w", err)
	}
	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		_ = e.Conn.SetReadDeadline(time.Now())
	})
	defer func() {
		if !stopCancel() {
			<-cancelDone
		}
		_ = e.Conn.SetReadDeadline(time.Time{})
	}()

	buffer := make([]byte, MaxUDPProbePayload)
	for {
		control, err := peekUDPControlDatagram(e.Conn, buffer, e.Prefix)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return contextErr
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil
			}
			return fmt.Errorf("peek UDP packet during handoff: %w", err)
		}
		if !control {
			// Leave the first data-plane datagram queued for the fastpath worker.
			return nil
		}
		n, peer, err := e.Conn.ReadFromUDP(buffer)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return contextErr
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil
			}
			return fmt.Errorf("receive UDP probe during handoff: %w", err)
		}
		if !sameUDPAddr(peer, e.Peer) {
			continue
		}
		if err := e.respondToPeerProbeLocked(buffer[:n], peer); err != nil {
			return err
		}
	}
}

func (e *UDPExchanger) respondToPeerProbeLocked(packet []byte, peer *net.UDPAddr) error {
	wirePacket := packet
	packet, ok := stripUDPPacketPrefix(packet, e.Prefix)
	if !ok {
		return nil
	}
	maxPayload := e.MaxPayload
	if maxPayload <= 0 {
		maxPayload = MaxUDPProbePayload
	}
	if len(wirePacket) > maxPayload {
		return nil
	}
	message, err := ParseProbe(packet)
	if err != nil {
		return nil
	}
	peerAddr := peer.AddrPort()
	switch message.Type {
	case ProbeRequest:
		response, err := ProbeResponseFor(packet)
		if err != nil {
			return nil
		}
		if _, err := e.Conn.WriteToUDP(prefixUDPPacket(e.Prefix, response), peer); err != nil {
			if errors.Is(err, syscall.EMSGSIZE) {
				return nil
			}
			return fmt.Errorf("send simultaneous UDP probe response: %w", err)
		}
		e.pending[e.nextPending] = pendingProbe{
			valid: true, peer: peerAddr, token: message.Token, size: message.Size,
			bodyHash: crc32.ChecksumIEEE(message.Body),
		}
		e.nextPending = (e.nextPending + 1) % len(e.pending)
	case ProbeCommit:
		bodyHash := crc32.ChecksumIEEE(message.Body)
		for index := range e.pending {
			item := &e.pending[index]
			if !item.valid || item.peer != peerAddr || item.token != message.Token ||
				item.size != message.Size || item.bodyHash != bodyHash {
				continue
			}
			response, err := ProbeCommittedFor(packet)
			if err != nil {
				return nil
			}
			if _, err := e.Conn.WriteToUDP(prefixUDPPacket(e.Prefix, response), peer); err != nil {
				return fmt.Errorf("send simultaneous UDP commit acknowledgement: %w", err)
			}
			item.committed = true
			return nil
		}
	}
	return nil
}

type UDPResponderOptions struct {
	MaxPayload  int
	Prefix      []byte
	CommitGrace time.Duration
	OnConfirmed func(UDPConfirmedPath)
}

type UDPConfirmedPath struct {
	Peer        netip.AddrPort
	PayloadSize int
	Token       uint64
}

const pendingProbeSlots = 64

type pendingProbe struct {
	valid     bool
	peer      netip.AddrPort
	token     uint64
	size      int
	bodyHash  uint32
	committed bool
}

// ServeUDPProbeResponses provides the peer-confirmation half of path probing.
// It ignores malformed, oversized, and response packets and never amplifies a
// datagram: every valid reply is exactly the same size as its request.
func ServeUDPProbeResponses(ctx context.Context, conn *net.UDPConn, options UDPResponderOptions) error {
	if conn == nil {
		return errors.New("UDP probe responder connection is required")
	}
	maxPayload := options.MaxPayload
	if maxPayload <= 0 {
		maxPayload = MaxUDPProbePayload
	}
	if maxPayload < ProbeHeaderSize || maxPayload > MaxUDPProbePayload {
		return fmt.Errorf("UDP probe responder payload limit must be between %d and %d", ProbeHeaderSize, MaxUDPProbePayload)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		_ = conn.SetReadDeadline(time.Now())
	})
	defer func() {
		if !stopCancel() {
			<-cancelDone
		}
		_ = conn.SetReadDeadline(time.Time{})
	}()

	// Always receive the complete UDP datagram before applying the simulated or
	// configured path limit. A shorter socket buffer produces MSG_TRUNC on Unix
	// and WSAEMSGSIZE on Windows, which would incorrectly stop the responder.
	buffer := make([]byte, MaxUDPProbePayload)
	var pending [pendingProbeSlots]pendingProbe
	nextPending := 0
	var commitDeadline time.Time
	for {
		n, peer, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && !commitDeadline.IsZero() &&
				!time.Now().Before(commitDeadline) {
				return nil
			}
			return fmt.Errorf("receive UDP probe request: %w", err)
		}
		if n > maxPayload {
			continue
		}
		wirePacket := buffer[:n]
		packet, ok := stripUDPPacketPrefix(wirePacket, options.Prefix)
		if !ok {
			continue
		}
		message, err := ParseProbe(packet)
		if err != nil {
			continue
		}
		if !commitDeadline.IsZero() && message.Type != ProbeCommit {
			continue
		}
		peerAddr := peer.AddrPort()
		var response []byte
		var matchedPending *pendingProbe
		switch message.Type {
		case ProbeRequest:
			response, err = ProbeResponseFor(packet)
		case ProbeCommit:
			bodyHash := crc32.ChecksumIEEE(message.Body)
			for index := range pending {
				item := &pending[index]
				if item.valid && item.peer == peerAddr && item.token == message.Token &&
					item.size == message.Size && item.bodyHash == bodyHash {
					matchedPending = item
					break
				}
			}
			if matchedPending == nil {
				continue
			}
			response, err = ProbeCommittedFor(packet)
		default:
			continue
		}
		if err != nil {
			continue
		}
		if _, err := conn.WriteToUDP(prefixUDPPacket(options.Prefix, response), peer); err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil
			}
			if message.Type == ProbeRequest && errors.Is(err, syscall.EMSGSIZE) {
				continue
			}
			return fmt.Errorf("send UDP probe response: %w", err)
		}
		if message.Type == ProbeRequest {
			pending[nextPending] = pendingProbe{
				valid: true, peer: peerAddr, token: message.Token, size: message.Size,
				bodyHash: crc32.ChecksumIEEE(message.Body),
			}
			nextPending = (nextPending + 1) % len(pending)
		} else {
			item := matchedPending
			if !item.committed && options.OnConfirmed != nil {
				options.OnConfirmed(UDPConfirmedPath{
					Peer: peerAddr, PayloadSize: message.Size, Token: message.Token,
				})
			}
			if !item.committed && options.CommitGrace > 0 {
				commitDeadline = time.Now().Add(options.CommitGrace)
				if err := conn.SetReadDeadline(commitDeadline); err != nil {
					return fmt.Errorf("set UDP probe commit grace deadline: %w", err)
				}
			}
			item.committed = true
		}
	}
}

func sameUDPAddr(left, right *net.UDPAddr) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Port == right.Port && left.Zone == right.Zone && left.IP.Equal(right.IP)
}

func prefixUDPPacket(prefix, payload []byte) []byte {
	if len(prefix) == 0 {
		return payload
	}
	packet := make([]byte, len(prefix)+len(payload))
	copy(packet, prefix)
	copy(packet[len(prefix):], payload)
	return packet
}

func stripUDPPacketPrefix(packet, prefix []byte) ([]byte, bool) {
	if len(prefix) == 0 {
		return packet, true
	}
	if len(packet) < len(prefix) || !bytes.Equal(packet[:len(prefix)], prefix) {
		return nil, false
	}
	return packet[len(prefix):], true
}
