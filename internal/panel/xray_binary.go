package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxXrayBinarySize = 128 << 20

var xrayBinaryHTTPClient = &http.Client{Timeout: 120 * time.Second}

type xrayBinaryDownloadRequest struct {
	URL  string
	Path string
}

type xrayBinaryStatus struct {
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`
	IsRegular  bool   `json:"isRegular"`
	Executable bool   `json:"executable"`
	Size       int64  `json:"size"`
	Mode       string `json:"mode,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
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

	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, req.URL, nil)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	response, err := xrayBinaryHTTPClient.Do(request)
	if err != nil {
		writeError(w, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		writeErrorStatus(w, http.StatusBadGateway, fmt.Errorf("download returned %s", response.Status))
		return
	}

	status, err := writeXrayBinary(path, response.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "xray.binary.download", fmt.Sprintf("downloaded external xray binary to %s", status.Path))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "binary": status})
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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

	limited := &io.LimitedReader{R: reader, N: maxXrayBinarySize + 1}
	written, err := io.Copy(temp, limited)
	if err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("write xray binary: %w", err)
	}
	if written == 0 {
		return xrayBinaryStatus{}, fmt.Errorf("xray binary is empty")
	}
	if written > maxXrayBinarySize {
		return xrayBinaryStatus{}, fmt.Errorf("xray binary exceeds %d bytes", maxXrayBinarySize)
	}
	if err := temp.Chmod(0o755); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("chmod temporary xray binary: %w", err)
	}
	if err := temp.Close(); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("close temporary xray binary: %w", err)
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return xrayBinaryStatus{}, fmt.Errorf("install xray binary: %w", err)
	}
	cleanup = false
	_ = os.Chmod(path, 0o755)
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
	return status
}
