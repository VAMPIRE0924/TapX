package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"tapx/internal/buildinfo"
	"tapx/internal/config"
)

const (
	updatePanel        = "panel"
	updateTapX         = "tapx"
	updateEmbeddedXray = "embedded-xray"
	updateExternalXray = "external-xray"
)

type updateVersion struct {
	Version     string `json:"version"`
	Current     bool   `json:"current"`
	Latest      bool   `json:"latest"`
	Installable bool   `json:"installable"`
}

type updateCatalog struct {
	Component       string            `json:"component"`
	CurrentVersion  string            `json:"currentVersion"`
	Channel         string            `json:"channel"`
	Source          string            `json:"source"`
	Delivery        string            `json:"delivery"`
	Platform        string            `json:"platform"`
	Versions        []updateVersion   `json:"versions"`
	RelatedVersions map[string]string `json:"relatedVersions,omitempty"`
	InstallReady    bool              `json:"installReady"`
	Message         string            `json:"message,omitempty"`
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

type componentUpdateRequest struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

func (s *Server) handleUpdateCatalog(w http.ResponseWriter, r *http.Request) {
	component := strings.TrimSpace(r.PathValue("component"))
	catalog, repo, err := s.baseUpdateCatalog(r, component)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if r.URL.Query().Get("channel") == "development" {
		catalog.Channel = "development"
	}
	versions, fetchErr := s.fetchReleaseVersions(r.Context(), repo, catalog.Channel == "development")
	if fetchErr != nil {
		catalog.Message = fetchErr.Error()
	}
	installable := catalog.InstallReady
	if isTapXBundleComponent(component) {
		installable = bundleUpdateSupported() && s.restart != nil
	}
	catalog.InstallReady = installable
	catalog.Versions = buildUpdateVersions(catalog.CurrentVersion, versions, installable)
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) baseUpdateCatalog(r *http.Request, component string) (updateCatalog, string, error) {
	catalog := updateCatalog{
		Component: component,
		Channel:   "stable",
		Platform:  runtime.GOOS + "-" + runtime.GOARCH,
	}
	switch component {
	case updatePanel:
		catalog.CurrentVersion = buildinfo.Version
		catalog.Source = "VAMPIRE0924/TapX"
		catalog.Delivery = "tapx-release-bundle"
		catalog.Message = "TapX-UI is upgraded from the compatible TapX release bundle"
		return catalog, catalog.Source, nil
	case updateTapX:
		catalog.CurrentVersion = buildinfo.Version
		catalog.Source = "VAMPIRE0924/TapX"
		catalog.Delivery = "tapx-release-bundle"
		catalog.RelatedVersions = map[string]string{"embeddedXray": normalizeVersion(buildinfo.XrayVersion())}
		catalog.Message = "TapX and the embedded Xray runtime use one compatible core bundle"
		return catalog, catalog.Source, nil
	case updateEmbeddedXray:
		catalog.CurrentVersion = buildinfo.Version
		catalog.Source = "VAMPIRE0924/TapX"
		catalog.Delivery = "tapx-release-bundle"
		catalog.RelatedVersions = map[string]string{"embeddedXray": normalizeVersion(buildinfo.XrayVersion())}
		catalog.Message = "Embedded Xray follows the unmodified official module version bundled with TapX Core"
		return catalog, catalog.Source, nil
	case updateExternalXray:
		path, err := s.resolveExternalXrayPath(r, r.URL.Query().Get("path"))
		if err != nil {
			catalog.Message = err.Error()
		} else {
			catalog.InstallReady = true
			catalog.CurrentVersion = inspectXrayBinary(path).Version
		}
		catalog.Source = "XTLS/Xray-core"
		catalog.Delivery = "official-xray-release"
		return catalog, catalog.Source, nil
	default:
		return updateCatalog{}, "", fmt.Errorf("unsupported update component %q", component)
	}
}

func (s *Server) fetchReleaseVersions(ctx context.Context, repo string, includePrerelease bool) ([]string, error) {
	client, err := s.panelHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	url := "https://api.github.com/repos/" + repo + "/releases?per_page=12"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "TapX-UI/"+buildinfo.Version)
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query release versions: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("query release versions returned %s", response.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode release versions: %w", err)
	}
	versions := make([]string, 0, len(releases))
	for _, release := range releases {
		if release.Draft || (release.Prerelease && !includePrerelease) {
			continue
		}
		if version := normalizeVersion(release.TagName); version != "" {
			versions = append(versions, version)
		}
	}
	return versions, nil
}

