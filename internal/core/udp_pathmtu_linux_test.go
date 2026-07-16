//go:build linux

package core

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/pathmtu"
)

func TestRawUDPPathPreparerConfirmsBothPeers(t *testing.T) {
	listener := listenCoreUDP4(t)
	defer listener.Close()
	connector := listenCoreUDP4(t)
	defer connector.Close()

	cache := pathmtu.NewCache()
	preparer := rawUDPPathPreparer{
		cache:        cache,
		runner:       fixedRouteMTURunner(1500),
		probeTimeout: 10 * time.Millisecond,
		commitGrace:  25 * time.Millisecond,
		handoffDelay: 35 * time.Millisecond,
	}
	device := config.RuntimeDevice{ID: "tun0", Type: model.DeviceTUN, MTU: 1500}
	listenerPipe := config.RuntimeUDPPipe{
		EndpointID: "listener", EndpointKind: "listener", DeviceID: device.ID,
		MaxFrameSize: 1500, LinkAutoOptimize: true,
	}
	connectorPipe := listenerPipe
	connectorPipe.EndpointID = "connector"
	connectorPipe.EndpointKind = "connector"

	type result struct {
		pipe config.RuntimeUDPPipe
		peer netip.AddrPort
		err  error
	}
	listenerResult := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		pipe, peer, err := preparer.prepare(ctx, listenerPipe, device, listener, netip.AddrPort{})
		listenerResult <- result{pipe: pipe, peer: peer, err: err}
	}()

	connectorPeer := listener.LocalAddr().(*net.UDPAddr).AddrPort()
	preparedConnector, confirmedConnectorPeer, err := preparer.prepare(ctx, connectorPipe, device, connector, connectorPeer)
	if err != nil {
		t.Fatalf("prepare connector: %v", err)
	}
	preparedListener := <-listenerResult
	if preparedListener.err != nil {
		t.Fatalf("prepare listener: %v", preparedListener.err)
	}
	if confirmedConnectorPeer != connectorPeer {
		t.Fatalf("connector peer = %v, want %v", confirmedConnectorPeer, connectorPeer)
	}
	wantListenerPeer := connector.LocalAddr().(*net.UDPAddr).AddrPort()
	if preparedListener.peer != wantListenerPeer {
		t.Fatalf("listener peer = %v, want %v", preparedListener.peer, wantListenerPeer)
	}
	if preparedConnector.MaxDatagramPayload <= pathmtu.SegmentHeaderSize {
		t.Fatalf("connector max datagram payload = %d", preparedConnector.MaxDatagramPayload)
	}
	if preparedListener.pipe.MaxDatagramPayload != preparedConnector.MaxDatagramPayload ||
		preparedListener.pipe.ConfirmedPathMTU != preparedConnector.ConfirmedPathMTU ||
		preparedListener.pipe.EffectiveNetworkMTU != preparedConnector.EffectiveNetworkMTU {
		t.Fatalf("confirmed plans differ: listener=%+v connector=%+v", preparedListener, preparedConnector)
	}
	for _, key := range []pathmtu.PathKey{
		rawUDPConfirmationInput(listenerPipe, device, wantListenerPeer).Key,
		rawUDPConfirmationInput(connectorPipe, device, connectorPeer).Key,
	} {
		if _, ok := cache.Load(key); !ok {
			t.Fatalf("confirmed path missing from cache for %+v", key)
		}
	}
}

func TestRawUDPPathPreparerConfirmsSymmetricFixedListeners(t *testing.T) {
	left := listenCoreUDP4(t)
	defer left.Close()
	right := listenCoreUDP4(t)
	defer right.Close()

	cache := pathmtu.NewCache()
	preparer := rawUDPPathPreparer{
		cache: cache, runner: fixedRouteMTURunner(1320),
		probeTimeout: 25 * time.Millisecond, handoffDelay: 50 * time.Millisecond,
	}
	device := config.RuntimeDevice{ID: "tun0", Type: model.DeviceTUN, MTU: 1500}
	leftPipe := config.RuntimeUDPPipe{
		EndpointID: "left-listener", EndpointKind: "listener", DeviceID: device.ID,
		MaxFrameSize: 1500, LinkAutoOptimize: true,
	}
	rightPipe := leftPipe
	rightPipe.EndpointID = "right-listener"

	type result struct {
		pipe config.RuntimeUDPPipe
		peer netip.AddrPort
		err  error
	}
	results := make(chan result, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		pipe, peer, err := preparer.prepare(ctx, leftPipe, device, left, right.LocalAddr().(*net.UDPAddr).AddrPort())
		results <- result{pipe: pipe, peer: peer, err: err}
	}()
	go func() {
		pipe, peer, err := preparer.prepare(ctx, rightPipe, device, right, left.LocalAddr().(*net.UDPAddr).AddrPort())
		results <- result{pipe: pipe, peer: peer, err: err}
	}()

	first := <-results
	second := <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("symmetric listener confirmation errors: first=%v second=%v", first.err, second.err)
	}
	if first.pipe.ConfirmedPathMTU != 1548 || second.pipe.ConfirmedPathMTU != 1548 {
		t.Fatalf("confirmed path MTUs = %d and %d, want peer-confirmed 1548", first.pipe.ConfirmedPathMTU, second.pipe.ConfirmedPathMTU)
	}
	if first.pipe.MaxDatagramPayload != second.pipe.MaxDatagramPayload {
		t.Fatalf("symmetric listener payloads differ: %d and %d", first.pipe.MaxDatagramPayload, second.pipe.MaxDatagramPayload)
	}
	for _, key := range []pathmtu.PathKey{
		rawUDPConfirmationInput(leftPipe, device, right.LocalAddr().(*net.UDPAddr).AddrPort()).Key,
		rawUDPConfirmationInput(rightPipe, device, left.LocalAddr().(*net.UDPAddr).AddrPort()).Key,
	} {
		if _, ok := cache.Load(key); !ok {
			t.Fatalf("symmetric listener path missing from cache for %+v", key)
		}
	}
}

