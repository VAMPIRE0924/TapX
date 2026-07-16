package panel

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxXrayBinarySize = 128 << 20

type xrayBinaryDownloadRequest struct {
	URL               string `json:"url"`
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	TimeoutSecond     int    `json:"timeoutSecond"`
	RetryCount        int    `json:"retryCount"`
	OverwriteStrategy string `json:"overwriteStrategy"`
}

type xrayBinaryStatus struct {
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	IsRegular  bool   `json:"isRegular"`
	Executable bool   `json:"executable"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
	Version    string `json:"version,omitempty"`
	Error      string `json:"error,omitempty"`
}

func (s *Server) handleXrayExternalStatus(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolveExternalXrayPath(r, r.URL.Query().Get("path"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"binary": inspectXrayBinary(path)})
}

func (s *Server) handleXrayExternalUpload(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolveExternalXrayPath(r, r.URL.Query().Get("path"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}

	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if strings.EqualFold(contentType, "multipart/form-data") {
		s.handleXrayExternalMultipartUpload(w, r, path)
		return
	}

	defer r.Body.Close()
	status, err := writeXrayBinary(path, r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "xray.binary.upload", fmt.Sprintf("uploaded external xray binary to %s", status.Path))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "binary": status})
}

func (s *Server) handleXrayExternalMultipartUpload(w http.ResponseWriter, r *http.Request, path string) {
	reader, err := r.MultipartReader()
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("multipart field file is required"))
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		if part.FormName() != "file" {
			_ = part.Close()
			continue
		}
		status, err := writeXrayBinary(path, part)
		_ = part.Close()
		if err != nil {
			writeError(w, err)
			return
		}
		s.log("info", "xray.binary.upload", fmt.Sprintf("uploaded external xray binary to %s", status.Path))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "binary": status})
		return
	}
}

func (s *Server) handleXrayExternalDownload(w http.ResponseWriter, r *http.Request) {
	raw, err := readBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req xrayBinaryDownloadRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, err)
		return
	}
	if err := validateDownloadURL(req.URL); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	path, err := s.resolveExternalXrayPath(r, req.Path)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}

	if err := validateXrayDownloadOptions(&req); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if req.OverwriteStrategy == "skip" {
		if status := inspectXrayBinary(path); status.Exists {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": true, "binary": status})
			return
		}
	}
	client, err := s.panelHTTPClient(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	payload, contentType, err := downloadXrayPayload(r.Context(), client, req)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	if err := verifyXrayChecksum(payload, req.SHA256); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	status, err := writeDownloadedXrayBinary(path, req.URL, contentType, bytes.NewReader(payload), req.OverwriteStrategy)
	if err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "xray.binary.download", fmt.Sprintf("downloaded external xray binary to %s", status.Path))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "binary": status})
}

func validateXrayDownloadOptions(req *xrayBinaryDownloadRequest) error {
	if req.TimeoutSecond == 0 {
		req.TimeoutSecond = 120
	}
	if req.TimeoutSecond < 1 || req.TimeoutSecond > 3600 {
		return fmt.Errorf("download timeout must be between 1 and 3600 seconds")
	}
	if req.RetryCount < 0 || req.RetryCount > 10 {
		return fmt.Errorf("download retry count must be between 0 and 10")
	}
	if req.OverwriteStrategy == "" {
		req.OverwriteStrategy = "backup"
	}
	switch req.OverwriteStrategy {
	case "backup", "overwrite", "skip":
	default:
		return fmt.Errorf("overwrite strategy must be backup, overwrite, or skip")
	}
	checksum := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(req.SHA256)), "sha256:")
	if checksum != "" {
		if len(checksum) != sha256.Size*2 {
			return fmt.Errorf("SHA256 must contain 64 hexadecimal characters")
		}
		for _, char := range checksum {
			if !strings.ContainsRune("0123456789abcdef", char) {
				return fmt.Errorf("SHA256 must contain only hexadecimal characters")
			}
		}
	}
	req.SHA256 = checksum
	return nil
}

func downloadXrayPayload(ctx context.Context, client *http.Client, req xrayBinaryDownloadRequest) ([]byte, string, error) {
	var lastErr error
	for attempt := 0; attempt <= req.RetryCount; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(req.TimeoutSecond)*time.Second)
		request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, req.URL, nil)
		if err != nil {
			cancel()
			return nil, "", err
		}
		response, err := client.Do(request)
		if err == nil && response.StatusCode >= 200 && response.StatusCode <= 299 {
			payload, readErr := readXrayBinaryPayload(response.Body)
			contentType := response.Header.Get("Content-Type")
			_ = response.Body.Close()
			cancel()
			if readErr == nil {
				return payload, contentType, nil
			}
			lastErr = readErr
		} else {
			if response != nil {
				lastErr = fmt.Errorf("download returned %s", response.Status)
				_ = response.Body.Close()
			} else {
				lastErr = err
			}
			cancel()
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
	}
	return nil, "", fmt.Errorf("download external xray: %w", lastErr)
}

func verifyXrayChecksum(payload []byte, expected string) error {
	if expected == "" {
		return nil
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(payload))
	if actual != expected {
		return fmt.Errorf("SHA256 mismatch: got %s", actual)
	}
	return nil
}

func (s *Server) resolveExternalXrayPath(r *http.Request, explicit string) (string, error) {
	path := strings.TrimSpace(explicit)
	if path == "" {
		cfg, err := s.store.LoadConfig(r.Context())
		if err != nil {
			return "", err
		}
		for _, item := range cfg.Settings {
			if !item.Enabled {
				continue
			}
			if candidate := strings.TrimSpace(item.ExternalXrayPath); candidate != "" {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		return "", fmt.Errorf("external xray path is required")
	}
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("external xray path must not contain NUL")
	}
	return filepath.Clean(path), nil
}

func validateDownloadURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("download URL must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("download URL host is required")
	}
	return nil
}

func writeXrayBinary(path string, reader io.Reader) (xrayBinaryStatus, error) {
	payload, err := readXrayBinaryPayload(reader)
	if err != nil {
		return xrayBinaryStatus{}, err
	}
	return writeXrayBinaryBytes(path, payload)
}

func writeDownloadedXrayBinary(path string, sourceURL string, contentType string, reader io.Reader, strategy string) (xrayBinaryStatus, error) {
	payload, err := readXrayBinaryPayload(reader)
	if err != nil {
		return xrayBinaryStatus{}, err
	}
	if looksLikeZip(sourceURL, contentType, payload) {
		payload, err = extractXrayFromZip(payload)
		if err != nil {
			return xrayBinaryStatus{}, err
		}
	}
	return writeXrayBinaryBytesWithStrategy(path, payload, strategy)
}

func readXrayBinaryPayload(reader io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: maxXrayBinarySize + 1}
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read xray binary: %w", err)
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("xray binary is empty")
	}
	if len(payload) > maxXrayBinarySize {
		return nil, fmt.Errorf("xray binary exceeds %d bytes", maxXrayBinarySize)
	}
	return payload, nil
}

func looksLikeZip(sourceURL string, contentType string, payload []byte) bool {
	contentType = strings.ToLower(contentType)
	sourceURL = strings.ToLower(sourceURL)
	return strings.Contains(contentType, "zip") ||
		strings.HasSuffix(sourceURL, ".zip") ||
		bytes.HasPrefix(payload, []byte("PK\x03\x04"))
}

func extractXrayFromZip(payload []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("read xray zip: %w", err)
	}
	for _, file := range reader.File {
		name := strings.ToLower(filepath.Base(file.Name))
		if file.FileInfo().IsDir() || (name != "xray" && name != "xray.exe") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open xray in zip: %w", err)
		}
		defer rc.Close()
		return readXrayBinaryPayload(rc)
	}
	return nil, fmt.Errorf("xray executable not found in zip")
}

func writeXrayBinaryBytes(path string, payload []byte) (xrayBinaryStatus, error) {
	return writeXrayBinaryBytesWithStrategy(path, payload, "overwrite")
}

func writeXrayBinaryBytesWithStrategy(path string, payload []byte, strategy string) (xrayBinaryStatus, error) {
	if strategy == "skip" {
		if status := inspectXrayBinary(path); status.Exists {
			return status, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("create xray binary directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tapx-xray-*")
	if err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("create temporary xray binary: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	written, err := temp.Write(payload)
	if err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("write xray binary: %w", err)
	}
	if written != len(payload) {
		return xrayBinaryStatus{}, fmt.Errorf("write xray binary: short write")
	}
	if err := temp.Chmod(0o755); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("chmod temporary xray binary: %w", err)
	}
	if err := temp.Close(); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("close temporary xray binary: %w", err)
	}
	backupPath := path + ".bak"
	backedUp := false
	if strategy == "backup" {
		if _, err := os.Stat(path); err == nil {
			_ = os.Remove(backupPath)
			if err := os.Rename(path, backupPath); err != nil {
				return xrayBinaryStatus{}, fmt.Errorf("backup existing xray binary: %w", err)
			}
			backedUp = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return xrayBinaryStatus{}, fmt.Errorf("inspect existing xray binary: %w", err)
		}
	}
	if runtime.GOOS == "windows" && !backedUp {
		_ = os.Remove(path)
	}
	if err := os.Rename(tempPath, path); err != nil {
		if backedUp {
			_ = os.Rename(backupPath, path)
		}
		return xrayBinaryStatus{}, fmt.Errorf("install xray binary: %w", err)
	}
	cleanup = false
	if err := os.Chmod(path, 0o755); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("chmod installed xray binary: %w", err)
	}
	return inspectXrayBinary(path), nil
}

func inspectXrayBinary(path string) xrayBinaryStatus {
	status := xrayBinaryStatus{Path: filepath.Clean(path)}
	info, err := os.Stat(status.Path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			status.Error = err.Error()
		}
		return status
	}
	status.Exists = true
	status.IsRegular = info.Mode().IsRegular()
	status.Executable = status.IsRegular && (runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0)
	status.Size = info.Size()
	status.Mode = info.Mode().String()
	status.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	if status.Executable {
		status.Version = inspectXrayVersion(status.Path)
	}
	return status
}

func inspectXrayVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(output))
	if len(fields) < 2 || !strings.EqualFold(fields[0], "xray") {
		return ""
	}
	return strings.TrimPrefix(fields[1], "v")
}
