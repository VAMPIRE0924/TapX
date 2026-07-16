package panel

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
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

func TestServerExternalXrayDownloadExtractsOfficialZip(t *testing.T) {
	store := newTestStore(t)
	binaryPath := filepath.Join(t.TempDir(), "xray")
	cfg := sampleConfig()
	cfg.Settings[0].ExternalXrayPath = binaryPath
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range map[string]string{
		"README.md": "not the binary",
		"xray":      "fake-official-xray",
	} {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip member: %v", err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("write zip member: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(archive.Bytes())
	}))
	t.Cleanup(source.Close)

	downloaded := postJSON(t, server.URL+"/api/xray/external/download", []byte(`{"url":"`+source.URL+`/Xray-linux-64.zip"}`), http.StatusOK)["binary"].(map[string]any)
	if downloaded["exists"] != true || downloaded["size"].(float64) != float64(len("fake-official-xray")) {
		t.Fatalf("downloaded xray binary status = %+v", downloaded)
	}
	if payload, err := os.ReadFile(binaryPath); err != nil || string(payload) != "fake-official-xray" {
		t.Fatalf("downloaded zip file = %q err=%v", string(payload), err)
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

func TestServerExternalXrayDownloadRetriesVerifiesChecksumAndBacksUp(t *testing.T) {
	store := newTestStore(t)
	binaryPath := filepath.Join(t.TempDir(), "xray")
	if err := os.WriteFile(binaryPath, []byte("old-xray"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := sampleConfig()
	cfg.Settings[0].ExternalXrayPath = binaryPath
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	payload := []byte("verified-xray")
	attempts := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "retry", http.StatusBadGateway)
			return
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(source.Close)
	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))
	body := fmt.Sprintf(`{"url":%q,"sha256":%q,"retryCount":1,"timeoutSecond":5,"overwriteStrategy":"backup"}`, source.URL, checksum)
	postJSON(t, server.URL+"/api/xray/external/download", []byte(body), http.StatusOK)
	if attempts != 2 {
		t.Fatalf("download attempts = %d, want 2", attempts)
	}
	if got, err := os.ReadFile(binaryPath); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("installed payload = %q err=%v", got, err)
	}
	if got, err := os.ReadFile(binaryPath + ".bak"); err != nil || string(got) != "old-xray" {
		t.Fatalf("backup payload = %q err=%v", got, err)
	}
}

func TestServerExternalXrayDownloadRejectsChecksumWithoutReplacingBinary(t *testing.T) {
	store := newTestStore(t)
	binaryPath := filepath.Join(t.TempDir(), "xray")
	if err := os.WriteFile(binaryPath, []byte("keep-me"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := sampleConfig()
	cfg.Settings[0].ExternalXrayPath = binaryPath
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("wrong-payload"))
	}))
	t.Cleanup(source.Close)

	body := fmt.Sprintf(`{"url":%q,"sha256":"%064x","overwriteStrategy":"overwrite"}`, source.URL, 1)
	postJSON(t, server.URL+"/api/xray/external/download", []byte(body), http.StatusBadRequest)
	if got, err := os.ReadFile(binaryPath); err != nil || string(got) != "keep-me" {
		t.Fatalf("binary changed after checksum failure: %q err=%v", got, err)
	}
}

func TestServerExternalXrayDownloadSkipDoesNotContactSource(t *testing.T) {
	store := newTestStore(t)
	binaryPath := filepath.Join(t.TempDir(), "xray")
	if err := os.WriteFile(binaryPath, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := sampleConfig()
	cfg.Settings[0].ExternalXrayPath = binaryPath
	if err := store.ReplaceConfig(context.Background(), cfg); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)
	contacted := false
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		_, _ = w.Write([]byte("replacement"))
	}))
	t.Cleanup(source.Close)

	body := fmt.Sprintf(`{"url":%q,"overwriteStrategy":"skip"}`, source.URL)
	postJSON(t, server.URL+"/api/xray/external/download", []byte(body), http.StatusOK)
	if contacted {
		t.Fatal("download source was contacted for skip strategy")
	}
}
