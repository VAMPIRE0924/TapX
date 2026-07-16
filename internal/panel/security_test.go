package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tapx/internal/model"
)

func TestTOTPCodeVerification(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	code := totpCode(secret, now.Unix()/30)
	if len(code) != 6 || !verifyTOTP(secret, code, now) {
		t.Fatalf("generated TOTP code %q did not verify", code)
	}
	if verifyTOTP(secret, "000000", now) && code != "000000" {
		t.Fatal("invalid TOTP code verified")
	}
}

func TestAPITokenAuthenticatesPanelAPI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	created := postJSON(t, server.URL+"/api/security/tokens", []byte(`{"Name":"automation"}`), http.StatusCreated)
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("token was not returned: %+v", created)
	}

	hash, err := HashPanelPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg := sampleConfig()
	cfg.Settings = []model.Settings{{
		ID: "global", Enabled: true, PanelAuthEnabled: true,
		AdminUsername: "admin", AdminPasswordHash: hash, SessionTTLSecond: 60,
		LogLevel: "info", OpenWrtBuildTarget: "x86-64",
	}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/config", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, response, http.StatusOK)

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/api/config", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, response, http.StatusUnauthorized)
}

func TestLoginRequiresTOTPWhenEnabled(t *testing.T) {
	store := newTestStore(t)
	hash, err := HashPanelPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg := sampleConfig()
	cfg.Settings = []model.Settings{{
		ID: "global", Enabled: true, PanelAuthEnabled: true,
		AdminUsername: "admin", AdminPasswordHash: hash, SessionTTLSecond: 60,
		LogLevel: "info", OpenWrtBuildTarget: "x86-64",
	}}
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	secret := "JBSWY3DPEHPK3PXP"
	if err := store.SetIntegration(context.Background(), panelSecurityIntegration, panelSecurityState{TOTPSecret: secret}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/api/auth/login", []byte(`{"Username":"admin","Password":"secret"}`), http.StatusUnauthorized)
	code := totpCode(secret, time.Now().Unix()/30)
	body, _ := json.Marshal(map[string]string{"Username": "admin", "Password": "secret", "TwoFactorCode": code})
	response, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, response, http.StatusOK)

	session := getJSON(t, server.URL+"/api/auth/session", http.StatusOK)
	if session["twoFactorEnabled"] != true {
		t.Fatalf("session did not report enabled 2FA: %+v", session)
	}
}

func TestTOTPEnableAndDisableEndpoints(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	prepared := postJSON(t, server.URL+"/api/security/totp/prepare", nil, http.StatusOK)
	secret, _ := prepared["secret"].(string)
	if secret == "" {
		t.Fatalf("prepare did not return a secret: %+v", prepared)
	}
	code := totpCode(secret, time.Now().Unix()/30)
	body, _ := json.Marshal(map[string]string{"Secret": secret, "Code": code})
	postJSON(t, server.URL+"/api/security/totp/enable", body, http.StatusOK)
	status := getJSON(t, server.URL+"/api/security", http.StatusOK)
	if status["twoFactorEnabled"] != true {
		t.Fatalf("2FA was not enabled: %+v", status)
	}

	code = totpCode(secret, time.Now().Unix()/30)
	body, _ = json.Marshal(map[string]string{"Code": code})
	postJSON(t, server.URL+"/api/security/totp/disable", body, http.StatusOK)
	status = getJSON(t, server.URL+"/api/security", http.StatusOK)
	if status["twoFactorEnabled"] != false {
		t.Fatalf("2FA was not disabled: %+v", status)
	}
}
