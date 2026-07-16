package panel

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

func TestOpenStoreRestrictsDatabasePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "tapx.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat database: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database permissions = %04o, want 0600", got)
	}
}

func TestPostgresQueryBinding(t *testing.T) {
	query := bindQuery(DatabasePostgres, `SELECT payload FROM tapx_objects WHERE kind = ? AND id = ?`)
	if query != `SELECT payload FROM tapx_objects WHERE kind = $1 AND id = $2` {
		t.Fatalf("postgres query = %q", query)
	}
	if query := bindQuery(DatabaseSQLite, `SELECT ?`); query != `SELECT ?` {
		t.Fatalf("sqlite query changed to %q", query)
	}
}

func TestOpenStoreRejectsUnknownDatabaseDriver(t *testing.T) {
	_, err := OpenStoreWithOptions(StoreOptions{Driver: "mysql", DataSource: "ignored"})
	if err == nil || !strings.Contains(err.Error(), "unsupported database driver") {
		t.Fatalf("OpenStoreWithOptions() error = %v", err)
	}
}

func TestStoreReplaceLoadAndGenerateRuntime(t *testing.T) {
	store := newTestStore(t)

	cfg := sampleConfig()
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}

	loaded, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Devices) != 1 || loaded.Devices[0].ID != "tun-a" {
		t.Fatalf("unexpected devices: %+v", loaded.Devices)
	}
	if !loaded.Devices[0].LinkAutoOptimize {
		t.Fatalf("automatic link optimization setting was not persisted: %+v", loaded.Devices[0])
	}
	if len(loaded.XrayProfiles) != 1 || loaded.XrayProfiles[0].ID != "xr-a" {
		t.Fatalf("unexpected xray profiles: %+v", loaded.XrayProfiles)
	}
	if len(loaded.Settings) != 1 || loaded.Settings[0].ID != "global" {
		t.Fatalf("unexpected settings: %+v", loaded.Settings)
	}
	runtime, err := config.GenerateRuntime(loaded)
	if err != nil {
		t.Fatalf("generate runtime: %v", err)
	}
	if len(runtime.UDPPipes) != 1 {
		t.Fatalf("expected one udp pipe, got %+v", runtime.UDPPipes)
	}
	if runtime.UDPPipes[0].Binding.VKeyValue != "vk-secret" {
		t.Fatalf("expected routed vkey value, got %+v", runtime.UDPPipes[0].Binding)
	}
	if got := runtime.UDPPipes[0].AddressGuard.IPv4CIDRs; len(got) != 1 || got[0] != "10.10.0.2/32" {
		t.Fatalf("expected address guard, got %+v", runtime.UDPPipes[0].AddressGuard)
	}
}

func TestStoreRoundTripsEveryObjectKind(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig()
	cfg.Connectors = []model.Connector{{
		ID: "tcp-out", Enabled: true, Name: "TLS connector", Remote: "198.51.100.20", Port: 5000,
		Transport: model.TransportTCP,
		RawTCP: model.RawTCPSettings{
			LengthMode: model.TCPLength32, QueueSize: 4096, ZeroCopy: true,
			TLS: model.RawTLSSettings{Enabled: true, ServerName: "edge.example", AllowInsecure: true},
		},
		Binding: model.Binding{RouteID: "route-a"},
	}}
	cfg.Clients = []model.Client{{
		ID: "client-a", Enabled: true, Name: "operator", ListenerID: "udp-a",
		CredentialType: "vkey", CredentialValue: "vk-secret", UUID: "11111111-1111-4111-8111-111111111111",
		Password: "password", Auth: "hysteria-auth", AllowedDeviceIDs: []string{"tun-a"},
		AddressID: "addr-a", Binding: model.Binding{RouteID: "route-a"}, TrafficCap: 1 << 30,
		UploadRateLimit: 3_000_000, DownloadRateLimit: 5_000_000,
	}}
	cfg.Settings[0].AdvancedJSON = `{"tapx":{"workers":4}}`
	cfg.XrayProfiles[0].SockoptJSON = `{"mark":9}`

	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace complete config: %v", err)
	}
	loaded, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load complete config: %v", err)
	}
	counts := []int{
		len(loaded.Devices), len(loaded.Listeners), len(loaded.Connectors), len(loaded.Clients),
		len(loaded.Routes), len(loaded.VKeys), len(loaded.Addresses), len(loaded.XrayProfiles), len(loaded.Settings),
	}
	for index, count := range counts {
		if count != 1 {
			t.Fatalf("object kind %d count = %d, want 1", index, count)
		}
	}
	if loaded.Connectors[0].RawTCP.LengthMode != model.TCPLength32 || !loaded.Connectors[0].RawTCP.TLS.Enabled {
		t.Fatalf("connector settings were not preserved: %+v", loaded.Connectors[0])
	}
	if loaded.Clients[0].Auth != "hysteria-auth" || loaded.Clients[0].TrafficCap != 1<<30 ||
		loaded.Clients[0].UploadRateLimit != 3_000_000 || loaded.Clients[0].DownloadRateLimit != 5_000_000 {
		t.Fatalf("client credentials or limits were not preserved: %+v", loaded.Clients[0])
	}
	if loaded.XrayProfiles[0].SockoptJSON != `{"mark":9}` || loaded.Settings[0].AdvancedJSON != `{"tapx":{"workers":4}}` {
		t.Fatalf("advanced settings were not preserved: profile=%+v settings=%+v", loaded.XrayProfiles[0], loaded.Settings[0])
	}
}

