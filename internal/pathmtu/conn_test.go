package pathmtu

import (
	"context"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestConnExchangerCommitsConfirmedPath(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	confirmed := make(chan CommittedProbe, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ServeConnProbeResponses(ctx, server, ConnResponderOptions{
			MaxPayload: 1320, CommitGrace: 10 * time.Millisecond,
			OnConfirmed: func(value CommittedProbe) { confirmed <- value },
		})
	}()

	exchanger := &ConnExchanger{Conn: client, Timeout: 10 * time.Millisecond}
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1500, CandidatePayload: 1320, MinimumPayload: 600, Attempts: 1,
	}, exchanger.Exchange)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != 1320 {
		t.Fatalf("confirmed payload = %d, want 1320", result.PayloadSize)
	}
	if err := exchanger.Commit(context.Background(), result.PayloadSize, 3); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-confirmed:
		if value.PayloadSize != result.PayloadSize {
			t.Fatalf("committed payload = %d, want %d", value.PayloadSize, result.PayloadSize)
		}
	case <-time.After(time.Second):
		t.Fatal("responder did not publish committed path")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("responder did not finish commit grace period")
	}
}

func TestConnResponderContinuesAfterMessageTooLong(t *testing.T) {
	client, rawServer := net.Pipe()
	defer client.Close()
	defer rawServer.Close()
	server := &writeLimitedConn{Conn: rawServer, maxWrite: 1320}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ServeConnProbeResponses(ctx, server, ConnResponderOptions{
			MaxPayload: 1500, CommitGrace: 10 * time.Millisecond,
		})
	}()
	exchanger := &ConnExchanger{Conn: client, Timeout: 10 * time.Millisecond}
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1500, CandidatePayload: 1320, MinimumPayload: 600, Attempts: 1,
	}, exchanger.Exchange)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != 1320 {
		t.Fatalf("confirmed payload = %d, want 1320", result.PayloadSize)
	}
	if err := exchanger.Commit(context.Background(), result.PayloadSize, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("responder did not finish")
	}
}

type writeLimitedConn struct {
	net.Conn
	maxWrite int
}

func (c *writeLimitedConn) Write(payload []byte) (int, error) {
	if len(payload) > c.maxWrite {
		return 0, syscall.EMSGSIZE
	}
	return c.Conn.Write(payload)
}
