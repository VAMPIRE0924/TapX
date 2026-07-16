package panel

import (
	"context"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/fastpath"
	"tapx/internal/model"
)

func TestTrafficResetDueBoundaries(t *testing.T) {
	location := time.FixedZone("test", 8*60*60)
	now := time.Date(2026, 7, 11, 12, 34, 56, 0, location)
	tests := []struct {
		mode string
		last time.Time
		want bool
	}{
		{mode: "hourly", last: now.Add(-time.Hour), want: true},
		{mode: "hourly", last: now.Add(-time.Minute), want: false},
		{mode: "daily", last: now.AddDate(0, 0, -1), want: true},
		{mode: "weekly", last: now.AddDate(0, 0, -7), want: true},
		{mode: "monthly", last: now.AddDate(0, -1, 0), want: true},
		{mode: "never", last: time.Time{}, want: false},
	}
	for _, test := range tests {
		if got := trafficResetDue(test.mode, test.last.Unix(), now); got != test.want {
			t.Errorf("trafficResetDue(%q, %s) = %v, want %v", test.mode, test.last, got, test.want)
		}
	}
}

func TestSettingsLocationUsesEnabledTimezone(t *testing.T) {
	location := settingsLocation([]model.Settings{{Enabled: true, Timezone: "Asia/Hong_Kong"}})
	_, offset := time.Date(2026, 7, 12, 0, 0, 0, 0, location).Zone()
	if offset != 8*60*60 {
		t.Fatalf("timezone offset = %d, want %d", offset, 8*60*60)
	}
}

func TestRefreshLimitConfigResetsDueListenerAndClient(t *testing.T) {
	store := newTestStore(t)
	cfg := config.RuntimeConfig{
		Listeners: []model.Listener{{
			ID: "listener-a", Enabled: true, BindHost: "127.0.0.1", BindPort: 44000,
			Transport: model.TransportUDP, RawUDP: model.RawUDPSettings{PeerMode: model.UDPPeerAny}, TrafficReset: "hourly",
		}},
		Clients: []model.Client{{ID: "client-a", Enabled: true, TrafficReset: "daily"}},
	}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	state := RuntimeState{Generation: 9, UDPPipes: []RuntimePipeState{{
		EndpointID: "listener-a", EndpointKind: "listener", ClientID: "client-a",
		Counters: fastpath.CountersSnapshot{RXBytes: 300, TXBytes: 100},
	}}}
	now := time.Date(2026, 7, 11, 12, 34, 56, 0, time.Local)

	next, err := refreshLimitConfig(store, state, now)
	if err != nil {
		t.Fatalf("refresh limit config: %v", err)
	}
	listener := next.Listeners[0]
	client := next.Clients[0]
	if listener.TrafficResetGeneration != 9 || listener.TrafficRXOffset != 300 || listener.TrafficTXOffset != 100 {
		t.Fatalf("listener reset = %+v", listener)
	}
	if client.TrafficResetGeneration != 9 || client.TrafficRXOffset != 300 || client.TrafficTXOffset != 100 {
		t.Fatalf("client reset = %+v", client)
	}
}

func TestRefreshLimitConfigStartsDelayedClientOnFirstTraffic(t *testing.T) {
	store := newTestStore(t)
	cfg := config.RuntimeConfig{Clients: []model.Client{
		{ID: "waiting", Enabled: true, ExpiresAt: -7 * 86400},
		{ID: "active", Enabled: true, ExpiresAt: -3 * 86400},
	}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	now := time.Date(2026, 7, 12, 8, 0, 0, 0, time.Local)
	state := RuntimeState{UDPPipes: []RuntimePipeState{
		{ClientID: "waiting"},
		{ClientID: "active", Counters: fastpath.CountersSnapshot{RXPackets: 1, RXBytes: 64}},
	}}

	next, err := refreshLimitConfig(store, state, now)
	if err != nil {
		t.Fatalf("refresh limit config: %v", err)
	}
	clients := map[string]model.Client{}
	for _, client := range next.Clients {
		clients[client.ID] = client
	}
	if clients["waiting"].ExpiresAt != -7*86400 {
		t.Fatalf("waiting client expiry = %d, want delayed value unchanged", clients["waiting"].ExpiresAt)
	}
	want := now.Add(3 * 24 * time.Hour).Unix()
	if clients["active"].ExpiresAt != want {
		t.Fatalf("active client expiry = %d, want %d", clients["active"].ExpiresAt, want)
	}
	stored, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load stored config: %v", err)
	}
	storedClients := map[string]model.Client{}
	for _, client := range stored.Clients {
		storedClients[client.ID] = client
	}
	if storedClients["active"].ExpiresAt != want {
		t.Fatalf("stored active client expiry = %d, want %d", storedClients["active"].ExpiresAt, want)
	}
}