func TestRawUDPPathPreparerConfirmsDTLSApplicationPayload(t *testing.T) {
	connectorConn, listenerConn := net.Pipe()
	defer connectorConn.Close()
	defer listenerConn.Close()
	preparer := rawUDPPathPreparer{
		cache: pathmtu.NewCache(), runner: fixedRouteMTURunner(1500),
		probeTimeout: 10 * time.Millisecond, commitGrace: 25 * time.Millisecond, handoffDelay: 35 * time.Millisecond,
	}
	device := config.RuntimeDevice{ID: "tun0", Type: model.DeviceTUN, MTU: 1500}
	listenerPipe := config.RuntimeUDPPipe{
		EndpointID: "dtls-listener", EndpointKind: "listener", DeviceID: device.ID,
		MaxFrameSize: 1500, LinkAutoOptimize: true,
	}
	connectorPipe := listenerPipe
	connectorPipe.EndpointID = "dtls-connector"
	connectorPipe.EndpointKind = "connector"
	listenerPeer := netip.MustParseAddrPort("192.0.2.10:4433")
	connectorPeer := netip.MustParseAddrPort("198.51.100.20:55000")

	type result struct {
		pipe config.RuntimeUDPPipe
		err  error
	}
	listenerResult := make(chan result, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		pipe, err := preparer.prepareConn(ctx, listenerPipe, device, listenerConn, connectorPeer, 37)
		listenerResult <- result{pipe: pipe, err: err}
	}()
	preparedConnector, err := preparer.prepareConn(ctx, connectorPipe, device, connectorConn, listenerPeer, 37)
	if err != nil {
		t.Fatalf("prepare DTLS connector: %v", err)
	}
	preparedListener := <-listenerResult
	if preparedListener.err != nil {
		t.Fatalf("prepare DTLS listener: %v", preparedListener.err)
	}
	if preparedConnector.MaxDatagramPayload != 1520 || preparedConnector.EffectiveNetworkMTU != 1500 {
		t.Fatalf("DTLS connector plan = %+v", preparedConnector)
	}
	if preparedListener.pipe.MaxDatagramPayload != preparedConnector.MaxDatagramPayload ||
		preparedListener.pipe.ConfirmedPathMTU != preparedConnector.ConfirmedPathMTU {
		t.Fatalf("DTLS plans differ: listener=%+v connector=%+v", preparedListener.pipe, preparedConnector)
	}
}

func TestDuplicateUDPConnDoesNotOwnOriginalSocket(t *testing.T) {
	original := listenCoreUDP4(t)
	defer original.Close()
	raw, err := original.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	fd := -1
	if err := raw.Control(func(value uintptr) { fd = int(value) }); err != nil {
		t.Fatal(err)
	}
	duplicate, err := duplicateUDPConn(fd)
	if err != nil {
		t.Fatal(err)
	}
	if err := duplicate.Close(); err != nil {
		t.Fatal(err)
	}
	if err := original.SetReadDeadline(time.Now()); err != nil {
		t.Fatalf("original socket was closed with duplicate: %v", err)
	}
}

func listenCoreUDP4(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func fixedRouteMTURunner(mtu int) pathmtu.CommandRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ip" || len(args) < 4 || strings.Join(args[:3], " ") != "-json route get" {
			return nil, fmt.Errorf("unexpected route command: %s %v", name, args)
		}
		return []byte(fmt.Sprintf(`[{"dst":%q,"dev":"lo","prefsrc":"127.0.0.1","mtu":%d}]`, args[3], mtu)), nil
	}
}
