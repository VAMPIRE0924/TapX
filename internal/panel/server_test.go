package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"tapx/internal/config"
	"tapx/internal/core"
	"tapx/internal/model"
)

func TestServerConfigRuntimeAndObjectAPI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	putJSON(t, server.URL+"/api/config", mustJSON(t, sampleConfig()), http.StatusOK)

	resp := getJSON(t, server.URL+"/api/runtime", http.StatusOK)
	runtime := resp["runtime"].(map[string]any)
	if pipes := runtime["UDPPipes"].([]any); len(pipes) != 1 {
		t.Fatalf("expected one UDP pipe, got %+v", pipes)
	}
	if profiles := runtime["XrayProfiles"].([]any); len(profiles) != 1 {
		t.Fatalf("expected one Xray profile, got %+v", profiles)
	}

	deviceRaw := []byte(`{"Enabled":true,"Type":"tap","IfName":"tapx1","MTU":1500}`)
	putJSON(t, server.URL+"/api/objects/devices/tap-a", deviceRaw, http.StatusOK)
	devices := getJSON(t, server.URL+"/api/objects/devices", http.StatusOK)
	if items := devices["items"].([]any); len(items) != 2 {
		t.Fatalf("expected two devices, got %+v", items)
	}

	bad := []byte(`{"ID":"other","Enabled":true,"Type":"tun","IfName":"tapx2"}`)
	putJSON(t, server.URL+"/api/objects/devices/path-id", bad, http.StatusBadRequest)
}

func TestServerConfigAPIRoundTripUsesConfigEnvelope(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	cfg := sampleConfig()
	putResp := putJSON(t, server.URL+"/api/config", mustJSON(t, cfg), http.StatusOK)
	if putResp["ok"] != true {
		t.Fatalf("PUT /api/config missing ok=true: %+v", putResp)
	}
	if _, ok := putResp["config"].(map[string]any); !ok {
		t.Fatalf("PUT /api/config missing config envelope: %+v", putResp)
	}
	if _, ok := putResp["obj"]; ok {
		t.Fatalf("PUT /api/config returned legacy obj envelope: %+v", putResp)
	}

	getResp := getJSON(t, server.URL+"/api/config", http.StatusOK)
	body, ok := getResp["config"].(map[string]any)
	if !ok {
		t.Fatalf("GET /api/config missing config envelope: %+v", getResp)
	}
	if _, ok := getResp["obj"]; ok {
		t.Fatalf("GET /api/config returned legacy obj envelope: %+v", getResp)
	}
	devices := body["Devices"].([]any)
	listeners := body["Listeners"].([]any)
	routes := body["Routes"].([]any)
	settings := body["Settings"].([]any)
	if len(devices) != 1 || len(listeners) != 1 || len(routes) != 1 || len(settings) != 1 {
		t.Fatalf("GET /api/config did not round-trip DB objects: %+v", body)
	}
	device := devices[0].(map[string]any)
	listener := listeners[0].(map[string]any)
	if device["ID"] != "tun-a" || listener["ID"] != "udp-a" {
		t.Fatalf("GET /api/config returned wrong objects: device=%+v listener=%+v", device, listener)
	}
}

func TestServerRuntimeApplyStateAndStop(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	apply := postJSON(t, server.URL+"/api/runtime/apply", nil, http.StatusOK)
	if state := apply["state"].(map[string]any); state["running"] != true {
		t.Fatalf("expected running state after apply, got %+v", state)
	}

	current := getJSON(t, server.URL+"/api/runtime/state", http.StatusOK)
	if state := current["state"].(map[string]any); state["running"] != true {
		t.Fatalf("expected running state, got %+v", state)
	}

	stopped := postJSON(t, server.URL+"/api/runtime/stop", nil, http.StatusOK)
	if state := stopped["state"].(map[string]any); state["running"] != false {
		t.Fatalf("expected stopped state, got %+v", state)
	}
}

