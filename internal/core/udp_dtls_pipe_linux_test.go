//go:build linux

package core

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/pion/dtls/v3"

	"tapx/internal/config"
	"tapx/internal/linkdiag"
	"tapx/internal/model"
	"tapx/internal/pathmtu"
)

func TestDTLSCipherSuiteRecordOverhead(t *testing.T) {
	tests := []struct {
		id   dtls.CipherSuiteID
		want int
	}{
		{dtls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, 37},
		{dtls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, 29},
		{dtls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA, 65},
		{dtls.TLS_PSK_WITH_AES_128_CBC_SHA256, 77},
	}
	for _, test := range tests {
		got, err := dtlsCipherSuiteRecordOverhead(test.id)
		if err != nil {
			t.Fatalf("cipher suite %v: %v", test.id, err)
		}
		if got != test.want {
			t.Fatalf("cipher suite %v overhead = %d, want %d", test.id, got, test.want)
		}
	}
}

func TestDTLSCipherSuiteRecordOverheadRejectsUnknownSuite(t *testing.T) {
	if _, err := dtlsCipherSuiteRecordOverhead(dtls.CipherSuiteID(0xffff)); err == nil {
		t.Fatal("unknown cipher suite was accepted")
	}
}

func TestDTLSVKeyControlDistinguishesDataAndDiagnosticSessions(t *testing.T) {
	for _, test := range []struct {
		name       string
		diagnostic bool
	}{
		{name: "data"},
		{name: "diagnostic", diagnostic: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			result := make(chan struct {
				hello dtlsVKeyHello
				err   error
			}, 1)
			go func() {
				hello, err := receiveDTLSVKeyHello(context.Background(), server, time.Second)
				result <- struct {
					hello dtlsVKeyHello
					err   error
				}{hello: hello, err: err}
			}()
			var err error
			if test.diagnostic {
				err = sendDTLSDiagnosticHello(context.Background(), client, "alpha", time.Second)
			} else {
				err = sendDTLSVKeyHello(context.Background(), client, "alpha", time.Second)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := <-result
			if got.err != nil {
				t.Fatal(got.err)
			}
			if got.hello.vkey != "alpha" || got.hello.diagnostic != test.diagnostic {
				t.Fatalf("hello = %+v, want vKey alpha diagnostic=%v", got.hello, test.diagnostic)
			}
		})
	}
}

func TestDTLSPathConfirmationOverNegotiatedConnection(t *testing.T) {
	certificate := testDTLSCertificate(t)
	type accepted struct {
		conn   *dtls.Conn
		remote netip.AddrPort
		err    error
	}
	serverResult := make(chan accepted, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	listener, err := dtls.ListenWithOptions("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)},
		dtls.WithCertificates(certificate))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		rawConn, err := listener.Accept()
		if err != nil {
			serverResult <- accepted{err: err}
			return
		}
		conn, ok := rawConn.(*dtls.Conn)
		if !ok {
			_ = rawConn.Close()
			serverResult <- accepted{err: &net.AddrError{Err: "accepted non-DTLS connection", Addr: rawConn.RemoteAddr().String()}}
			return
		}
		remoteAddr, err := addrPortFromNetAddr(conn.RemoteAddr())
		serverResult <- accepted{conn: conn, remote: remoteAddr, err: err}
	}()
	serverAddr := listener.Addr().(*net.UDPAddr)
	clientConn, err := dtls.DialWithOptions("udp4", serverAddr,
		dtls.WithInsecureSkipVerify(true)) //nolint:gosec -- test certificate
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()
	clientHandshake := make(chan error, 1)
	go func() { clientHandshake <- clientConn.Handshake() }()
	server := <-serverResult
	if server.err != nil {
		t.Fatal(server.err)
	}
	defer server.conn.Close()
	if err := server.conn.Handshake(); err != nil {
		t.Fatal(err)
	}
	if err := <-clientHandshake; err != nil {
		t.Fatal(err)
	}
	overhead, err := dtlsRecordOverhead(clientConn)
	if err != nil {
		t.Fatal(err)
	}

	preparer := rawUDPPathPreparer{
		cache: pathmtu.NewCache(), runner: fixedRouteMTURunner(1500),
		probeTimeout: 50 * time.Millisecond, commitGrace: 100 * time.Millisecond, handoffDelay: 120 * time.Millisecond,
	}
	device := config.RuntimeDevice{ID: "tun0", Type: model.DeviceTUN, MTU: 1500}
	listenerPipe := config.RuntimeUDPPipe{
		EndpointID: "dtls-listener", EndpointKind: "listener", DeviceID: device.ID,
		MaxFrameSize: 1500, LinkAutoOptimize: true,
	}
	connectorPipe := listenerPipe
	connectorPipe.EndpointID = "dtls-connector"
	connectorPipe.EndpointKind = "connector"
	type result struct {
		pipe config.RuntimeUDPPipe
		err  error
	}
	listenerResult := make(chan result, 1)
	go func() {
		pipe, err := preparer.prepareConn(ctx, listenerPipe, device, server.conn, server.remote, overhead)
		listenerResult <- result{pipe: pipe, err: err}
	}()
	preparedConnector, err := preparer.prepareConn(ctx, connectorPipe, device, clientConn, serverAddr.AddrPort(), overhead)
	if err != nil {
		t.Fatalf("prepare DTLS connector: %v", err)
	}
	preparedListener := <-listenerResult
	if preparedListener.err != nil {
		t.Fatalf("prepare DTLS listener: %v", preparedListener.err)
	}
	if preparedConnector.MaxDatagramPayload != preparedListener.pipe.MaxDatagramPayload ||
		preparedConnector.ConfirmedPathMTU != preparedListener.pipe.ConfirmedPathMTU {
		t.Fatalf("negotiated DTLS plans differ: listener=%+v connector=%+v", preparedListener.pipe, preparedConnector)
	}
	if preparedConnector.EffectiveNetworkMTU != 1500 {
		t.Fatalf("effective network MTU = %d, want 1500", preparedConnector.EffectiveNetworkMTU)
	}
}

