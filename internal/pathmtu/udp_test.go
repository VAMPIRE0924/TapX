package pathmtu

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestUDPExchangerRoundTrip(t *testing.T) {
	server, stopServer := startUDPProbeServer(t, MaxUDPProbePayload)
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()

	exchanger := &UDPExchanger{
		Conn:    client,
		Peer:    server.LocalAddr().(*net.UDPAddr),
		Timeout: time.Second,
	}
	request, err := NewProbeRequest(1400)
	if err != nil {
		t.Fatal(err)
	}
	response, err := exchanger.Exchange(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ProbeResponseFor(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != string(want) {
		t.Fatal("UDP probe response does not match request")
	}
}

func TestUDPExchangerRoundTripWithPrefix(t *testing.T) {
	prefix := []byte("TXV1-prefix")
	server, stopServer := startUDPProbeServerWithOptions(t, UDPResponderOptions{
		MaxPayload: MaxUDPProbePayload, Prefix: prefix,
	})
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()

	exchanger := &UDPExchanger{
		Conn: client, Peer: server.LocalAddr().(*net.UDPAddr), Timeout: time.Second, Prefix: prefix,
	}
	request, err := NewProbeRequest(600)
	if err != nil {
		t.Fatal(err)
	}
	response, err := exchanger.Exchange(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ProbeResponseFor(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(response) != string(want) {
		t.Fatal("prefixed UDP probe response does not match request")
	}
}

func TestUDPExchangerConfirmsCappedPath(t *testing.T) {
	const pathLimit = 1320
	server, stopServer := startUDPProbeServer(t, pathLimit)
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()

	exchanger := &UDPExchanger{
		Conn:    client,
		Peer:    server.LocalAddr().(*net.UDPAddr),
		Timeout: 100 * time.Millisecond,
	}
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload:   1500,
		CandidatePayload: 1472,
		MinimumPayload:   600,
		Attempts:         1,
	}, exchanger.Exchange)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != pathLimit {
		t.Fatalf("confirmed payload = %d, want %d", result.PayloadSize, pathLimit)
	}
}

func TestUDPExchangerCommitsFinalConfirmedPath(t *testing.T) {
	const pathLimit = 1320
	confirmed := make(chan UDPConfirmedPath, 2)
	server, stopServer := startUDPProbeServerWithOptions(t, UDPResponderOptions{
		MaxPayload: pathLimit,
		OnConfirmed: func(value UDPConfirmedPath) {
			confirmed <- value
		},
	})
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()

	exchanger := &UDPExchanger{
		Conn: client, Peer: server.LocalAddr().(*net.UDPAddr), Timeout: 100 * time.Millisecond,
	}
	result, err := ConfirmPayload(context.Background(), ConfirmOptions{
		DesiredPayload: 1500, CandidatePayload: 1472, MinimumPayload: 600, Attempts: 1,
	}, exchanger.Exchange)
	if err != nil {
		t.Fatal(err)
	}
	if result.PayloadSize != pathLimit {
		t.Fatalf("confirmed payload = %d, want %d", result.PayloadSize, pathLimit)
	}
	if err := exchanger.Commit(context.Background(), result.PayloadSize, 3); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-confirmed:
		if value.PayloadSize != pathLimit || value.Peer != client.LocalAddr().(*net.UDPAddr).AddrPort() {
			t.Fatalf("committed path = %+v", value)
		}
	case <-time.After(time.Second):
		t.Fatal("responder did not publish committed path")
	}
	select {
	case duplicate := <-confirmed:
		t.Fatalf("duplicate confirmation callback = %+v", duplicate)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestUDPExchangerHonorsCancellation(t *testing.T) {
	client := listenUDP4(t)
	defer client.Close()
	unused := listenUDP4(t)
	peer := unused.LocalAddr().(*net.UDPAddr)
	if err := unused.Close(); err != nil {
		t.Fatal(err)
	}

	exchanger := &UDPExchanger{Conn: client, Peer: peer, Timeout: time.Second}
	request, err := NewProbeRequest(600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err = exchanger.Exchange(ctx, request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Exchange() error = %v, want context canceled", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatalf("canceled exchange took %s", time.Since(started))
	}
}

func TestUDPExchangerClearsCanceledDeadlineBeforeReuse(t *testing.T) {
	client := listenUDP4(t)
	defer client.Close()
	silent := listenUDP4(t)
	defer silent.Close()
	exchanger := &UDPExchanger{
		Conn: client, Peer: silent.LocalAddr().(*net.UDPAddr), Timeout: time.Second,
	}
	request, err := NewProbeRequest(600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := exchanger.Exchange(ctx, request); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Exchange() error = %v, want deadline exceeded", err)
	}

	server, stopServer := startUDPProbeServer(t, MaxUDPProbePayload)
	defer stopServer()
	exchanger.Peer = server.LocalAddr().(*net.UDPAddr)
	if _, err := exchanger.Exchange(context.Background(), request); err != nil {
		t.Fatalf("reused Exchange() error = %v", err)
	}
}

func TestUDPResponderIgnoresMalformedPackets(t *testing.T) {
	server, stopServer := startUDPProbeServer(t, MaxUDPProbePayload)
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()

	if _, err := client.WriteToUDP([]byte("not-a-probe"), server.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReadDeadline(time.Now().Add(25 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64)
	if _, _, err := client.ReadFromUDP(buffer); err == nil {
		t.Fatal("malformed request received a response")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("ReadFromUDP() error = %v, want timeout", err)
	}
}

func TestUDPResponderRejectsCommitWithoutSuccessfulProbe(t *testing.T) {
	server, stopServer := startUDPProbeServer(t, MaxUDPProbePayload)
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()
	request, err := NewProbeRequest(600)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := ProbeCommitFor(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.WriteToUDP(commit, server.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	if err := client.SetReadDeadline(time.Now().Add(25 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 700)
	if _, _, err := client.ReadFromUDP(buffer); err == nil {
		t.Fatal("unmatched commit received an acknowledgement")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("ReadFromUDP() error = %v, want timeout", err)
	}
}

func startUDPProbeServer(t *testing.T, maxPayload int) (*net.UDPConn, func()) {
	t.Helper()
	return startUDPProbeServerWithOptions(t, UDPResponderOptions{MaxPayload: maxPayload})
}

func startUDPProbeServerWithOptions(t *testing.T, options UDPResponderOptions) (*net.UDPConn, func()) {
	t.Helper()
	server := listenUDP4(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- ServeUDPProbeResponses(ctx, server, options)
	}()
	return server, func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("ServeUDPProbeResponses() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("UDP probe responder did not stop")
		}
		_ = server.Close()
	}
}

func listenUDP4(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}
