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

func TestAddressLeaseRoundTrip(t *testing.T) {
	server, client := net.Pipe()
	serverErr := make(chan error, 1)
	requestCh := make(chan AddressLeaseRequest, 1)
	go func() {
		serverErr <- ServeStreamWithOptions(context.Background(), server, StreamOptions{
			Credential: "vkey",
			AddressLease: func(request AddressLeaseRequest) (AddressLease, error) {
				requestCh <- request
				return AddressLease{IPv4CIDR: "10.20.0.2/24", IPv6CIDR: "fd20::2/64", Gateway: "10.20.0.1", DNS: []string{"1.1.1.1"}}, nil
			},
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, err := RequestAddressLease(ctx, client, "vkey", AddressLeaseRequest{Key: "client-a", Protocol: "dual"})
	if err != nil {
		t.Fatal(err)
	}
	if lease.IPv4CIDR != "10.20.0.2/24" || lease.IPv6CIDR != "fd20::2/64" || lease.Gateway != "10.20.0.1" {
		t.Fatalf("unexpected lease: %+v", lease)
	}
	request := <-requestCh
	if request.Key != "client-a" || request.Protocol != "dual" {
		t.Fatalf("unexpected request: %+v", request)
	}
	_ = client.Close()
	if err := <-serverErr; err == nil {
		t.Fatal("server should stop after the client closes")
	}
}

func TestStreamOneWayThroughput(t *testing.T) {
	for _, test := range []struct {
		name   string
		upload bool
	}{
		{name: "upload", upload: true},
		{name: "download", upload: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, client := net.Pipe()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errCh := make(chan error, 1)
			go func() { errCh <- ServeStream(ctx, server, "secret") }()
			result, err := ThroughputOneWay(ctx, client, "secret", 20*time.Millisecond, test.upload)
			if err != nil {
				t.Fatalf("ThroughputOneWay() error = %v", err)
			}
			if test.upload && (result.UploadBytes == 0 || result.DownloadBytes != 0) {
				t.Fatalf("upload result = %+v", result)
			}
			if !test.upload && (result.DownloadBytes == 0 || result.UploadBytes != 0) {
				t.Fatalf("download result = %+v", result)
			}
			_ = client.Close()
			<-errCh
		})
	}
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
