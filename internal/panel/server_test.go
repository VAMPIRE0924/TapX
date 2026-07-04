package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	counts := dashboard["objectCounts"].(map[string]any)
	if counts["devices"].(float64) != 1 || counts["settings"].(float64) != 1 {
		t.Fatalf("dashboard object counts = %+v, want device/settings", counts)
	}
	if logs := dashboard["recentLogs"].([]any); len(logs) == 0 {
		t.Fatalf("dashboard missing recent logs: %+v", dashboard)
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
	if !strings.HasPrefix(share["qrPng"].(string), "data:image/png;base64,") {
		t.Fatalf("share qr = %+v", share["qrPng"])
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
	getRaw(t, server.URL+"/app.js", http.StatusOK)
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
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	putJSON(t, server.URL+"/api/config", mustJSON(t, sampleConfig()), http.StatusOK)

	backup := getJSON(t, server.URL+"/api/backup", http.StatusOK)
	if backup["product"] != "TapX" {
		t.Fatalf("backup product = %v, want TapX", backup["product"])
	}
	if _, ok := backup["config"].(map[string]any); !ok {
		t.Fatalf("backup missing config: %+v", backup)
	}

	putJSON(t, server.URL+"/api/config", []byte(`{}`), http.StatusOK)
	restoreBody := mustJSON(t, backup)
	restored := postJSON(t, server.URL+"/api/backup/restore", restoreBody, http.StatusOK)
	cfg := restored["config"].(map[string]any)
	if devices := cfg["Devices"].([]any); len(devices) != 1 {
		t.Fatalf("restored devices = %+v, want one", devices)
	}

	logs := getJSON(t, server.URL+"/api/logs", http.StatusOK)
	events := logs["events"].([]any)
	if len(events) < 3 {
		t.Fatalf("expected backup/config events, got %+v", events)
	}
	deleteJSON(t, server.URL+"/api/logs", http.StatusOK)
	cleared := getJSON(t, server.URL+"/api/logs", http.StatusOK)
	if events := cleared["events"].([]any); len(events) != 0 {
		t.Fatalf("expected cleared logs, got %+v", events)
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
	if !bytes.Contains(index, []byte("<title>TapX</title>")) {
		t.Fatalf("index page missing TapX title")
	}
	app := getRaw(t, server.URL+"/app.js", http.StatusOK)
	if !bytes.Contains(app, []byte("runtime/apply")) {
		t.Fatalf("app script missing runtime apply call")
	}
	if !bytes.Contains(app, []byte("RawUDP.ReceiveBuffer")) || !bytes.Contains(app, []byte("RawTCP.FastOpen")) {
		t.Fatalf("app script missing advanced raw field editor definitions")
	}
	if !bytes.Contains(app, []byte("RawTCP.TLS.CertFile")) || !bytes.Contains(app, []byte("RawUDP.DTLS.CertFile")) || !bytes.Contains(app, []byte("RawUDP.DTLS.ReplayWindow")) {
		t.Fatalf("app script missing raw TLS/DTLS field editor definitions")
	}
	if !bytes.Contains(app, []byte("XrayProfileID")) || !bytes.Contains(app, []byte("OpenWrtBuildTarget")) {
		t.Fatalf("app script missing xray/settings field editor definitions")
	}
	if !bytes.Contains(app, []byte("PanelAuthEnabled")) || !bytes.Contains(app, []byte("/api/auth/login")) {
		t.Fatalf("app script missing panel auth UI/API markers")
	}
	if !bytes.Contains(app, []byte("/api/backup")) || !bytes.Contains(app, []byte("/api/logs")) {
		t.Fatalf("app script missing backup/log API calls")
	}
	if !bytes.Contains(app, []byte("/api/diagnostics")) {
		t.Fatalf("app script missing diagnostics API call")
	}
	if !bytes.Contains(app, []byte("/api/stats")) || !bytes.Contains(app, []byte("Client Quota")) {
		t.Fatalf("app script missing stats API/UI markers")
	}
	if !bytes.Contains(app, []byte("/api/dashboard")) || !bytes.Contains(app, []byte("Recent Logs")) || !bytes.Contains(app, []byte("RX Rate")) {
		t.Fatalf("app script missing dashboard API/UI markers")
	}
	if !bytes.Contains(app, []byte("/api/templates/raw-pair")) || !bytes.Contains(app, []byte("template-generate")) {
		t.Fatalf("app script missing raw template UI/API markers")
	}
	if !bytes.Contains(app, []byte("/api/share/clients")) || !bytes.Contains(app, []byte("Client Share")) {
		t.Fatalf("app script missing client share UI/API markers")
	}
	if !bytes.Contains(app, []byte("/api/clients/")) || !bytes.Contains(app, []byte("reset-traffic")) || !bytes.Contains(app, []byte("TrafficResetAt")) {
		t.Fatalf("app script missing client traffic reset UI/API markers")
	}
	if !bytes.Contains(app, []byte("xrayRuntimes")) || !bytes.Contains(app, []byte("Xray Runtimes")) {
		t.Fatalf("app script missing xray runtime state UI markers")
	}
	if !bytes.Contains(app, []byte("xrayPipes")) || !bytes.Contains(app, []byte("Xray Pipes")) {
		t.Fatalf("app script missing xray pipe state UI markers")
	}
	if !bytes.Contains(app, []byte("/api/xray/external/status")) || !bytes.Contains(app, []byte("Xray Binary")) {
		t.Fatalf("app script missing external xray binary UI/API markers")
	}
	if !bytes.Contains(index, []byte(`id="objectForm"`)) {
		t.Fatalf("index page missing object field form")
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
	if !bytes.Contains(index, []byte(`href="app.css"`)) || !bytes.Contains(index, []byte(`src="app.js"`)) {
		t.Fatalf("index should use relative assets under base path")
	}
	app := getRaw(t, server.URL+"/tapx-secret/app.js", http.StatusOK)
	if !bytes.Contains(app, []byte("detectBasePath")) || !bytes.Contains(app, []byte("apiURL")) {
		t.Fatalf("app script missing base-path API helpers")
	}
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
