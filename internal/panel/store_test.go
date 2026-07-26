package panel

import (
	"context"
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

func TestStoreRoundTripsAdvancedDeviceNetworkConfig(t *testing.T) {
	store := newTestStore(t)
	cfg := config.RuntimeConfig{Devices: []model.Device{{
		ID: "tap-shared", Enabled: true, Type: model.DeviceTAP, IfName: "tapx0",
		TapMode: model.TapModeSharedIP, AccessRole: model.AccessRoleServer,
		DHCP: &model.DHCPConfig{Mode: model.DHCPModeMirror},
		SharedIP: &model.SharedIPConfig{
			Role: model.SharedIPRoleService, UplinkInterface: "eth0", AddressSource: "auto",
			FirewallBackend: model.FirewallNFTables, ReservedTCPPorts: []string{"22", "443"},
		},
	}}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Devices) != 1 || loaded.Devices[0].SharedIP == nil {
		t.Fatalf("loaded devices = %+v", loaded.Devices)
	}
	device := loaded.Devices[0]
	if device.TapMode != model.TapModeSharedIP || device.SharedIP.UplinkInterface != "eth0" || len(device.SharedIP.ReservedTCPPorts) != 2 {
		t.Fatalf("advanced device config did not round trip: %+v", device)
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
		UUID:     "11111111-1111-4111-8111-111111111111",
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

func TestStoreRoundTripsCurrentWebContract(t *testing.T) {
	store := newTestStore(t)
	want := currentWebContractConfig()
	if err := store.ReplaceConfig(context.Background(), want); err != nil {
		t.Fatalf("replace current Web contract: %v", err)
	}
	got, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load current Web contract: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("current Web contract changed during SQLite round trip:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestStoreRejectsRemovedUserCredentialFields(t *testing.T) {
	store := newTestStore(t)
	raw := []byte(`{
		"ID":"legacy-user","Enabled":true,"Name":"legacy",
		"Security":"auto","Flow":"xtls-rprx-vision","ReverseTag":"reverse",
		"WireguardPrivateKey":"private","WireguardPublicKey":"public",
		"WireguardPreSharedKey":"psk","WireguardAllowedIPs":["10.0.0.2/32"]
	}`)
	if _, err := store.UpsertObject(context.Background(), KindClients, "legacy-user", raw); err == nil || !strings.Contains(err.Error(), `unknown field "Security"`) {
		t.Fatalf("upsert removed user fields error = %v, want unknown Security field", err)
	}
}

func TestStoreUpsertAndDeleteValidateReferences(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.UpsertObject(ctx, KindDevices, "tun-a", []byte(`{"Enabled":true,"Type":"tun","IfName":"tapx0","MTU":1500}`)); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindVKeys, "vkey-a", []byte(`{"Enabled":true,"Value":"test-vkey"}`)); err != nil {
		t.Fatalf("upsert vKey: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindRoutes, "route-a", []byte(`{"Enabled":true,"Priority":40,"Action":"allow","DeviceID":"tun-a","VKeyID":"vkey-a"}`)); err != nil {
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

func TestStoreRoundTripsRichDashboardMetrics(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "tapx.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
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
				RawUDP:    model.RawUDPSettings{},
				Binding:   model.Binding{RouteID: "route-a"},
			},
		},
	}
}

func currentWebContractConfig() config.RuntimeConfig {
	return config.RuntimeConfig{
		Devices: []model.Device{
			{
				ID: "tap-dhcp", Enabled: true, Name: "TAP DHCP", Type: model.DeviceTAP, IfName: "tapx-tap0",
				MTU: 1500, MSSClamp: 1452, TapMode: model.TapModeStandalone, AccessRole: model.AccessRoleServer,
				Bridge: &model.BridgeConfig{Enabled: true, Name: "br-tapx", IfName: "eth1", MTU: 1500},
				DHCP: &model.DHCPConfig{
					Mode: model.DHCPModeServer, IPv4CIDR: "10.20.0.1/24", PoolStart: "10.20.0.20", PoolEnd: "10.20.0.200",
					PrefixLength: 24, Gateway: "10.20.0.1", DNS: []string{"1.1.1.1", "8.8.8.8"}, LeaseSeconds: 3600,
					Authoritative: true, ConflictDetection: true,
					StaticLeases: []model.DHCPStaticLease{{Name: "camera", MAC: "02:00:00:00:00:20", Address: "10.20.0.20"}},
				},
				Routes: []model.DeviceRoute{{Enabled: true, Destination: "10.21.0.0/16", Gateway: "10.20.0.254", Source: "10.20.0.1", IfName: "tapx-tap0", Metric: 20, Table: "main"}},
				Source: "manual", Remark: "TAP server contract",
			},
			{
				ID: "tap-shared", Enabled: true, Name: "TAP shared address", Type: model.DeviceTAP, IfName: "tapx-tap1",
				MTU: 9000, LinkAutoOptimize: true, TapMode: model.TapModeSharedIP, AccessRole: model.AccessRoleServer,
				DHCP: &model.DHCPConfig{Mode: model.DHCPModeMirror},
				SharedIP: &model.SharedIPConfig{
					Role: model.SharedIPRoleService, UplinkInterface: "eth0", AddressSource: "manual", IPv4CIDR: "192.0.2.10/24",
					Gateway: "192.0.2.1", DNS: []string{"9.9.9.9"}, FirewallBackend: model.FirewallNFTables,
					HostPortPriority: true, TrackAddressChanges: true, ReservedTCPPorts: []string{"22", "443", "2000-2010"},
					ReservedUDPPorts: []string{"53", "3000-3010"}, ClientMAC: "02:00:00:00:00:30",
				},
				Source: "manual", Remark: "shared IP contract",
			},
			{
				ID: "tun-client", Enabled: true, Name: "TUN client", Type: model.DeviceTUN, IfName: "tapx-tun0", MTU: 1500,
				LinkAutoOptimize: true, AccessRole: model.AccessRoleClient,
				TUNDHCP: &model.TUNDHCPConfig{Mode: model.TUNDHCPModeClient, Protocol: "dual", LeaseSeconds: 7200},
				Source:  "connector-auto", Remark: "control-channel lease client",
			},
			{
				ID: "tun-server", Enabled: true, Name: "TUN server", Type: model.DeviceTUN, IfName: "tapx-tun1", MTU: 1500,
				AccessRole: model.AccessRoleServer,
				TUNDHCP: &model.TUNDHCPConfig{
					Mode: model.TUNDHCPModeServer, Protocol: "dual", RelayEnabled: true, RelayProtocol: "dual",
					IPv4CIDR: "10.30.0.1/24", IPv6CIDR: "fd30::1/64", PoolStart: "10.30.0.20", PoolEnd: "10.30.0.200",
					IPv6PoolStart: "fd30::20", IPv6PoolEnd: "fd30::ffff", Gateway: "10.30.0.1", DNS: []string{"1.1.1.1"},
					OfferedGateway: "10.30.0.1", OfferedDNS: []string{"1.1.1.1", "2606:4700:4700::1111"}, LeaseSeconds: 7200,
					Authoritative: true, ConflictDetection: true, RelayDownstreamInterfaces: []string{"br-lan"},
					RelayServers: []string{"10.0.0.1", "fd00::1"}, MaxHops: 8,
				},
				Source: "listener-auto", Remark: "control-channel lease server",
			},
		},
		Listeners: []model.Listener{
			{
				ID: "listener-tcp", Enabled: true, Name: "Raw TCP TLS", BindHost: "0.0.0.0", BindPort: 44000, Transport: model.TransportTCP,
				RawTCP: model.RawTCPSettings{
					LengthMode: model.TCPLength32, NoDelay: true, KeepAliveSecond: 30, FastOpen: true, ConnectTimeout: 5,
					ReconnectSecond: 3, Workers: 1, QueueSize: 4096, ZeroCopy: true, IdleTimeout: 120,
					TLS: model.RawTLSSettings{Enabled: true, CertFile: "/etc/tapx/server.crt", KeyFile: "/etc/tapx/server.key", MinVersion: "1.2", MaxVersion: "1.3"},
				},
				Binding:              model.Binding{VKeyID: "vkey-a", DeviceID: "tap-dhcp", AddressID: "address-a"},
				ShareAddressStrategy: "custom", ShareAddress: "edge.example.com", ExpiresAt: 2_000_000_000,
				TrafficCap: 1 << 40, TrafficReset: "monthly", TrafficResetAt: 2_000_000_100, TrafficResetGeneration: 2,
				TrafficRXOffset: 100, TrafficTXOffset: 200, Remark: "listener contract",
			},
			{
				ID: "listener-udp", Enabled: true, Name: "Raw UDP DTLS", BindHost: "::", BindPort: 44001, Transport: model.TransportUDP,
				RawUDP: model.RawUDPSettings{
					KeepAliveSecond: 15, Workers: 1, QueueSize: 4096, ZeroCopy: true, ConnectTimeout: 5, IdleTimeout: 90,
					DTLS: model.RawDTLSSettings{Enabled: true, CertFile: "/etc/tapx/server.crt", KeyFile: "/etc/tapx/server.key", MinVersion: "1.2", MaxVersion: "1.2", MTU: 1400, ReplayWindow: 128},
				},
				Binding: model.Binding{DeviceID: "tun-server"}, Remark: "UDP contract",
			},
		},
		Connectors: []model.Connector{
			{
				ID: "connector-tcp", Enabled: true, Name: "Raw TCP client", Remote: "edge.example.com", Port: 44000, Transport: model.TransportTCP,
				RawTCP: model.RawTCPSettings{
					LengthMode: model.TCPLength32, NoDelay: true, KeepAliveSecond: 30, FastOpen: true, ConnectTimeout: 5,
					ReconnectSecond: 3, Workers: 1, QueueSize: 4096, ZeroCopy: true, IdleTimeout: 120,
					TLS: model.RawTLSSettings{Enabled: true, ServerName: "edge.example.com", MinVersion: "1.2", MaxVersion: "1.3"},
				},
				Binding:        model.Binding{VKeyID: "vkey-a", DeviceID: "tap-shared", AddressID: "address-a"},
				TrafficResetAt: 2_000_000_200, TrafficResetGeneration: 3, TrafficRXOffset: 300, TrafficTXOffset: 400,
				Remark: "connector contract", CreatedAt: 1_900_000_000, UpdatedAt: 1_900_000_100,
			},
			{
				ID: "connector-udp", Enabled: true, Name: "Raw UDP client", Remote: "2001:db8::10", Port: 44001, Transport: model.TransportUDP,
				RawUDP: model.RawUDPSettings{
					KeepAliveSecond: 15, Workers: 1, QueueSize: 2048, ZeroCopy: true, ConnectTimeout: 5, IdleTimeout: 90,
					DTLS: model.RawDTLSSettings{Enabled: true, ServerName: "edge.example.com", MinVersion: "1.2", MaxVersion: "1.2", MTU: 1400, ReplayWindow: 128},
				},
				Binding: model.Binding{DeviceID: "tun-client"}, Remark: "UDP client contract",
			},
		},
		Clients: []model.Client{{
			ID: "client-a", Enabled: true, Name: "user", Email: "user@example.com", ListenerID: "listener-tcp",
			ListenerIDs: []string{"listener-tcp", "listener-udp"}, UUID: "11111111-1111-4111-8111-111111111111",
			Password: "password", Auth: "hysteria-auth", AllowedDeviceIDs: []string{"tap-dhcp", "tun-server"},
			Binding: model.Binding{VKeyID: "vkey-a", DeviceID: "tap-dhcp", AddressID: "address-a"}, AddressID: "address-a",
			ExpiresAt: 2_000_000_000, TrafficCap: 1 << 39, UploadRateLimit: 100_000_000, DownloadRateLimit: 200_000_000,
			TrafficReset: "monthly", TrafficResetAt: 2_000_000_100, TrafficResetGeneration: 4,
			TrafficRXOffset: 500, TrafficTXOffset: 600, Remark: "user contract", CreatedAt: 1_900_000_000, UpdatedAt: 1_900_000_100,
		}},
		Routes: []model.Route{{
			ID: "route-a", Enabled: true, Priority: 10, Action: model.RouteActionBindDevice, VKeyID: "vkey-a",
			ListenerID: "listener-tcp", DeviceID: "tap-dhcp", ClientID: "client-a", AddressID: "address-a",
		}},
		VKeys: []model.VKey{{ID: "vkey-a", Enabled: true, Name: "primary", Value: "vkey-secret", Remark: "vKey contract"}},
		Addresses: []model.AddressLimit{{
			ID: "address-a", Enabled: true, Name: "user address policy", DeviceID: "tap-dhcp", ClientID: "client-a",
			MACs: []string{"02:00:00:00:00:20"}, IPv4CIDRs: []string{"10.20.0.20/32"}, IPv6CIDRs: []string{"fd20::20/128"},
			IPv4Gateway: "10.20.0.1", IPv6Gateway: "fd20::1", DNS: []string{"1.1.1.1", "2606:4700:4700::1111"},
			Routes: []string{"10.21.0.0/16", "fd21::/64"}, AllowDefaultRoute: true, Remark: "address contract",
		}},
		XrayProfiles: []model.XrayProfile{{
			ID: "xray-a", Enabled: true, Name: "official embedded Xray", Runtime: model.XrayEmbedded,
			InboundProtocol: "vless", InboundSettingsJSON: `{"clients":[]}`, OutboundProtocol: "freedom", OutboundSettingsJSON: `{}`,
			SendThrough: "0.0.0.0", TargetStrategy: "UseIP", Network: "xhttp", Security: "tls",
			StreamSettingsJSON: `{"network":"xhttp","security":"tls"}`, SniffingJSON: `{"enabled":false}`,
			MuxJSON: `{"enabled":false}`, SockoptJSON: `{"tcpFastOpen":true}`, FallbacksJSON: `{"items":[]}`,
			RoutingJSON: `{"rules":[]}`, DNSJSON: `{"servers":["1.1.1.1"]}`, PolicyJSON: `{"levels":{}}`,
			AdvancedJSON: `{"observatory":{}}`, Remark: "Xray contract",
		}},
		Settings: []model.Settings{{
			ID: "settings", Enabled: true, Name: "TapX", PanelName: "Edge TapX", PanelListen: "[::]:2053",
			PanelDomain: "panel.example.com", PanelBasePath: "/tapx/", PanelHTTPS: true,
			PanelCertFile: "/etc/tapx/panel.crt", PanelKeyFile: "/etc/tapx/panel.key", PanelAuthEnabled: false,
			SessionTTLSecond: 86400, Timezone: "Asia/Hong_Kong", PanelOutbound: "direct",
			ExternalXrayPath: "/usr/bin/xray", ExternalXrayConfigFile: "/var/lib/tapx/xray.json",
			ExternalXrayWorkDir: "/var/lib/tapx", ExternalXrayArgs: "run\n-config\n{config}", LogLevel: "info",
			StatsIntervalSecond: 5, BackupDir: "/var/lib/tapx/backups", DataDir: "/var/lib/tapx",
			OpenWrtBuildTarget: "mediatek-filogic", AdvancedJSON: `{"embeddedXrayEnabled":true,"tapxEnabled":true,"pageSize":20,"language":"zh-CN"}`,
			Remark: "settings contract",
		}},
	}
}
