package pathmtu

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestConfirmerStoresOnlyPeerConfirmedPlan(t *testing.T) {
	key := testPathKey("connector-a", 9000)
	confirmedAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.FixedZone("test", 8*60*60))
	cache := NewCache()
	confirmer := &Confirmer{
		Cache:  cache,
		Runner: routeRunnerWithMTU(1500),
		Now:    func() time.Time { return confirmedAt },
	}
	entry, err := confirmer.ConfirmRawUDP(context.Background(), RawUDPConfirmationInput{
		Key: key, DeviceMTUCeiling: 1500, MinimumNetworkMTU: 576, Attempts: 1,
	}, cappedProbeExchange(1320))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Probe.PayloadSize != 1320 || entry.Plan.PathMTU != 1348 || entry.Plan.MaxFrameSize != 1500 || entry.Plan.EffectiveNetworkMTU != 1300 {
		t.Fatalf("confirmed entry = %+v", entry)
	}
	if !entry.ConfirmedAt.Equal(confirmedAt.UTC()) {
		t.Fatalf("confirmed at = %s, want %s", entry.ConfirmedAt, confirmedAt.UTC())
	}
	cached, ok := cache.Load(key)
	if !ok || cached != entry {
		t.Fatalf("cached entry = %+v, %v", cached, ok)
	}
}

func TestFailedReconfirmationLeavesPreviousPlan(t *testing.T) {
	key := testPathKey("connector-a", 9000)
	cache := NewCache()
	previous := ConfirmedPath{Key: key, Probe: ConfirmResult{PayloadSize: 1400}}
	cache.Store(previous)
	confirmer := &Confirmer{Cache: cache, Runner: routeRunnerWithMTU(1500)}

	_, err := confirmer.ConfirmRawUDP(context.Background(), RawUDPConfirmationInput{
		Key: key, DeviceMTUCeiling: 1500, MinimumNetworkMTU: 576, Attempts: 1,
	}, func(context.Context, []byte) ([]byte, error) { return nil, errors.New("path down") })
	if err == nil {
		t.Fatal("ConfirmRawUDP() error = nil")
	}
	cached, ok := cache.Load(key)
	if !ok || cached != previous {
		t.Fatalf("failed confirmation replaced cached plan: %+v, %v", cached, ok)
	}
}

func TestCacheInvalidationIsEndpointScoped(t *testing.T) {
	cache := NewCache()
	first := testPathKey("connector-a", 9000)
	second := testPathKey("connector-b", 9000)
	cache.Store(ConfirmedPath{Key: first})
	cache.Store(ConfirmedPath{Key: second})

	if removed := cache.InvalidateEndpoint("connector", "connector-a"); removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, ok := cache.Load(first); ok {
		t.Fatal("first endpoint was not invalidated")
	}
	if _, ok := cache.Load(second); !ok {
		t.Fatal("unrelated endpoint was invalidated")
	}
}

func TestConfirmerCommitsBothConnectorAndListenerPlans(t *testing.T) {
	listenerCache := NewCache()
	listenerConfirmer := &Confirmer{Cache: listenerCache, Runner: routeRunnerWithMTU(1500)}
	listenerResult := make(chan ConfirmedPath, 1)
	listenerError := make(chan error, 1)
	server, stopServer := startUDPProbeServerWithOptions(t, UDPResponderOptions{
		MaxPayload: 1320,
		OnConfirmed: func(committed UDPConfirmedPath) {
			entry, err := listenerConfirmer.AcceptRawUDPCommit(context.Background(), RawUDPConfirmationInput{
				Key:              PathKey{EndpointKind: "listener", EndpointID: "listener-a", DeviceID: "tun-b", Transport: "raw-udp"},
				DeviceMTUCeiling: 1500, MinimumNetworkMTU: 576, Attempts: 1,
			}, committed)
			if err != nil {
				listenerError <- err
				return
			}
			listenerResult <- entry
		},
	})
	defer stopServer()
	client := listenUDP4(t)
	defer client.Close()
	serverPeer := server.LocalAddr().(*net.UDPAddr)

	connectorCache := NewCache()
	connectorConfirmer := &Confirmer{Cache: connectorCache, Runner: routeRunnerWithMTU(1500)}
	key := PathKey{
		EndpointKind: "connector", EndpointID: "connector-a", DeviceID: "tun-a", Transport: "raw-udp",
		Remote: serverPeer.AddrPort(),
	}
	entry, err := connectorConfirmer.ConfirmAndCommitRawUDP(context.Background(), RawUDPConfirmationInput{
		Key: key, DeviceMTUCeiling: 1500, MinimumNetworkMTU: 576, Attempts: 1,
	}, &UDPExchanger{Conn: client, Peer: serverPeer, Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Probe.PayloadSize != 1320 {
		t.Fatalf("connector payload = %d, want 1320", entry.Probe.PayloadSize)
	}
	select {
	case err := <-listenerError:
		t.Fatal(err)
	case listener := <-listenerResult:
		if listener.Probe.PayloadSize != entry.Probe.PayloadSize || listener.Key.Remote != client.LocalAddr().(*net.UDPAddr).AddrPort() {
			t.Fatalf("listener = %+v, connector = %+v", listener, entry)
		}
	case <-time.After(time.Second):
		t.Fatal("listener did not compile committed path")
	}
	if _, ok := connectorCache.Load(key); !ok {
		t.Fatal("connector committed plan was not cached")
	}
}

func testPathKey(endpointID string, port uint16) PathKey {
	return PathKey{
		EndpointKind: "connector",
		EndpointID:   endpointID,
		DeviceID:     "tun-a",
		Transport:    "raw-udp",
		Remote:       netip.AddrPortFrom(netip.MustParseAddr("203.0.113.10"), port),
	}
}

func routeRunnerWithMTU(mtu int) CommandRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ip" || len(args) < 3 || args[1] != "route" {
			return nil, errors.New("unexpected command")
		}
		if mtu == 1500 {
			return []byte(`[{"dst":"203.0.113.10","dev":"eth0","prefsrc":"192.0.2.10","mtu":1500}]`), nil
		}
		return nil, errors.New("unexpected MTU")
	}
}

func cappedProbeExchange(limit int) ProbeExchange {
	return func(_ context.Context, request []byte) ([]byte, error) {
		if len(request) > limit {
			return nil, errors.New("probe lost")
		}
		return ProbeResponseFor(request)
	}
}
