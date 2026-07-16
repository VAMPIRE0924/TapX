package linkdiag

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestInspectStreamHelloPrefix(t *testing.T) {
	hello := make([]byte, len(streamMagic)+3+6)
	copy(hello, streamMagic)
	hello[len(streamMagic)] = streamVersion
	binary.BigEndian.PutUint16(hello[len(streamMagic)+1:], 6)
	copy(hello[len(streamMagic)+3:], "secret")

	partial, err := InspectStreamHelloPrefix(hello[:5])
	if err != nil || !partial.Matched || partial.Complete || partial.Required != len(streamMagic) {
		t.Fatalf("partial inspection = %+v, %v", partial, err)
	}
	complete, err := InspectStreamHelloPrefix(hello)
	if err != nil || !complete.Matched || !complete.Complete || complete.Credential != "secret" || complete.Required != len(hello) {
		t.Fatalf("complete inspection = %+v, %v", complete, err)
	}
	plain, err := InspectStreamHelloPrefix([]byte("plain traffic"))
	if err != nil || plain.Matched {
		t.Fatalf("plain inspection = %+v, %v", plain, err)
	}
}

func TestStreamPingAndThroughput(t *testing.T) {
	server, client := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ServeStream(ctx, server, "secret") }()

	delay, err := Ping(ctx, client, "secret")
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
	_ = delay
	_ = client.Close()
	if err := <-errCh; err == nil {
		t.Fatal("ServeStream() error = nil after peer close")
	}

	server, client = net.Pipe()
	errCh = make(chan error, 1)
	go func() { errCh <- ServeStream(ctx, server, "secret") }()
	result, err := Throughput(ctx, client, "secret", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("Throughput() error = %v", err)
	}
	if result.UploadBytes == 0 || result.DownloadBytes == 0 || result.UploadBPS == 0 || result.DownloadBPS == 0 {
		t.Fatalf("Throughput() = %+v", result)
	}
	_ = client.Close()
	<-errCh
}

func TestStreamFrameProbe(t *testing.T) {
	server, client := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ServeStream(ctx, server, "secret") }()
	if _, err := ProbeFrame(ctx, client, "secret", 1522); err != nil {
		t.Fatalf("ProbeFrame() error = %v", err)
	}
	_ = client.Close()
	<-errCh
}
