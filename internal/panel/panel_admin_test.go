package panel

import (
	"bytes"
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminCredentialsCreateAndRotate(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/api/panel/credentials", []byte(`{
		"NewUsername":"admin",
		"NewPassword":"initial-secret"
	}`), http.StatusOK)
	cfg, err := store.LoadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Settings) != 1 || !cfg.Settings[0].PanelAuthEnabled || cfg.Settings[0].AdminUsername != "admin" {
		t.Fatalf("credentials were not stored: %+v", cfg.Settings)
	}
	if !VerifyPanelPassword(cfg.Settings[0].AdminPasswordHash, "initial-secret") {
		t.Fatal("stored password hash does not verify")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	postJSONWithClient(t, client, server.URL+"/api/auth/login", []byte(`{
		"Username":"admin",
		"Password":"initial-secret"
	}`), http.StatusOK)
	postJSONWithClient(t, client, server.URL+"/api/panel/credentials", []byte(`{
		"OldUsername":"admin",
		"OldPassword":"wrong",
		"NewUsername":"operator",
		"NewPassword":"next-secret"
	}`), http.StatusUnauthorized)
	postJSONWithClient(t, client, server.URL+"/api/panel/credentials", []byte(`{
		"OldUsername":"admin",
		"OldPassword":"initial-secret",
		"NewUsername":"operator",
		"NewPassword":"next-secret"
	}`), http.StatusOK)

	cfg, err = store.LoadConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Settings[0].AdminUsername != "operator" || !VerifyPanelPassword(cfg.Settings[0].AdminPasswordHash, "next-secret") {
		t.Fatalf("credentials were not rotated: %+v", cfg.Settings[0])
	}
}

func TestPanelRestartRequestsSupervisorRestart(t *testing.T) {
	store := newTestStore(t)
	restarted := make(chan struct{}, 1)
	server := httptest.NewServer(NewServerWithOptions(store, ServerOptions{
		Restart: func() error {
			restarted <- struct{}{}
			return nil
		},
	}).Handler())
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/api/panel/restart", nil, http.StatusAccepted)
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart callback was not invoked")
	}
}

func postJSONWithClient(t *testing.T, client *http.Client, url string, body []byte, wantStatus int) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return decodeResponse(t, resp, wantStatus)
}
