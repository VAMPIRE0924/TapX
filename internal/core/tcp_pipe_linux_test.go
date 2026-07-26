//go:build linux

package core

import (
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestListenTCPAppliesDerivedQueueBuffers(t *testing.T) {
	pipe := config.RuntimeTCPPipe{
		EndpointID:   "tcp-listener",
		EndpointKind: "listener",
		FrameKind:    model.DeviceTUN,
		BindHost:     "127.0.0.1",
		QueueSize:    8,
		MaxFrameSize: 2048,
		TCPMaxSeg:    1200,
	}

	listener, local, err := listenTCP(pipe)
	if err != nil {
		t.Fatalf("listenTCP() error = %v", err)
	}
	defer listener.Close()

	if got := local.Addr().String(); got != "127.0.0.1" {
		t.Fatalf("local addr = %s, want 127.0.0.1", got)
	}

	file, err := listener.File()
	if err != nil {
		t.Fatalf("listener file: %v", err)
	}
	defer file.Close()
	fd := int(file.Fd())
	wantBuffer := socketQueueBytes(pipe.QueueSize, pipe.MaxFrameSize)
	assertSockoptAtLeast(t, fd, unix.SO_RCVBUF, wantBuffer)
	assertSockoptAtLeast(t, fd, unix.SO_SNDBUF, wantBuffer)

	client, err := net.DialTimeout("tcp", local.String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial listener: %v", err)
	}
	defer client.Close()
	accepted, err := listener.AcceptTCP()
	if err != nil {
		t.Fatalf("accept listener connection: %v", err)
	}
	defer accepted.Close()
	acceptedFile, err := accepted.File()
	if err != nil {
		t.Fatalf("accepted connection file: %v", err)
	}
	defer acceptedFile.Close()
	if got, err := unix.GetsockoptInt(int(acceptedFile.Fd()), unix.IPPROTO_TCP, unix.TCP_MAXSEG); err != nil {
		t.Fatalf("getsockopt TCP_MAXSEG: %v", err)
	} else if got <= 0 || got > pipe.TCPMaxSeg {
		t.Fatalf("accepted TCP_MAXSEG = %d, want a positive value no greater than %d", got, pipe.TCPMaxSeg)
	}
}

func TestDialTCPUsesRemoteAddressAndDerivedQueueBuffers(t *testing.T) {
	server, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen test server: %v", err)
	}
	defer server.Close()

	accepted := make(chan *net.TCPConn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := server.AcceptTCP()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()

	_, portText, err := net.SplitHostPort(server.Addr().String())
	if err != nil {
		t.Fatalf("split server addr: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse server port: %v", err)
	}

	conn, local, remote, err := dialTCP(config.RuntimeTCPPipe{
		EndpointID:     "tcp-connector",
		EndpointKind:   "connector",
		Remote:         "127.0.0.1",
		Port:           uint16(port),
		QueueSize:      8,
		MaxFrameSize:   2048,
		NoDelay:        true,
		ConnectTimeout: 2,
	})
	if err != nil {
		t.Fatalf("dialTCP() error = %v", err)
	}
	defer conn.Close()

	if got := local.Addr().String(); got != "127.0.0.1" {
		t.Fatalf("local addr = %s, want 127.0.0.1", got)
	}
	if remote.Port() != uint16(port) {
		t.Fatalf("remote port = %d, want %d", remote.Port(), port)
	}
	pathMTU, mss, family := tcpConnPathMetrics(conn)
	if pathMTU <= 0 || mss <= 0 || family != "ipv4" {
		t.Fatalf("tcp path metrics = mtu %d, mss %d, family %q", pathMTU, mss, family)
	}

	select {
	case acceptedConn := <-accepted:
		acceptedConn.Close()
	case err := <-acceptErr:
		t.Fatalf("accept error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not accept tcp connection")
	}
}

func TestListenTCPRejectsInvalidBindHost(t *testing.T) {
	_, _, err := listenTCP(config.RuntimeTCPPipe{
		EndpointID:   "tcp-listener",
		EndpointKind: "listener",
		BindHost:     "not-an-ip",
	})
	if err == nil {
		t.Fatal("listenTCP() error = nil, want invalid bind address error")
	}
}