func buildUpdateVersions(current string, releases []string, installable bool) []updateVersion {
	current = normalizeVersion(current)
	latest := ""
	if len(releases) > 0 {
		latest = normalizeVersion(releases[0])
	}
	seen := make(map[string]bool, len(releases)+1)
	versions := make([]string, 0, len(releases)+1)
	for _, version := range append([]string{current}, releases...) {
		version = normalizeVersion(version)
		if version == "" || seen[version] {
			continue
		}
		seen[version] = true
		versions = append(versions, version)
	}
	result := make([]updateVersion, 0, len(versions))
	for _, version := range versions {
		result = append(result, updateVersion{
			Version:     version,
			Current:     version == current,
			Latest:      version == latest,
			Installable: installable && version != current,
		})
	}
	return result
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if version == "" || version == "dev" || version == "unknown" {
		return version
	}
	for _, char := range version {
		if (char < '0' || char > '9') && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && !strings.ContainsRune(".-_+", char) {
			return ""
		}
	}
	return version
}

func (s *Server) handleComponentUpdate(w http.ResponseWriter, r *http.Request) {
	component := strings.TrimSpace(r.PathValue("component"))
	var request componentUpdateRequest
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	version := normalizeVersion(request.Version)
	if version == "" || version == "dev" || version == "unknown" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("a released version is required"))
		return
	}
	if isTapXBundleComponent(component) {
		if !bundleUpdateSupported() {
			writeErrorStatus(w, http.StatusNotImplemented, fmt.Errorf("automatic TapX bundle updates are unavailable on %s-%s", runtime.GOOS, runtime.GOARCH))
			return
		}
		if s.restart == nil {
			writeErrorStatus(w, http.StatusNotImplemented, fmt.Errorf("automatic TapX bundle updates require a panel service supervisor"))
			return
		}
		client, err := s.panelHTTPClient(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		if err := installTapXReleaseBundle(r.Context(), client, version); err != nil {
			writeError(w, err)
			return
		}
		s.log("info", "tapx.bundle.update", fmt.Sprintf("installed TapX compatible release bundle %s", version))
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "restarting": true, "version": version})
		time.AfterFunc(250*time.Millisecond, func() { _ = s.restart() })
		return
	}
	if component != updateExternalXray {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("unsupported update component %q", component))
		return
	}
	asset := officialXrayAsset(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("official Xray does not provide an automatic asset for %s-%s", runtime.GOOS, runtime.GOARCH))
		return
	}
	path, err := s.resolveExternalXrayPath(r, request.Path)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	download := xrayBinaryDownloadRequest{
		URL:               fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/v%s/%s", version, asset),
		Path:              path,
		TimeoutSecond:     180,
		RetryCount:        2,
		OverwriteStrategy: "backup",
	}
	client, err := s.panelHTTPClient(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	payload, contentType, err := downloadXrayPayload(r.Context(), client, download)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	status, err := writeDownloadedXrayBinary(path, download.URL, contentType, bytes.NewReader(payload), download.OverwriteStrategy)
	if err != nil {
		writeError(w, err)
		return
	}
	runtimeApplied := false
	runtimeWarning := ""
	if cfg, loadErr := s.store.LoadConfig(r.Context()); loadErr != nil {
		runtimeWarning = loadErr.Error()
	} else if generated, generateErr := config.GenerateRuntime(cfg); generateErr != nil {
		runtimeWarning = generateErr.Error()
	} else if _, applyErr := s.runtime.Apply(generated, cfg); applyErr != nil {
		runtimeWarning = applyErr.Error()
	} else {
		runtimeApplied = true
	}
	s.log("info", "xray.binary.update", fmt.Sprintf("updated external xray to %s at %s", version, status.Path))
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"binary":         status,
		"runtimeApplied": runtimeApplied,
		"runtimeWarning": runtimeWarning,
	})
}

func isTapXBundleComponent(component string) bool {
	return component == updatePanel || component == updateTapX || component == updateEmbeddedXray
}

func officialXrayAsset(goos, goarch string) string {
	assets := map[string]string{
		"linux-amd64":   "Xray-linux-64.zip",
		"linux-386":     "Xray-linux-32.zip",
		"linux-arm64":   "Xray-linux-arm64-v8a.zip",
		"linux-arm":     "Xray-linux-arm32-v7a.zip",
		"windows-amd64": "Xray-windows-64.zip",
		"windows-386":   "Xray-windows-32.zip",
		"windows-arm64": "Xray-windows-arm64-v8a.zip",
		"darwin-amd64":  "Xray-macos-64.zip",
		"darwin-arm64":  "Xray-macos-arm64-v8a.zip",
	}
	return assets[goos+"-"+goarch]
}