func TestServerRuntimeComponentActions(t *testing.T) {
	store := newTestStore(t)
	controller := &fakeRuntimeController{}
	manager := newFakeRuntimeManager(controller)
	if _, err := manager.Apply(&config.GeneratedRuntime{}); err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	server := httptest.NewServer(NewServer(store, manager).Handler())
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/api/runtime/components/embedded-xray/restart", nil, http.StatusOK)
	postJSON(t, server.URL+"/api/runtime/components/tapx/stop", nil, http.StatusOK)
	if len(controller.componentRestarts) != 1 || controller.componentRestarts[0] != core.RuntimeComponentEmbeddedXray {
		t.Fatalf("component restarts = %+v", controller.componentRestarts)
	}
	if len(controller.componentStops) != 1 || controller.componentStops[0] != core.RuntimeComponentTapX {
		t.Fatalf("component stops = %+v", controller.componentStops)
	}
	postJSON(t, server.URL+"/api/runtime/components/unknown/restart", nil, http.StatusBadRequest)
	postJSON(t, server.URL+"/api/runtime/components/tapx/unknown", nil, http.StatusBadRequest)
}

func TestServerStatsAPI(t *testing.T) {
	store := newTestStore(t)
	if err := store.ReplaceConfig(context.Background(), sampleConfig()); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	stats := getJSON(t, server.URL+"/api/stats", http.StatusOK)
	if _, ok := stats["totals"].(map[string]any); !ok {
		t.Fatalf("stats missing totals: %+v", stats)
	}
	if _, ok := stats["byDevice"].([]any); !ok {
		t.Fatalf("stats missing byDevice: %+v", stats)
	}
	if _, ok := stats["clients"].([]any); !ok {
		t.Fatalf("stats missing clients: %+v", stats)
	}
}

func TestServerDashboardAPI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	putJSON(t, server.URL+"/api/config", mustJSON(t, sampleConfig()), http.StatusOK)

	dashboard := getJSON(t, server.URL+"/api/dashboard", http.StatusOK)
	if dashboard["generatedAt"] == "" {
		t.Fatalf("dashboard missing generatedAt: %+v", dashboard)
	}
	if _, ok := dashboard["runtime"].(map[string]any); !ok {
		t.Fatalf("dashboard missing runtime: %+v", dashboard)
	}
	if _, ok := dashboard["stats"].(map[string]any); !ok {
		t.Fatalf("dashboard missing stats: %+v", dashboard)
	}
	if _, ok := dashboard["rates"].(map[string]any); !ok {
		t.Fatalf("dashboard missing rates: %+v", dashboard)
	}
	system := dashboard["system"].(map[string]any)
	if system["cpuCores"].(float64) <= 0 {
		t.Fatalf("dashboard system cpuCores = %+v, want positive", system)
	}
	if _, ok := system["runningPipes"].(float64); !ok {
		t.Fatalf("dashboard system missing runningPipes: %+v", system)
	}
	if _, ok := system["dropCount"].(float64); !ok {
		t.Fatalf("dashboard system missing dropCount: %+v", system)
	}
	counts := dashboard["objectCounts"].(map[string]any)
	if counts["devices"].(float64) != 1 || counts["settings"].(float64) != 1 {
		t.Fatalf("dashboard object counts = %+v, want device/settings", counts)
	}
	if logs := dashboard["recentLogs"].([]any); len(logs) == 0 {
		t.Fatalf("dashboard missing recent logs: %+v", dashboard)
	}
	if history, ok := dashboard["history"].([]any); !ok || len(history) != 1 {
		t.Fatalf("dashboard history = %+v, want one persisted sample", dashboard["history"])
	}
}

func TestServerSystemInterfacesAliases(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	for _, path := range []string{"/api/server/interfaces", "/api/system/interfaces", "/panel/api/server/interfaces"} {
		resp := getJSON(t, server.URL+path, http.StatusOK)
		if resp["success"] != true {
			t.Fatalf("%s success = %+v, want true", path, resp["success"])
		}
		if _, ok := resp["obj"].([]any); !ok {
			t.Fatalf("%s obj = %+v, want array", path, resp["obj"])
		}
	}
}

func TestServerRawPairTemplateAPI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	resp := getJSON(t, server.URL+"/api/templates/raw-pair?transport=tcp&hostA=192.0.2.10&hostB=192.0.2.20&port=46001", http.StatusOK)
	template := resp["template"].(map[string]any)
	if template["transport"] != "tcp" {
		t.Fatalf("template transport = %v, want tcp", template["transport"])
	}
	sideA := template["a"].(map[string]any)
	sideB := template["b"].(map[string]any)
	if listeners := sideA["Listeners"].([]any); len(listeners) != 1 {
		t.Fatalf("side A listeners = %+v, want one", listeners)
	}
	if connectors := sideB["Connectors"].([]any); len(connectors) != 1 {
		t.Fatalf("side B connectors = %+v, want one", connectors)
	}

	getJSON(t, server.URL+"/api/templates/raw-pair?transport=udp&hostA=bad&hostB=192.0.2.20", http.StatusBadRequest)
}

