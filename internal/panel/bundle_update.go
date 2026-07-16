package panel

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxTapXBundleSize = 256 << 20

type tapXUpdateManifest struct {
	SchemaVersion  int    `json:"schemaVersion"`
	ReleaseVersion string `json:"releaseVersion"`
	Compatibility  struct {
		Panel        string `json:"panel"`
		TapXCore     string `json:"tapxCore"`
		EmbeddedXray string `json:"embeddedXray"`
	} `json:"compatibility"`
	Platforms map[string]struct {
		Asset  string `json:"asset"`
		SHA256 string `json:"sha256"`
	} `json:"platforms"`
}

func bundleUpdateSupported() bool {
	return runtime.GOOS == "linux" && runtime.GOARCH == "amd64"
}

func installTapXReleaseBundle(ctx context.Context, client *http.Client, version string) error {
	if !bundleUpdateSupported() {
		return fmt.Errorf("TapX release-bundle updates are unsupported on %s-%s", runtime.GOOS, runtime.GOARCH)
	}
	tag := "v" + normalizeVersion(version)
	baseURL := "https://github.com/VAMPIRE0924/TapX/releases/download/" + tag + "/"
	manifestPayload, err := downloadUpdatePayload(ctx, client, baseURL+"tapx-update-manifest.json", 1<<20)
	if err != nil {
		return fmt.Errorf("download update manifest: %w", err)
	}
	var manifest tapXUpdateManifest
	if err := json.Unmarshal(manifestPayload, &manifest); err != nil {
		return fmt.Errorf("decode update manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 || normalizeVersion(manifest.ReleaseVersion) != normalizeVersion(version) {
		return fmt.Errorf("update manifest does not match release %s", version)
	}
	if normalizeVersion(manifest.Compatibility.Panel) != normalizeVersion(version) ||
		normalizeVersion(manifest.Compatibility.TapXCore) != normalizeVersion(version) {
		return fmt.Errorf("update manifest contains incompatible panel or TapX Core versions")
	}
	platform, ok := manifest.Platforms["linux-amd64"]
	if !ok || platform.Asset != "tapx-linux-amd64.tar.gz" {
		return fmt.Errorf("update manifest does not contain the Linux amd64 TapX bundle")
	}
	bundle, err := downloadUpdatePayload(ctx, client, baseURL+platform.Asset, maxTapXBundleSize)
	if err != nil {
		return fmt.Errorf("download TapX release bundle: %w", err)
	}
	if !strings.EqualFold(fmt.Sprintf("%x", sha256.Sum256(bundle)), strings.TrimSpace(platform.SHA256)) {
		return fmt.Errorf("TapX release bundle SHA256 mismatch")
	}
	binaries, err := extractTapXBinaries(bundle)
	if err != nil {
		return err
	}
	panelPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current panel executable: %w", err)
	}
	panelPath, err = filepath.EvalSymlinks(panelPath)
	if err != nil {
		return fmt.Errorf("resolve panel executable symlink: %w", err)
	}
	corePath := filepath.Join(filepath.Dir(panelPath), "tapx-core")
	return installTapXBinaries(panelPath, corePath, binaries)
}

func downloadUpdatePayload(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/octet-stream, application/json")
	request.Header.Set("User-Agent", "TapX-UI updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("download returned %s", response.Status)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || int64(len(payload)) > limit {
		return nil, fmt.Errorf("download payload is empty or exceeds %d bytes", limit)
	}
	return payload, nil
}

func extractTapXBinaries(bundle []byte) (map[string][]byte, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return nil, fmt.Errorf("open TapX release bundle: %w", err)
	}
	defer gzipReader.Close()
	archive := tar.NewReader(gzipReader)
	result := make(map[string][]byte, 2)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read TapX release bundle: %w", err)
		}
		name := filepath.ToSlash(header.Name)
		base := filepath.Base(name)
		if header.Typeflag != tar.TypeReg || (base != "tapx-panel" && base != "tapx-core") {
			continue
		}
		if !strings.HasPrefix(name, "tapx-linux-amd64/") || header.Size <= 0 || header.Size > maxTapXBundleSize {
			return nil, fmt.Errorf("invalid TapX binary entry %q", header.Name)
		}
		payload, err := io.ReadAll(io.LimitReader(archive, header.Size+1))
		if err != nil || int64(len(payload)) != header.Size {
			return nil, fmt.Errorf("read TapX binary %q", header.Name)
		}
		result[base] = payload
	}
	if len(result["tapx-panel"]) == 0 || len(result["tapx-core"]) == 0 {
		return nil, fmt.Errorf("TapX release bundle is missing tapx-panel or tapx-core")
	}
	return result, nil
}

type preparedBinary struct {
	path   string
	temp   string
	backup string
}

func installTapXBinaries(panelPath, corePath string, binaries map[string][]byte) error {
	items := []preparedBinary{
		{path: corePath, backup: corePath + ".bak"},
		{path: panelPath, backup: panelPath + ".bak"},
	}
	for index := range items {
		name := "tapx-core"
		if index == 1 {
			name = "tapx-panel"
		}
		temp, err := os.CreateTemp(filepath.Dir(items[index].path), "."+name+"-update-*")
		if err != nil {
			cleanupPreparedBinaries(items)
			return fmt.Errorf("prepare %s update: %w", name, err)
		}
		items[index].temp = temp.Name()
		if _, err := temp.Write(binaries[name]); err != nil {
			_ = temp.Close()
			cleanupPreparedBinaries(items)
			return fmt.Errorf("write %s update: %w", name, err)
		}
		if err := temp.Chmod(0o755); err != nil {
			_ = temp.Close()
			cleanupPreparedBinaries(items)
			return err
		}
		if err := temp.Close(); err != nil {
			cleanupPreparedBinaries(items)
			return err
		}
		if err := validateTapXBinary(items[index].temp, name); err != nil {
			cleanupPreparedBinaries(items)
			return err
		}
	}

	replaced := make([]preparedBinary, 0, len(items))
	for _, item := range items {
		_ = os.Remove(item.backup)
		if _, err := os.Stat(item.path); err == nil {
			if err := os.Rename(item.path, item.backup); err != nil {
				rollbackPreparedBinaries(replaced)
				cleanupPreparedBinaries(items)
				return fmt.Errorf("backup %s: %w", item.path, err)
			}
		} else if !os.IsNotExist(err) {
			rollbackPreparedBinaries(replaced)
			cleanupPreparedBinaries(items)
			return err
		}
		if err := os.Rename(item.temp, item.path); err != nil {
			_ = os.Rename(item.backup, item.path)
			rollbackPreparedBinaries(replaced)
			cleanupPreparedBinaries(items)
			return fmt.Errorf("activate %s: %w", item.path, err)
		}
		replaced = append(replaced, item)
	}
	return nil
}

func validateTapXBinary(path, name string) error {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s update is not a valid Go executable: %w", name, err)
	}
	wantPackage := "tapx/cmd/" + name
	if info.Path != wantPackage || info.Main.Path != "tapx" {
		return fmt.Errorf("%s update identifies as %q from module %q", name, info.Path, info.Main.Path)
	}
	return nil
}

func rollbackPreparedBinaries(items []preparedBinary) {
	for index := len(items) - 1; index >= 0; index-- {
		_ = os.Remove(items[index].path)
		_ = os.Rename(items[index].backup, items[index].path)
	}
}

func cleanupPreparedBinaries(items []preparedBinary) {
	for _, item := range items {
		if item.temp != "" {
			_ = os.Remove(item.temp)
		}
	}
}