func TestStoreDropsRemovedUserCredentialFields(t *testing.T) {
	store := newTestStore(t)
	raw := []byte(`{
		"ID":"legacy-user","Enabled":true,"Name":"legacy",
		"Security":"auto","Flow":"xtls-rprx-vision","ReverseTag":"reverse",
		"WireguardPrivateKey":"private","WireguardPublicKey":"public",
		"WireguardPreSharedKey":"psk","WireguardAllowedIPs":["10.0.0.2/32"]
	}`)
	if _, err := store.UpsertObject(context.Background(), KindClients, "legacy-user", raw); err != nil {
		t.Fatalf("upsert legacy user: %v", err)
	}
	persisted, err := store.GetObject(context.Background(), KindClients, "legacy-user")
	if err != nil {
		t.Fatalf("get normalized user: %v", err)
	}
	for _, removed := range []string{"Security", "Flow", "ReverseTag", "WireguardPrivateKey", "WireguardPublicKey", "WireguardPreSharedKey", "WireguardAllowedIPs"} {
		if strings.Contains(string(persisted), removed) {
			t.Fatalf("normalized user still contains removed field %s: %s", removed, persisted)
		}
	}
}

func TestStoreUpsertAndDeleteValidateReferences(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.UpsertObject(ctx, KindDevices, "tun-a", []byte(`{"Enabled":true,"Type":"tun","IfName":"tapx0","MTU":1500}`)); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindRoutes, "route-a", []byte(`{"Enabled":true,"Priority":40,"Action":"allow","DeviceID":"tun-a"}`)); err != nil {
		t.Fatalf("upsert route: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindListeners, "udp-a", []byte(`{"Enabled":true,"BindHost":"127.0.0.1","BindPort":44000,"Transport":"udp","Binding":{"RouteID":"route-a"}}`)); err != nil {
		t.Fatalf("upsert listener: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindXray, "xr-a", []byte(`{"Enabled":true,"Runtime":"embedded","StreamSettingsJSON":"{}"}`)); err != nil {
		t.Fatalf("upsert xray profile: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindSettings, "global", []byte(`{"Enabled":true,"LogLevel":"info","OpenWrtBuildTarget":"x86-64"}`)); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}

	if _, err := store.DeleteObject(ctx, KindDevices, "tun-a"); err == nil {
		t.Fatalf("expected referenced device delete to fail")
	} else if !config.IsValidationError(err) {
		t.Fatalf("expected validation error, got %T %v", err, err)
	}

	item, err := store.GetObject(ctx, KindDevices, "tun-a")
	if err != nil {
		t.Fatalf("device should still exist after rejected delete: %v", err)
	}
	if len(item) == 0 {
		t.Fatalf("empty device payload")
	}

	if _, err := store.GetObject(ctx, KindDevices, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	loaded, err := store.LoadConfig(ctx)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(loaded.Routes) != 1 || loaded.Routes[0].Priority != 40 || loaded.Routes[0].Action != model.RouteActionAllow {
		t.Fatalf("loaded route = %+v, want priority/action preserved", loaded.Routes)
	}
}

func TestStoreRejectsIDMismatch(t *testing.T) {
	store := newTestStore(t)
	_, err := store.UpsertObject(context.Background(), KindDevices, "path-id", []byte(`{"ID":"body-id","Type":"tun","IfName":"tapx0"}`))
	if !errors.Is(err, ErrIDMismatch) {
		t.Fatalf("expected ErrIDMismatch, got %v", err)
	}
}

func TestStorePersistsPrunesAndClearsLogs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for seq, action := range []string{"one", "two", "three"} {
		event := LogEvent{Seq: uint64(seq + 1), Time: "2026-07-13T00:00:00Z", Level: "info", Action: action, Message: action}
		if err := store.AppendLog(ctx, event, 2); err != nil {
			t.Fatalf("append log %s: %v", action, err)
		}
	}
	events, err := store.LoadLogs(ctx, 10)
	if err != nil {
		t.Fatalf("load logs: %v", err)
	}
	if got := []string{events[0].Action, events[1].Action}; !reflect.DeepEqual(got, []string{"two", "three"}) {
		t.Fatalf("log actions = %v, want latest two", got)
	}
	if err := store.ClearLogs(ctx); err != nil {
		t.Fatalf("clear logs: %v", err)
	}
	events, err = store.LoadLogs(ctx, 10)
	if err != nil {
		t.Fatalf("load cleared logs: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("cleared logs = %+v, want none", events)
	}
}

func TestStoreMigratesAndRoundTripsRichDashboardMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE tapx_metrics (
		sampled_at BIGINT NOT NULL PRIMARY KEY, cpu DOUBLE PRECISION NOT NULL,
		memory DOUBLE PRECISION NOT NULL, embedded_xray BIGINT NOT NULL,
		external_xray BIGINT NOT NULL, tapx BIGINT NOT NULL, rx BIGINT NOT NULL,
		tx BIGINT NOT NULL, drops BIGINT NOT NULL)`)
	if err != nil {
		t.Fatalf("create legacy metric table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sample := DashboardMetricSample{
		At: 1000, CPU: 12.5, Memory: 25, Swap: 5, DiskUsage: 40,
		EmbeddedXray: 2, ExternalXray: 1, TapX: 3,
		RX: 100, TX: 200, RXPackets: 10, TXPackets: 20,
		DiskRead: 300, DiskWrite: 400, TCPConnections: 5, UDPConnections: 6,
		Online: 7, Load1: 0.5, Load5: 0.25, Load15: 0.1, Drops: 8,
		TapXHeap: 500, TapXSys: 600, TapXObjects: 700, TapXGC: 9,
		TapXGCPause: 10, TapXObservatory: 3, EmbeddedHeap: 500,
		EmbeddedSys: 600, EmbeddedObjects: 700, EmbeddedGC: 9,
		EmbeddedGCPause: 10, EmbeddedObservatory: 2, ExternalObservatory: 1,
	}
	if err := store.AppendMetric(context.Background(), sample, 0, 10); err != nil {
		t.Fatalf("append rich metric: %v", err)
	}
	loaded, err := store.LoadMetrics(context.Background(), 10)
	if err != nil {
		t.Fatalf("load rich metric: %v", err)
	}
	if len(loaded) != 1 || !reflect.DeepEqual(loaded[0], sample) {
		t.Fatalf("loaded metrics = %+v, want %+v", loaded, sample)
	}
}

func TestReplaceConfigAndIntegrationsPreservesLogSequence(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	logs := []LogEvent{{Seq: 42, Time: "2026-07-16T00:00:00Z", Level: "info", Action: "restore", Message: "preserved"}}
	if err := store.ReplaceConfigAndIntegrations(ctx, config.RuntimeConfig{}, nil, logs, nil); err != nil {
		t.Fatalf("replace database state: %v", err)
	}
	restored, err := store.LoadLogs(ctx, 10)
	if err != nil {
		t.Fatalf("load restored logs: %v", err)
	}
	if len(restored) != 1 || restored[0].Seq != 42 {
		t.Fatalf("restored logs = %+v, want sequence 42", restored)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "tapx.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return store
}

func sampleConfig() config.RuntimeConfig {
	return config.RuntimeConfig{
		Devices: []model.Device{
			{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0", MTU: 1500, LinkAutoOptimize: true},
		},
		VKeys: []model.VKey{
			{ID: "vk-a", Enabled: true, Value: "vk-secret"},
		},
		Addresses: []model.AddressLimit{
			{ID: "addr-a", Enabled: true, DeviceID: "tun-a", IPv4CIDRs: []string{"10.10.0.2/32"}},
		},
		XrayProfiles: []model.XrayProfile{
			{ID: "xr-a", Enabled: true, Runtime: model.XrayEmbedded, Network: "tcp", StreamSettingsJSON: "{}"},
		},
		Settings: []model.Settings{
			{ID: "global", Enabled: true, LogLevel: "info", OpenWrtBuildTarget: "x86-64"},
		},
		Routes: []model.Route{
			{ID: "route-a", Enabled: true, Priority: 10, Action: model.RouteActionBindDevice, DeviceID: "tun-a", VKeyID: "vk-a", AddressID: "addr-a"},
		},
		Listeners: []model.Listener{
			{
				ID:        "udp-a",
				Enabled:   true,
				BindHost:  "127.0.0.1",
				BindPort:  44000,
				Transport: model.TransportUDP,
				RawUDP:    model.RawUDPSettings{PeerMode: model.UDPPeerAny},
				Binding:   model.Binding{RouteID: "route-a"},
			},
		},
	}
}