func TestUDPPipeDiagnoseDTLSEndToEnd(t *testing.T) {
	certificate := testDTLSCertificate(t)
	listener, err := dtls.ListenWithOptions("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)},
		dtls.WithCertificates(certificate), dtls.WithMTU(1200))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverCtx, cancelServer := context.WithCancel(context.Background())
	defer cancelServer()
	serverResult := make(chan error, 1)
	go func() {
		for range 3 {
			rawConn, err := listener.Accept()
			if err != nil {
				serverResult <- err
				return
			}
			conn, ok := rawConn.(*dtls.Conn)
			if !ok {
				_ = rawConn.Close()
				serverResult <- fmt.Errorf("accepted %T, want *dtls.Conn", rawConn)
				return
			}
			if err := conn.HandshakeContext(serverCtx); err != nil {
				_ = conn.Close()
				serverResult <- err
				return
			}
			hello, err := receiveDTLSVKeyHello(serverCtx, conn, time.Second)
			if err != nil {
				_ = conn.Close()
				serverResult <- err
				return
			}
			if !hello.diagnostic || hello.vkey != "alpha" {
				_ = conn.Close()
				serverResult <- fmt.Errorf("hello = %+v, want diagnostic alpha", hello)
				return
			}
			if err := linkdiag.ServeStream(serverCtx, conn, hello.vkey); err != nil &&
				!errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				serverResult <- err
				return
			}
		}
		serverResult <- nil
	}()

	serverAddr := listener.Addr().(*net.UDPAddr).AddrPort()
	handle := &UDPPipeHandle{
		Pipe: config.RuntimeUDPPipe{
			EndpointID: "dtls-diagnostic", Remote: serverAddr.Addr().String(), Port: serverAddr.Port(),
			DTLS:    model.RawDTLSSettings{Enabled: true, AllowInsecure: true, MTU: 1200},
			Binding: config.RuntimeBinding{VKeyValue: "alpha"}, ConfirmedPathMTU: 1472, EffectiveNetworkMTU: 1472,
			TCPMSSIPv4: 1432, TCPMSSIPv6: 1412,
		},
		RemoteAddr: serverAddr,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	channel, err := handle.Diagnose(ctx, "channel", 0)
	if err != nil || channel.Delay <= 0 || channel.Transport != "dtls" {
		t.Fatalf("channel = %+v, err=%v", channel, err)
	}
	throughput, err := handle.Diagnose(ctx, "throughput", 25*time.Millisecond)
	if err != nil || throughput.UploadBytes == 0 || throughput.DownloadBytes == 0 {
		t.Fatalf("throughput = %+v, err=%v", throughput, err)
	}
	path, err := handle.Diagnose(ctx, "path-mtu", 0)
	if err != nil || path.PathMTU != 1472 || path.TCPMSS != 1432 {
		t.Fatalf("path = %+v, err=%v", path, err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}

func testDTLSCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey}
}
