package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	if settings.PanelBasePath != "/tapx-test/" {
		t.Fatalf("panel base path = %q, want /tapx-test/", settings.PanelBasePath)
	}
	if !panel.VerifyPanelPassword(settings.AdminPasswordHash, "secret") {
		t.Fatalf("stored password hash does not verify")
	}
}

func TestRunInitAdminConfiguresPanelCertificate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tapx.db")
	certPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(certPath, []byte("certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{
		"-db", dbPath,
		"-listen", "0.0.0.0:18443",
		"-base-path", "/tapx/",
		"-init-admin",
		"-admin-username", "admin",
		"-admin-password", "secret",
		"-panel-cert-file", certPath,
		"-panel-key-file", keyPath,
	}); err != nil {
		t.Fatalf("run init-admin with certificate: %v", err)
	}

	store, err := panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	settings := cfg.Settings[0]
	if !settings.PanelHTTPS || settings.PanelCertFile != certPath || settings.PanelKeyFile != keyPath {
		t.Fatalf("panel certificate settings = %+v", settings)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store before endpoint update: %v", err)
	}
	if err := run([]string{
		"-db", dbPath,
		"-listen", "0.0.0.0:18080",
		"-base-path", "/tapx-http/",
		"-set-panel-endpoint",
		"-disable-panel-https",
	}); err != nil {
		t.Fatalf("disable panel HTTPS: %v", err)
	}
	store, err = panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	settings = mustLoadSettings(t, store)
	if settings.PanelHTTPS || settings.PanelCertFile != "" || settings.PanelKeyFile != "" {
		t.Fatalf("panel HTTPS was not disabled: %+v", settings)
	}
}

func TestRunInitAdminRejectsIncompletePanelCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "fullchain.pem")
	if err := os.WriteFile(certPath, []byte("certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run([]string{
		"-db", filepath.Join(dir, "tapx.db"),
		"-init-admin",
		"-admin-username", "admin",
		"-admin-password", "secret",
		"-panel-cert-file", certPath,
	})
	if err == nil {
		t.Fatal("expected incomplete panel certificate to fail")
	}
}

func mustLoadSettings(t *testing.T, store *panel.Store) model.Settings {
	t.Helper()
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if len(cfg.Settings) == 0 {
		t.Fatal("settings are empty")
	}
	return cfg.Settings[0]
}

func TestRunInitAdminAcceptsPasswordHashAndEndpointUpdatePreservesAuth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tapx.db")
	hash, err := panel.HashPanelPassword("secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := run([]string{
		"-db", dbPath,
		"-listen", ":19090",
		"-base-path", "/tapx-openwrt/",
		"-init-admin",
		"-admin-username", "router-admin",
		"-admin-password-hash", hash,
	}); err != nil {
		t.Fatalf("init admin with hash: %v", err)
	}
	if err := run([]string{
		"-db", dbPath,
		"-listen", ":2053",
		"-base-path", "/tapx-new/",
		"-set-panel-endpoint",
	}); err != nil {
		t.Fatalf("update endpoint: %v", err)
	}

	store, err := panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	settings := cfg.Settings[0]
	if settings.PanelListen != ":2053" || settings.PanelBasePath != "/tapx-new/" {
		t.Fatalf("endpoint = %s %s", settings.PanelListen, settings.PanelBasePath)
	}
	if settings.AdminUsername != "router-admin" || settings.AdminPasswordHash != hash {
		t.Fatalf("endpoint update changed credentials")
	}
}

func TestRunUsesDatabaseEnvironmentDefaults(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "environment.db")
	t.Setenv("TAPX_DB_DRIVER", "sqlite")
	t.Setenv("TAPX_DB_SOURCE", dbPath)
	if err := run([]string{"-check"}); err != nil {
		t.Fatalf("run with database environment: %v", err)
	}
	store, err := panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("environment database was not created: %v", err)
	}
	_ = store.Close()
}

func TestRunExportsAndRestoresPortableBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tapx.db")
	backupPath := filepath.Join(dir, "backup", "tapx.db")
	if err := run([]string{
		"-db", dbPath,
		"-listen", ":2053",
		"-base-path", "/tapx/",
		"-init-admin",
		"-admin-username", "admin",
		"-admin-password", "first-password",
	}); err != nil {
		t.Fatalf("initialize database: %v", err)
	}
	if err := run([]string{"-db", dbPath, "-export-backup", backupPath}); err != nil {
		t.Fatalf("export backup: %v", err)
	}
	if err := run([]string{
		"-db", dbPath,
		"-listen", ":2053",
		"-base-path", "/tapx/",
		"-init-admin",
		"-admin-username", "changed",
		"-admin-password", "second-password",
	}); err != nil {
		t.Fatalf("change database: %v", err)
	}
	if err := run([]string{"-db", dbPath, "-restore-backup", backupPath}); err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	store, err := panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("load restored database: %v", err)
	}
	if cfg.Settings[0].AdminUsername != "admin" || !panel.VerifyPanelPassword(cfg.Settings[0].AdminPasswordHash, "first-password") {
		t.Fatalf("backup did not restore original credentials")
	}
}

func TestRunExportsRuntimeConfigFromDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tapx.db")
	runtimePath := filepath.Join(dir, "runtime", "runtime.json")
	store, err := panel.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	want := config.RuntimeConfig{Settings: []model.Settings{{
		ID:                  "global",
		Enabled:             true,
		Name:                "OpenWrt",
		LogLevel:            "info",
		OpenWrtBuildTarget:  "x86-64",
		StatsIntervalSecond: 5,
	}}}
	if err := store.ReplaceConfig(context.Background(), want); err != nil {
		_ = store.Close()
		t.Fatalf("replace config: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if err := run([]string{"-db", dbPath, "-export-runtime-config", runtimePath}); err != nil {
		t.Fatalf("export runtime config: %v", err)
	}
	data, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	var got config.RuntimeConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse runtime config: %v", err)
	}
	if len(got.Settings) != 1 || got.Settings[0].Name != "OpenWrt" {
		t.Fatalf("runtime config = %+v", got)
	}
}

func TestLoadPanelServerSettingsUsesSettingsHTTPS(t *testing.T) {
	store := newPanelTestStore(t)
	cfg := config.RuntimeConfig{Settings: []model.Settings{{
		ID:                 "global",
		Enabled:            true,
		PanelListen:        "127.0.0.1:18443",
		PanelDomain:        "panel.example.com",
		PanelBasePath:      "/tapx/",
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
	if settings.Listen != "127.0.0.1:18443" || settings.Domain != "panel.example.com" || settings.BasePath != "/tapx/" || !settings.HTTPS || settings.Scheme() != "https" {
		t.Fatalf("settings = %+v, want HTTPS settings listen", settings)
	}
}

func TestRequirePanelHost(t *testing.T) {
	handler := requirePanelHost(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), "panel.example.com")

	allowed := httptest.NewRecorder()
	allowedRequest := httptest.NewRequest(http.MethodGet, "http://panel.example.com/", nil)
	allowedRequest.Host = "PANEL.EXAMPLE.COM:2053"
	handler.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("allowed status = %d, want %d", allowed.Code, http.StatusNoContent)
	}

	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	handler.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusMisdirectedRequest {
		t.Fatalf("denied status = %d, want %d", denied.Code, http.StatusMisdirectedRequest)
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

func TestStoreRejectsIncompleteHTTPS(t *testing.T) {
	store := newPanelTestStore(t)
	cfg := config.RuntimeConfig{Settings: []model.Settings{{
		ID:                 "global",
		Enabled:            true,
		PanelHTTPS:         true,
		LogLevel:           "info",
		OpenWrtBuildTarget: "x86-64",
	}}}
	if err := store.ReplaceConfig(context.Background(), cfg); err == nil {
		t.Fatalf("expected incomplete HTTPS settings to be rejected")
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
