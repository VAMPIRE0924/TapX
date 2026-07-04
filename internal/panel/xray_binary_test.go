package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerExternalXrayStatusUploadAndDownload(t *testing.T) {
	store := newTestStore(t)
	binaryPath := filepath.Join(t.TempDir(), "xray")
	cfg := sampleConfig()
	cfg.Settings[0].ExternalXrayPath = binaryPath
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	status := getJSON(t, server.URL+"/api/xray/external/status", http.StatusOK)["binary"].(map[string]any)
	if status["path"] != filepath.Clean(binaryPath) || status["exists"] != false {
		t.Fatalf("initial xray binary status = %+v", status)
	}

	uploadBody := strings.NewReader("fake-xray-a")
	resp, err := http.Post(server.URL+"/api/xray/external/upload", "application/octet-stream", uploadBody)
	if err != nil {
		t.Fatalf("POST upload: %v", err)
	}
	uploaded := decodeResponse(t, resp, http.StatusOK)["binary"].(map[string]any)
	if uploaded["exists"] != true || uploaded["size"].(float64) != float64(len("fake-xray-a")) {
		t.Fatalf("uploaded xray binary status = %+v", uploaded)
	}
	if payload, err := os.ReadFile(binaryPath); err != nil || string(payload) != "fake-xray-a" {
		t.Fatalf("uploaded file = %q err=%v", string(payload), err)
	}

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake-xray-b"))
	}))
	t.Cleanup(source.Close)
	downloaded := postJSON(t, server.URL+"/api/xray/external/download", []byte(`{"url":"`+source.URL+`"}`), http.StatusOK)["binary"].(map[string]any)
	if downloaded["exists"] != true || downloaded["size"].(float64) != float64(len("fake-xray-b")) {
		t.Fatalf("downloaded xray binary status = %+v", downloaded)
	}
	if payload, err := os.ReadFile(binaryPath); err != nil || string(payload) != "fake-xray-b" {
		t.Fatalf("downloaded file = %q err=%v", string(payload), err)
	}
}

func TestServerExternalXrayRejectsBadDownloadURL(t *testing.T) {
	store := newTestStore(t)
	cfg := sampleConfig()
	cfg.Settings[0].ExternalXrayPath = filepath.Join(t.TempDir(), "xray")
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	postJSON(t, server.URL+"/api/xray/external/download", []byte(`{"url":"file:///tmp/xray"}`), http.StatusBadRequest)
}
