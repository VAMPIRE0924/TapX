package main

import (
	"context"
	"path/filepath"
	"testing"

	"tapx/internal/config"
	"tapx/internal/model"
	"tapx/internal/panel"
)

func TestRunInitAdminWritesPanelAuthSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tapx.db")
	if err := run([]string{
		"-db", dbPath,
		"-listen", "127.0.0.1:19090",
		"-base-path", "/tapx-test",
		"-init-admin",
		"-admin-username", "root",
		"-admin-password", "secret",
	}); err != nil {
		t.Fatalf("run init-admin: %v", err)
	}

	store, err := panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Settings) != 1 {
		t.Fatalf("settings = %+v, want one", cfg.Settings)
	}
	settings := cfg.Settings[0]
	if !settings.Enabled || !settings.PanelAuthEnabled || settings.AdminUsername != "root" || settings.PanelListen != "127.0.0.1:19090" {
		t.Fatalf("settings = %+v, want enabled panel auth", settings)
	}
	if !panel.VerifyPanelPassword(settings.AdminPasswordHash, "secret") {
		t.Fatalf("stored password hash does not verify")
	}
}

func TestLoadPanelServerSettingsUsesSettingsHTTPS(t *testing.T) {
	store := newPanelTestStore(t)
	cfg := config.RuntimeConfig{Settings: []model.Settings{{
		ID:                 "global",
		Enabled:            true,
		PanelListen:        "127.0.0.1:18443",
		PanelHTTPS:         true,
		PanelCertFile:      "/etc/tapx/panel.crt",
		PanelKeyFile:       "/etc/tapx/panel.key",
		LogLevel:           "info",
		OpenWrtBuildTarget: "x86-64",
	}}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}

	settings, err := loadPanelServerSettings(context.Background(), store, "127.0.0.1:8080", false)
	if err != nil {
		t.Fatalf("load panel server settings: %v", err)
	}
	if settings.Listen != "127.0.0.1:18443" || !settings.HTTPS || settings.Scheme() != "https" {
		t.Fatalf("settings = %+v, want HTTPS settings listen", settings)
	}
}

func TestLoadPanelServerSettingsListenFlagOverridesSettings(t *testing.T) {
	store := newPanelTestStore(t)
	cfg := config.RuntimeConfig{Settings: []model.Settings{{
		ID:                 "global",
		Enabled:            true,
		PanelListen:        "127.0.0.1:18443",
		LogLevel:           "info",
		OpenWrtBuildTarget: "x86-64",
	}}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}

	settings, err := loadPanelServerSettings(context.Background(), store, "127.0.0.1:18080", true)
	if err != nil {
		t.Fatalf("load panel server settings: %v", err)
	}
	if settings.Listen != "127.0.0.1:18080" || settings.Scheme() != "http" {
		t.Fatalf("settings = %+v, want flag listen", settings)
	}
}

func TestLoadPanelServerSettingsRejectsIncompleteHTTPS(t *testing.T) {
	store := newPanelTestStore(t)
	cfg := config.RuntimeConfig{Settings: []model.Settings{{
		ID:                 "global",
		Enabled:            true,
		PanelHTTPS:         true,
		LogLevel:           "info",
		OpenWrtBuildTarget: "x86-64",
	}}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}

	if _, err := loadPanelServerSettings(context.Background(), store, "127.0.0.1:8080", false); err == nil {
		t.Fatalf("expected incomplete HTTPS settings to fail")
	}
}

func newPanelTestStore(t *testing.T) *panel.Store {
	t.Helper()
	store, err := panel.OpenStore(filepath.Join(t.TempDir(), "tapx.db"))
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
