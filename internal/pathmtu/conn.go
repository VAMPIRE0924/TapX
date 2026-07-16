package pathmtu

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"net"
	"sync"
	"syscall"
	"time"
)

// ConnExchanger runs the path confirmation protocol over a datagram-preserving
// net.Conn such as a completed DTLS association. It is startup control traffic,
// never part of the frame forwarding path.
type ConnExchanger struct {
	Conn    net.Conn
	Timeout time.Duration

	mu         sync.Mutex
	successful map[int][]byte
}

func (e *ConnExchanger) Exchange(ctx context.Context, request []byte) ([]byte, error) {
	if e == nil || e.Conn == nil {
		return nil, errors.New("probe connection is required")
	}
	if len(request) > MaxUDPProbePayload {
		return nil, fmt.Errorf("probe payload %d exceeds %d bytes", len(request), MaxUDPProbePayload)
	}
	expected, err := ProbeResponseFor(request)
	if err != nil {
		return nil, fmt.Errorf("validate probe request: %w", err)
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

func (e *ConnExchanger) Commit(ctx context.Context, payloadSize, attempts int) error {
	if e == nil || e.Conn == nil {
		return errors.New("probe connection is required")
	}
	if attempts <= 0 {
		attempts = 3
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	request := e.successful[payloadSize]
	if len(request) == 0 {
		return fmt.Errorf("no successful probe is available for %d bytes", payloadSize)
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
	return fmt.Errorf("commit peer-confirmed payload %d: %w", payloadSize, lastErr)
}

func (e *ConnExchanger) exchangePacketLocked(ctx context.Context, outgoing, expected []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := e.Conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set probe deadline: %w", err)
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

	written, err := e.Conn.Write(outgoing)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("send probe: %w", err)
	}
	if written != len(outgoing) {
		return nil, fmt.Errorf("send probe: wrote %d of %d bytes", written, len(outgoing))
	}
	buffer := make([]byte, MaxUDPProbePayload)
	for {
		n, err := e.Conn.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return nil, fmt.Errorf("probe timed out after %s", timeout)
			}
			return nil, fmt.Errorf("receive probe response: %w", err)
		}
		if n == len(expected) && bytes.Equal(buffer[:n], expected) {
			return append([]byte(nil), buffer[:n]...), nil
		}
	}
}

type ConnResponderOptions struct {
	MaxPayload  int
	CommitGrace time.Duration
	OnConfirmed func(CommittedProbe)
}

type CommittedProbe struct {
	PayloadSize int
	Token       uint64
}

func ServeConnProbeResponses(ctx context.Context, conn net.Conn, options ConnResponderOptions) error {
	if conn == nil {
		return errors.New("probe responder connection is required")
	}
	maxPayload := options.MaxPayload
	if maxPayload <= 0 {
		maxPayload = MaxUDPProbePayload
	}
	if maxPayload < ProbeHeaderSize || maxPayload > MaxUDPProbePayload {
		return fmt.Errorf("probe responder payload limit must be between %d and %d", ProbeHeaderSize, MaxUDPProbePayload)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		_ = conn.SetDeadline(time.Now())
	})
	defer func() {
		if !stopCancel() {
			<-cancelDone
		}
		_ = conn.SetDeadline(time.Time{})
	}()

	buffer := make([]byte, MaxUDPProbePayload)
	var pending [pendingProbeSlots]pendingProbe
	nextPending := 0
	var commitDeadline time.Time
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && !commitDeadline.IsZero() &&
				!time.Now().Before(commitDeadline) {
				return nil
			}
			return fmt.Errorf("receive probe request: %w", err)
		}
		if n > maxPayload {
			continue
		}
		packet := buffer[:n]
		message, err := ParseProbe(packet)
		if err != nil || (!commitDeadline.IsZero() && message.Type != ProbeCommit) {
			continue
		}
		var response []byte
		var matchedPending *pendingProbe
		switch message.Type {
		case ProbeRequest:
			response, err = ProbeResponseFor(packet)
		case ProbeCommit:
			bodyHash := crc32.ChecksumIEEE(message.Body)
			for index := range pending {
				item := &pending[index]
				if item.valid && item.token == message.Token && item.size == message.Size && item.bodyHash == bodyHash {
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
		written, err := conn.Write(response)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if message.Type == ProbeRequest && errors.Is(err, syscall.EMSGSIZE) {
				continue
			}
			return fmt.Errorf("send probe response: %w", err)
		}
		if written != len(response) {
			return fmt.Errorf("send probe response: wrote %d of %d bytes", written, len(response))
		}
		if message.Type == ProbeRequest {
			pending[nextPending] = pendingProbe{
				valid: true, token: message.Token, size: message.Size, bodyHash: crc32.ChecksumIEEE(message.Body),
			}
			nextPending = (nextPending + 1) % len(pending)
			continue
		}
		item := matchedPending
		if !item.committed && options.OnConfirmed != nil {
			options.OnConfirmed(CommittedProbe{PayloadSize: message.Size, Token: message.Token})
		}
		if !item.committed && options.CommitGrace > 0 {
			commitDeadline = time.Now().Add(options.CommitGrace)
			if err := conn.SetDeadline(commitDeadline); err != nil {
				return fmt.Errorf("set probe commit grace deadline: %w", err)
			}
		}
		item.committed = true
	}
}