func TestServerClientShareAPI(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig()
	cfg.Clients = []model.Client{{
		ID: "client-a", Enabled: true, Name: "Alice", CredentialType: "vkey", CredentialValue: "vk-secret",
		Binding: model.Binding{RouteID: "route-a"}, AddressID: "addr-a",
	}}
	cfg.Routes[0].ClientID = "client-a"
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	resp := getJSON(t, server.URL+"/api/share/clients/client-a", http.StatusOK)
	share := resp["share"].(map[string]any)
	if !strings.HasPrefix(share["link"].(string), "tapx://client/gzip/") {
		t.Fatalf("share link = %+v", share["link"])
	}
	if _, exists := share["qrPng"]; exists {
		t.Fatalf("share response must not contain QR data")
	}
	links := share["links"].([]any)
	if len(links) != 1 || links[0] != share["link"] {
		t.Fatalf("share links = %+v", links)
	}
	payload := share["payload"].(map[string]any)
	if payload["client"].(map[string]any)["id"] != "client-a" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestServerClientTrafficResetAPI(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig()
	cfg.Clients = []model.Client{{ID: "client-a", Enabled: true, Name: "Alice", TrafficCap: 1000}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	resp := postJSON(t, server.URL+"/api/clients/client-a/traffic/reset", nil, http.StatusOK)
	reset := resp["reset"].(map[string]any)
	if reset["clientId"] != "client-a" || reset["resetAt"].(float64) == 0 {
		t.Fatalf("reset response = %+v", reset)
	}
	updated := resp["config"].(map[string]any)
	client := updated["Clients"].([]any)[0].(map[string]any)
	if client["TrafficResetAt"].(float64) == 0 {
		t.Fatalf("client after reset = %+v, want reset metadata", client)
	}
}

func TestServerConnectorTrafficResetAPI(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig()
	cfg.Connectors = []model.Connector{{
		ID: "connector-a", Enabled: true, Transport: model.TransportUDP, Remote: "127.0.0.1", Port: 44001,
	}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	resp := postJSON(t, server.URL+"/api/connectors/connector-a/traffic/reset", nil, http.StatusOK)
	reset := resp["reset"].(map[string]any)
	if reset["endpointId"] != "connector-a" || reset["endpointKind"] != "connector" || reset["resetAt"].(float64) == 0 {
		t.Fatalf("reset response = %+v", reset)
	}
	updated := resp["config"].(map[string]any)
	connector := updated["Connectors"].([]any)[0].(map[string]any)
	if connector["TrafficResetAt"].(float64) == 0 {
		t.Fatalf("connector after reset = %+v, want reset metadata", connector)
	}
}

func TestServerRuntimeApplyErrorIncludesRollbackState(t *testing.T) {
	store := newTestStore(t)
	manager := newFakeRuntimeManager(
		&fakeRuntimeController{},
		&fakeRuntimeController{startErr: errors.New("replacement failed")},
		&fakeRuntimeController{},
	)
	server := httptest.NewServer(NewServer(store, manager).Handler())
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/api/runtime/apply", nil, http.StatusOK)
	resp := postJSON(t, server.URL+"/api/runtime/apply", nil, http.StatusInternalServerError)
	state := resp["state"].(map[string]any)
	if state["running"] != true {
		t.Fatalf("expected rollback running state, got %+v", state)
	}
	if state["lastRollbackAt"] == "" {
		t.Fatalf("expected rollback timestamp, got %+v", state)
	}
	if !strings.Contains(state["lastError"].(string), "rolled back") {
		t.Fatalf("expected rollback note, got %+v", state)
	}
}

func TestServerPanelAuthSessionFlow(t *testing.T) {
	store := newTestStore(t)
	hash, err := HashPanelPassword("secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	cfg := sampleConfig()
	cfg.Settings = []model.Settings{{
		ID:                 "global",
		Enabled:            true,
		PanelAuthEnabled:   true,
		AdminUsername:      "admin",
		AdminPasswordHash:  hash,
		SessionTTLSecond:   60,
		LogLevel:           "info",
		OpenWrtBuildTarget: "x86-64",
	}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config with auth settings: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	getJSON(t, server.URL+"/api/config", http.StatusUnauthorized)
	index := getRaw(t, server.URL+"/", http.StatusOK)
	getRaw(t, server.URL+"/"+firstJSAsset(t, index), http.StatusOK)
	session := getJSON(t, server.URL+"/api/auth/session", http.StatusOK)
	if session["authEnabled"] != true || session["authenticated"] != false {
		t.Fatalf("unexpected unauthenticated session: %+v", session)
	}

	postJSON(t, server.URL+"/api/auth/login", []byte(`{"username":"admin","password":"wrong"}`), http.StatusUnauthorized)

	resp, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"secret"}`)))
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	cookies := resp.Cookies()
	login := decodeResponse(t, resp, http.StatusOK)
	if login["authenticated"] != true || len(cookies) == 0 {
		t.Fatalf("unexpected login response/cookies: %+v cookies=%+v", login, cookies)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/config", nil)
	if err != nil {
		t.Fatalf("new authenticated request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET authenticated config: %v", err)
	}
	decodeResponse(t, resp, http.StatusOK)

	req, err = http.NewRequest(http.MethodPost, server.URL+"/api/auth/logout", nil)
	if err != nil {
		t.Fatalf("new logout request: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST logout: %v", err)
	}
	decodeResponse(t, resp, http.StatusOK)

	req, err = http.NewRequest(http.MethodGet, server.URL+"/api/config", nil)
	if err != nil {
		t.Fatalf("new request after logout: %v", err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET after logout: %v", err)
	}
	decodeResponse(t, resp, http.StatusUnauthorized)
}

func TestServerBackupRestoreAndLogs(t *testing.T) {
	store := newTestStore(t)
	controller := &fakeRuntimeController{}
	manager := newFakeRuntimeManager(controller)
	runtime, err := config.GenerateRuntime(sampleConfig())
	if err != nil {
		t.Fatalf("generate active runtime: %v", err)
	}
	if _, err := manager.Apply(runtime, sampleConfig()); err != nil {
		t.Fatalf("apply active runtime: %v", err)
	}
	panelServer := NewServer(store, manager)
	session, _, err := panelServer.sessions.Create(time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	server := httptest.NewServer(panelServer.Handler())
	t.Cleanup(server.Close)

	putJSON(t, server.URL+"/api/config", mustJSON(t, sampleConfig()), http.StatusOK)
	if err := store.SetIntegration(context.Background(), nordIntegrationName, nordIntegrationState{PrivateKey: "backup-key"}); err != nil {
		t.Fatalf("seed integration: %v", err)
	}
	backupMetric := DashboardMetricSample{
		At: 1000, CPU: 12.5, Memory: 25, Swap: 5, DiskUsage: 40,
		TapX: 2, RX: 100, TX: 200, DiskRead: 300, DiskWrite: 400,
		Load1: 0.5, Load5: 0.25, Load15: 0.1, Drops: 3,
		TapXHeap: 500, TapXSys: 600, TapXObjects: 700,
	}
	if err := store.AppendMetric(context.Background(), backupMetric, 0, defaultMetricLimit); err != nil {
		t.Fatalf("seed metric: %v", err)
	}

	backupResponse, err := http.Get(server.URL + "/api/backup")
	if err != nil {
		t.Fatalf("GET database backup: %v", err)
	}
	if backupResponse.StatusCode != http.StatusOK {
		t.Fatalf("database backup status = %d", backupResponse.StatusCode)
	}
	if got := backupResponse.Header.Get("Content-Type"); got != "application/vnd.sqlite3" {
		t.Fatalf("database backup content type = %q", got)
	}
	backupRaw, err := io.ReadAll(backupResponse.Body)
	_ = backupResponse.Body.Close()
	if err != nil || !bytes.HasPrefix(backupRaw, []byte(sqliteHeader)) {
		t.Fatalf("database backup is not SQLite: len=%d err=%v", len(backupRaw), err)
	}
	backupDB, cleanupBackup, err := openBackupDatabase(context.Background(), backupRaw)
	if err != nil {
		t.Fatalf("open database backup: %v", err)
	}
	defer cleanupBackup()
	backupStore := &Store{db: backupDB}
	backupConfig, err := backupStore.LoadConfig(context.Background())
	if err != nil || len(backupConfig.Devices) != 1 {
		t.Fatalf("backup config devices = %+v, err=%v", backupConfig.Devices, err)
	}
	if _, err := backupStore.GetIntegration(context.Background(), nordIntegrationName); err != nil {
		t.Fatalf("backup integration: %v", err)
	}
	backupLogs, err := backupStore.LoadLogs(context.Background(), defaultLogLimit)
	if err != nil || len(backupLogs) == 0 {
		t.Fatalf("backup logs = %+v, err=%v", backupLogs, err)
	}
	backupMetrics, err := backupStore.LoadMetrics(context.Background(), 10)
	if err != nil || len(backupMetrics) != 1 || !reflect.DeepEqual(backupMetrics[0], backupMetric) {
		t.Fatalf("backup metrics = %+v, err=%v", backupMetrics, err)
	}

	putJSON(t, server.URL+"/api/config", []byte(`{}`), http.StatusOK)
	if err := store.DeleteIntegration(context.Background(), nordIntegrationName); err != nil {
		t.Fatalf("delete integration before restore: %v", err)
	}
	if err := store.AppendMetric(context.Background(), DashboardMetricSample{At: 2000, CPU: 99}, 0, defaultMetricLimit); err != nil {
		t.Fatalf("append post-backup metric: %v", err)
	}
	restoreRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/backup/restore", bytes.NewReader(backupRaw))
	if err != nil {
		t.Fatalf("new restore request: %v", err)
	}
	restoreRequest.Header.Set("Content-Type", "application/vnd.sqlite3")
	restoreResponse, err := http.DefaultClient.Do(restoreRequest)
	if err != nil {
		t.Fatalf("POST database restore: %v", err)
	}
	restored := decodeResponse(t, restoreResponse, http.StatusOK)
	if restored["restartRequired"] != true {
		t.Fatalf("restore restartRequired = %v, want true", restored["restartRequired"])
	}
	if manager.State().Running || controller.stopCalls != 1 {
		t.Fatalf("runtime after restore = %+v stopCalls=%d, want stopped", manager.State(), controller.stopCalls)
	}
	if panelServer.sessions.Valid(session) {
		t.Fatal("session created before restore remained valid")
	}
	cfg := restored["config"].(map[string]any)
	if devices := cfg["Devices"].([]any); len(devices) != 1 {
		t.Fatalf("restored devices = %+v, want one", devices)
	}
	if _, err := store.GetIntegration(context.Background(), nordIntegrationName); err != nil {
		t.Fatalf("restored integration: %v", err)
	}
	metrics, err := store.LoadMetrics(context.Background(), 10)
	if err != nil || len(metrics) != 1 || !reflect.DeepEqual(metrics[0], backupMetric) {
		t.Fatalf("restored metrics = %+v, err=%v", metrics, err)
	}

	logs := getJSON(t, server.URL+"/api/logs", http.StatusOK)
	events := logs["events"].([]any)
	if len(events) != len(backupLogs)+1 {
		t.Fatalf("restored logs = %+v, want %d backed-up events plus restore event", events, len(backupLogs))
	}
	lastEvent := events[len(events)-1].(map[string]any)
	if lastEvent["action"] != "backup.restore" {
		t.Fatalf("last restored log = %+v, want backup.restore", lastEvent)
	}
	restarted := NewServer(store)
	if got := len(restarted.logs.List()); got != len(events) {
		t.Fatalf("logs after panel restart = %d, want %d", got, len(events))
	}
	deleteJSON(t, server.URL+"/api/logs", http.StatusOK)
	cleared := getJSON(t, server.URL+"/api/logs", http.StatusOK)
	if events := cleared["events"].([]any); len(events) != 0 {
		t.Fatalf("expected cleared logs, got %+v", events)
	}
	if persisted, err := store.LoadLogs(context.Background(), defaultLogLimit); err != nil || len(persisted) != 0 {
		t.Fatalf("persisted logs after clear = %+v, err=%v", persisted, err)
	}
}

func TestDatabaseBackupRejectsJSONAndTruncatedSQLite(t *testing.T) {
	store := newTestStore(t)
	for name, raw := range map[string][]byte{
		"JSON":             []byte(`{"product":"TapX","config":{}}`),
		"truncated SQLite": append([]byte(sqliteHeader), make([]byte, 24)...),
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.ValidateDatabaseBackup(context.Background(), raw); err == nil {
				t.Fatal("ValidateDatabaseBackup() error = nil")
			}
		})
	}
}

func TestServerDiagnostics(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	putJSON(t, server.URL+"/api/config", mustJSON(t, sampleConfig()), http.StatusOK)

	diag := getJSON(t, server.URL+"/api/diagnostics", http.StatusOK)
	if diag["product"] != "TapX" {
		t.Fatalf("diagnostic product = %v, want TapX", diag["product"])
	}
	if diag["version"] == "" {
		t.Fatalf("diagnostic version is empty: %+v", diag)
	}
	counts := diag["objectCounts"].(map[string]any)
	if counts["devices"].(float64) != 1 || counts["listeners"].(float64) != 1 || counts["xrayProfiles"].(float64) != 1 || counts["settings"].(float64) != 1 {
		t.Fatalf("diagnostic counts = %+v, want one device/listener/xray/settings", counts)
	}
	openwrt := diag["openwrt"].(map[string]any)
	if openwrt["currentBuildTarget"] != "x86-64" {
		t.Fatalf("openwrt target = %+v, want x86-64", openwrt)
	}
	if _, ok := diag["runtime"].(map[string]any); !ok {
		t.Fatalf("diagnostic missing runtime: %+v", diag)
	}
}

func TestServerStaticUI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	index := getRaw(t, server.URL+"/", http.StatusOK)
	if !bytes.Contains(index, []byte("<title>TapX-UI</title>")) {
		t.Fatalf("index page missing TapX-UI title")
	}
	if !bytes.Contains(index, []byte(`id="root"`)) || !bytes.Contains(index, []byte(`./assets/`)) {
		t.Fatalf("index page missing Vite app/assets")
	}
	if !bytes.Contains(index, []byte(`<base href="/">`)) || !bytes.Contains(index, []byte(`<meta name="tapx-base-path" content="">`)) {
		t.Fatalf("index page missing root runtime path")
	}
	app := getRaw(t, server.URL+"/"+firstJSAsset(t, index), http.StatusOK)
	if !bytes.Contains(app, []byte("menu.dashboard")) || !bytes.Contains(app, []byte("document.getElementById(`root`)")) {
		t.Fatalf("app script missing current panel entry markers")
	}
	dashboardChunk := getRaw(t, server.URL+"/assets/"+firstImportedChunk(t, app, "DashboardPage"), http.StatusOK)
	if !bytes.Contains(dashboardChunk, []byte("app.brand")) || !bytes.Contains(dashboardChunk, []byte("dashboard.management")) || !bytes.Contains(dashboardChunk, []byte("dashboard.realtimeTransport")) || !bytes.Contains(dashboardChunk, []byte("dashboard.policyProtection")) {
		t.Fatalf("dashboard chunk missing approved status card markers")
	}
	login := getRaw(t, server.URL+"/login.html", http.StatusOK)
	if !bytes.Contains(login, []byte(`id="root"`)) || !bytes.Contains(login, []byte(`<meta name="tapx-base-path" content="">`)) {
		t.Fatalf("login page missing current panel/runtime markers")
	}
}

func TestServerBasePathScopesUIAndAPI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServerWithOptions(store, ServerOptions{BasePath: "/tapx-secret"}).Handler())
	t.Cleanup(server.Close)

	getRaw(t, server.URL+"/api/health", http.StatusNotFound)
	health := getJSON(t, server.URL+"/tapx-secret/api/health", http.StatusOK)
	if health["ok"] != true {
		t.Fatalf("health = %+v, want ok", health)
	}
	index := getRaw(t, server.URL+"/tapx-secret/", http.StatusOK)
	if !bytes.Contains(index, []byte(`./assets/`)) || !bytes.Contains(index, []byte(`id="root"`)) {
		t.Fatalf("index should use relative assets under base path")
	}
	if !bytes.Contains(index, []byte(`<base href="/tapx-secret/">`)) || !bytes.Contains(index, []byte(`<meta name="tapx-base-path" content="/tapx-secret">`)) {
		t.Fatalf("index missing injected base path")
	}
	app := getRaw(t, server.URL+"/tapx-secret/"+firstJSAsset(t, index), http.StatusOK)
	if !bytes.Contains(app, []byte("menu.dashboard")) {
		t.Fatalf("base-path app script missing current panel entry")
	}
	devices := getRaw(t, server.URL+"/tapx-secret/devices", http.StatusOK)
	if !bytes.Contains(devices, []byte(`<base href="/tapx-secret/">`)) || !bytes.Contains(devices, []byte(`id="root"`)) {
		t.Fatalf("nested panel route did not fall back to the SPA entry")
	}
	login := getRaw(t, server.URL+"/tapx-secret/login.html", http.StatusOK)
	if !bytes.Contains(login, []byte(`<meta name="tapx-base-path" content="/tapx-secret">`)) {
		t.Fatalf("login page missing injected base path")
	}
}

func TestStaticHandlerEscapesBasePathMetadata(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	staticHandler(`/tapx\"><script>alert(1)</script>`).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("static handler status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.Bytes()
	if bytes.Contains(body, []byte(`<script>alert(1)</script>`)) {
		t.Fatalf("base path was injected as executable markup: %s", body)
	}
	if !bytes.Contains(body, []byte(`&lt;script&gt;alert(1)&lt;/script&gt;`)) {
		t.Fatalf("base path metadata was not HTML escaped: %s", body)
	}
}

func firstJSAsset(t *testing.T, index []byte) string {
	t.Helper()
	match := regexp.MustCompile(`(?:src|href)="\./(assets/[^"]+\.js)"`).FindSubmatch(index)
	if len(match) != 2 {
		t.Fatalf("index missing JS asset: %s", string(index))
	}
	return string(match[1])
}

func firstImportedChunk(t *testing.T, app []byte, prefix string) string {
	t.Helper()
	pattern := fmt.Sprintf(`\./(%s-[^`+"`"+`"']+\.js)`, regexp.QuoteMeta(prefix))
	match := regexp.MustCompile(pattern).FindSubmatch(app)
	if len(match) != 2 {
		t.Fatalf("app missing %s chunk import", prefix)
	}
	return string(match[1])
}

func TestServerValidationProblems(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	badConfig := []byte(`{"Devices":[{"ID":"tun-a","Enabled":true,"Type":"tun","IfName":"tapx0"}],"Listeners":[{"ID":"udp-a","Enabled":true,"BindPort":44000,"Transport":"udp","Binding":{"RouteID":"missing"}}]}`)
	resp := postJSON(t, server.URL+"/api/config/validate?mode=apply", badConfig, http.StatusUnprocessableEntity)
	if _, ok := resp["problems"].([]any); !ok {
		t.Fatalf("expected validation problems, got %+v", resp)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}

func getJSON(t *testing.T, url string, wantStatus int) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return decodeResponse(t, resp, wantStatus)
}

func getRaw(t *testing.T, url string, wantStatus int) []byte {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		var body bytes.Buffer
		_, _ = body.ReadFrom(resp.Body)
		t.Fatalf("status = %d, want %d, body: %s", resp.StatusCode, wantStatus, body.String())
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response: %v", err)
	}
	return body.Bytes()
}

func putJSON(t *testing.T, url string, body []byte, wantStatus int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new PUT: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return decodeResponse(t, resp, wantStatus)
}

func postJSON(t *testing.T, url string, body []byte, wantStatus int) map[string]any {
	t.Helper()
	reader := bytes.NewReader(body)
	if body == nil {
		reader = bytes.NewReader([]byte{})
	}
	resp, err := http.Post(url, "application/json", reader)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return decodeResponse(t, resp, wantStatus)
}

func deleteJSON(t *testing.T, url string, wantStatus int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("new DELETE: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return decodeResponse(t, resp, wantStatus)
}

func decodeResponse(t *testing.T, resp *http.Response, wantStatus int) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		var body bytes.Buffer
		_, _ = body.ReadFrom(resp.Body)
		t.Fatalf("status = %d, want %d, body: %s", resp.StatusCode, wantStatus, body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}
