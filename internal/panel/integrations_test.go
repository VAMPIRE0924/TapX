package panel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIntegrationStoreRoundTrip(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	want := nordIntegrationState{PrivateKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}
	if err := store.SetIntegration(context.Background(), nordIntegrationName, want); err != nil {
		t.Fatal(err)
	}
	raw, err := store.GetIntegration(context.Background(), nordIntegrationName)
	if err != nil {
		t.Fatal(err)
	}
	var got nordIntegrationState
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.PrivateKey != want.PrivateKey {
		t.Fatalf("private key = %q, want %q", got.PrivateKey, want.PrivateKey)
	}
	if err := store.DeleteIntegration(context.Background(), nordIntegrationName); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetIntegration(context.Background(), nordIntegrationName); err != ErrNotFound {
		t.Fatalf("get deleted integration error = %v, want %v", err, ErrNotFound)
	}
}

func TestNordPrivateKeyAPI(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := NewServer(store)
	handler := server.Handler()
	privateKey := base64.StdEncoding.EncodeToString(make([]byte, 32))

	body, _ := json.Marshal(map[string]string{"privateKey": privateKey})
	req := httptest.NewRequest(http.MethodPost, "/api/integrations/nord/private-key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("set private key status = %d, body = %s", res.Code, res.Body.String())
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/integrations/nord/data", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("get data status = %d, body = %s", res.Code, res.Body.String())
	}
	var payload struct {
		OK   bool                 `json:"ok"`
		Data nordIntegrationState `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Data.PrivateKey != privateKey {
		t.Fatalf("unexpected data response: %+v", payload)
	}

	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodDelete, "/api/integrations/nord/delete", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", res.Code, res.Body.String())
	}
}

func TestGenerateWireguardKeypair(t *testing.T) {
	privateKey, publicKey, err := generateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"private": privateKey, "public": publicKey} {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			t.Fatalf("%s key is not 32-byte base64: %q, %v", name, value, err)
		}
	}
	if privateKey == publicKey {
		t.Fatal("private and public keys must differ")
	}
}

func TestBuildWarpOutboundSettings(t *testing.T) {
	state := &warpIntegrationState{
		PrivateKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		ClientID:   base64.StdEncoding.EncodeToString([]byte{1, 2, 255}),
	}
	deviceConfig := map[string]any{
		"config": map[string]any{
			"interface": map[string]any{"addresses": map[string]any{"v4": "172.16.0.2", "v6": "2606:4700:110::2"}},
			"peers": []any{map[string]any{
				"public_key": base64.StdEncoding.EncodeToString(make([]byte, 32)),
				"endpoint":   map[string]any{"host": "engage.cloudflareclient.com:2408"},
			}},
		},
	}
	raw, endpoint, err := buildWarpOutboundSettings(state, deviceConfig)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "engage.cloudflareclient.com:2408" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	var settings struct {
		Address  []string `json:"address"`
		Reserved []int    `json:"reserved"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Address) != 2 || len(settings.Reserved) != 3 || settings.Reserved[2] != 255 {
		t.Fatalf("unexpected WARP settings: %+v", settings)
	}
}

func TestSplitHostPortValidatesWarpEndpoint(t *testing.T) {
	host, port, err := splitHostPort("[2001:db8::1]:2408")
	if err != nil {
		t.Fatal(err)
	}
	if host != "2001:db8::1" || port != 2408 {
		t.Fatalf("split endpoint = %q:%d", host, port)
	}
	for _, endpoint := range []string{
		"missing-port",
		"example.com:0",
		"example.com:65536",
		"example.com:not-a-port",
	} {
		if _, _, err := splitHostPort(endpoint); err == nil {
			t.Fatalf("splitHostPort(%q) accepted an invalid endpoint", endpoint)
		}
	}
}
