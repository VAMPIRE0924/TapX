package panel

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
)

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

func TestStoreUpsertAndDeleteValidateReferences(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.UpsertObject(ctx, KindDevices, "tun-a", []byte(`{"Enabled":true,"Type":"tun","IfName":"tapx0","MTU":1500}`)); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	if _, err := store.UpsertObject(ctx, KindRoutes, "route-a", []byte(`{"Enabled":true,"DeviceID":"tun-a"}`)); err != nil {
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
}

func TestStoreRejectsIDMismatch(t *testing.T) {
	store := newTestStore(t)
	_, err := store.UpsertObject(context.Background(), KindDevices, "path-id", []byte(`{"ID":"body-id","Type":"tun","IfName":"tapx0"}`))
	if !errors.Is(err, ErrIDMismatch) {
		t.Fatalf("expected ErrIDMismatch, got %v", err)
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
			{ID: "tun-a", Enabled: true, Type: model.DeviceTUN, IfName: "tapx0", MTU: 1500},
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
			{ID: "route-a", Enabled: true, DeviceID: "tun-a", VKeyID: "vk-a", AddressID: "addr-a"},
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
