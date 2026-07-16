package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tapx/internal/config"
)

const (
	managedNodesIntegrationName  = "managed_nodes"
	managedNodeResponseLimit     = 16 << 20
	managedNodeRequestTimeout    = 12 * time.Second
	managedNodeDiagnosticTimeout = 35 * time.Second
)

var managedNodeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

type ManagedNode struct {
	ID                  string                  `json:"ID"`
	Enabled             bool                    `json:"Enabled"`
	Name                string                  `json:"Name"`
	Remark              string                  `json:"Remark,omitempty"`
	Protocol            string                  `json:"Protocol"`
	Host                string                  `json:"Host"`
	Port                uint16                  `json:"Port"`
	BasePath            string                  `json:"BasePath"`
	AllowPrivateAddress bool                    `json:"AllowPrivateAddress"`
	TLSVerify           string                  `json:"TLSVerify"`
	CertificateSHA256   string                  `json:"CertificateSHA256,omitempty"`
	APIToken            string                  `json:"APIToken,omitempty"`
	APITokenConfigured  bool                    `json:"APITokenConfigured,omitempty"`
	Status              string                  `json:"Status"`
	CPU                 *float64                `json:"CPU,omitempty"`
	Memory              *float64                `json:"Memory,omitempty"`
	PanelVersion        string                  `json:"PanelVersion,omitempty"`
	TapXVersion         string                  `json:"TapXVersion,omitempty"`
	EmbeddedXrayVersion string                  `json:"EmbeddedXrayVersion,omitempty"`
	ExternalXrayVersion string                  `json:"ExternalXrayVersion,omitempty"`
	Uptime              string                  `json:"Uptime,omitempty"`
	Latency             *int64                  `json:"Latency,omitempty"`
	LastSeen            string                  `json:"LastSeen,omitempty"`
	ObjectCounts        ManagedNodeObjectCounts `json:"ObjectCounts,omitempty"`
}

type ManagedNodeObjectCounts struct {
	Devices    int `json:"devices"`
	Listeners  int `json:"listeners"`
	Users      int `json:"users"`
	Connectors int `json:"connectors"`
	Links      int `json:"links"`
}

type ManagedNodeMTLS struct {
	Enabled         bool   `json:"Enabled"`
	CertificateFile string `json:"CertificateFile,omitempty"`
	PrivateKeyFile  string `json:"PrivateKeyFile,omitempty"`
	CAFile          string `json:"CAFile,omitempty"`
}

type managedNodeRegistry struct {
	Nodes []ManagedNode   `json:"nodes"`
	MTLS  ManagedNodeMTLS `json:"mtls"`
}

func (s *Server) handleManagedNodes(w http.ResponseWriter, r *http.Request) {
	registry, err := s.loadManagedNodeRegistry(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]ManagedNode, len(registry.Nodes))
	for index := range registry.Nodes {
		views[index] = managedNodeView(registry.Nodes[index])
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": views})
}

func (s *Server) handleManagedNodePut(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var input ManagedNode
	if err := decodeLimitedJSON(r, &input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if input.ID == "" {
		input.ID = id
	}
	if input.ID != id {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("node id does not match request path"))
		return
	}

	s.nodesMu.Lock()
	defer s.nodesMu.Unlock()
	registry, err := s.loadManagedNodeRegistry(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	index := managedNodeIndex(registry.Nodes, id)
	if index >= 0 {
		existing := registry.Nodes[index]
		if input.APIToken == "" {
			input.APIToken = existing.APIToken
		}
		input.Status = existing.Status
		input.CPU = existing.CPU
		input.Memory = existing.Memory
		input.PanelVersion = existing.PanelVersion
		input.TapXVersion = existing.TapXVersion
		input.EmbeddedXrayVersion = existing.EmbeddedXrayVersion
		input.ExternalXrayVersion = existing.ExternalXrayVersion
		input.Uptime = existing.Uptime
		input.Latency = existing.Latency
		input.LastSeen = existing.LastSeen
		input.ObjectCounts = existing.ObjectCounts
	} else {
		input.Status = "offline"
	}
	input.APITokenConfigured = false
	if err := normalizeManagedNode(&input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if index < 0 {
		registry.Nodes = append(registry.Nodes, input)
	} else {
		registry.Nodes[index] = input
	}
	if err := s.saveManagedNodeRegistry(r.Context(), registry); err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "node.save", fmt.Sprintf("managed node %s saved", id))
	writeJSON(w, http.StatusOK, map[string]any{"node": managedNodeView(input)})
}

func (s *Server) handleManagedNodeDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	s.nodesMu.Lock()
	defer s.nodesMu.Unlock()
	registry, err := s.loadManagedNodeRegistry(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	index := managedNodeIndex(registry.Nodes, id)
	if index < 0 {
		writeErrorStatus(w, http.StatusNotFound, ErrNotFound)
		return
	}
	registry.Nodes = append(registry.Nodes[:index], registry.Nodes[index+1:]...)
	if err := s.saveManagedNodeRegistry(r.Context(), registry); err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "node.delete", fmt.Sprintf("managed node %s deleted", id))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleManagedNodeTest(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	node, _, err := s.findManagedNode(r.Context(), id)
	if err != nil {
		writeManagedNodeError(w, err)
		return
	}
	node, err = s.probeManagedNode(r.Context(), node)
	if err != nil {
		node.Status = "offline"
		node.Latency = nil
		_ = s.replaceManagedNode(r.Context(), node)
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	if err := s.replaceManagedNode(r.Context(), node); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"node": managedNodeView(node)})
}

func (s *Server) handleManagedNodeDraftTest(w http.ResponseWriter, r *http.Request) {
	var node ManagedNode
	if err := decodeLimitedJSON(r, &node); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if node.APIToken == "" && node.ID != "" {
		registry, err := s.loadManagedNodeRegistry(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		if index := managedNodeIndex(registry.Nodes, node.ID); index >= 0 {
			node.APIToken = registry.Nodes[index].APIToken
		}
	}
	if err := normalizeManagedNode(&node); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	node, err := s.probeManagedNode(r.Context(), node)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"node": managedNodeView(node)})
}

func (s *Server) probeManagedNode(ctx context.Context, node ManagedNode) (ManagedNode, error) {
	started := time.Now()
	body, _, err := s.doManagedNodeRequest(ctx, node, http.MethodGet, "/api/diagnostics", nil)
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return node, err
	}
	var diagnostic struct {
		Product    string `json:"product"`
		Version    string `json:"version"`
		Components struct {
			Panel        string `json:"panel"`
			TapX         string `json:"tapx"`
			EmbeddedXray string `json:"embeddedXray"`
		} `json:"components"`
		Process struct {
			UptimeSecond int64 `json:"uptimeSecond"`
		} `json:"process"`
		ObjectCounts map[string]int `json:"objectCounts"`
	}
	if err := json.Unmarshal(body, &diagnostic); err != nil {
		return node, fmt.Errorf("decode node diagnostics: %w", err)
	}
	if diagnostic.Product != "TapX" {
		return node, fmt.Errorf("remote endpoint is not a TapX panel")
	}
	var dashboard struct {
		System struct {
			CPUPercent  float64 `json:"cpuPercent"`
			MemoryUsed  uint64  `json:"memoryUsed"`
			MemoryTotal uint64  `json:"memoryTotal"`
		} `json:"system"`
	}
	if dashboardBody, _, dashboardErr := s.doManagedNodeRequest(ctx, node, http.MethodGet, "/api/dashboard", nil); dashboardErr == nil {
		_ = json.Unmarshal(dashboardBody, &dashboard)
	}
	node.Status = "online"
	node.LastSeen = time.Now().UTC().Format(time.RFC3339Nano)
	node.Latency = &latency
	node.PanelVersion = firstNonEmpty(diagnostic.Components.Panel, diagnostic.Version)
	node.TapXVersion = firstNonEmpty(diagnostic.Components.TapX, diagnostic.Version)
	node.EmbeddedXrayVersion = diagnostic.Components.EmbeddedXray
	node.Uptime = formatManagedNodeUptime(diagnostic.Process.UptimeSecond)
	node.ObjectCounts = ManagedNodeObjectCounts{
		Devices: diagnostic.ObjectCounts[KindDevices], Listeners: diagnostic.ObjectCounts[KindListeners],
		Users: diagnostic.ObjectCounts[KindClients], Connectors: diagnostic.ObjectCounts[KindConnectors],
		Links: diagnostic.ObjectCounts[KindRoutes],
	}
	node.CPU = &dashboard.System.CPUPercent
	if dashboard.System.MemoryTotal > 0 {
		memory := float64(dashboard.System.MemoryUsed) * 100 / float64(dashboard.System.MemoryTotal)
		node.Memory = &memory
	}
	return node, nil
}

func (s *Server) handleManagedNodeConfig(w http.ResponseWriter, r *http.Request) {
	node, _, err := s.findManagedNode(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeManagedNodeError(w, err)
		return
	}
	var body []byte
	if r.Method == http.MethodPut {
		body, err = io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		if len(body) > maxRequestBody {
			writeErrorStatus(w, http.StatusRequestEntityTooLarge, fmt.Errorf("request body exceeds %d bytes", maxRequestBody))
			return
		}
		var cfg config.RuntimeConfig
		if err := json.Unmarshal(body, &cfg); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("decode node config: %w", err))
			return
		}
		if err := config.ValidateForSave(cfg); err != nil {
			writeErrorStatus(w, http.StatusUnprocessableEntity, err)
			return
		}
	}
	response, _, err := s.doManagedNodeRequest(r.Context(), node, r.Method, "/api/config", body)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

func (s *Server) handleManagedNodeRuntimeApply(w http.ResponseWriter, r *http.Request) {
	node, _, err := s.findManagedNode(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeManagedNodeError(w, err)
		return
	}
	response, _, err := s.doManagedNodeRequest(r.Context(), node, http.MethodPost, "/api/runtime/apply", nil)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

func (s *Server) handleManagedNodeStats(w http.ResponseWriter, r *http.Request) {
	s.proxyManagedNodeJSON(w, r, http.MethodGet, "/api/stats", nil, managedNodeRequestTimeout)
}

func (s *Server) handleManagedNodeSystemInterfaces(w http.ResponseWriter, r *http.Request) {
	s.proxyManagedNodeJSON(w, r, http.MethodGet, "/api/system/interfaces", nil, managedNodeRequestTimeout)
}

func (s *Server) handleManagedNodeClientShare(w http.ResponseWriter, r *http.Request) {
	objectID, err := managedNodeObjectID(r.PathValue("objectID"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	s.proxyManagedNodeJSON(w, r, http.MethodGet, "/api/share/clients/"+url.PathEscape(objectID), nil, managedNodeRequestTimeout)
}

func (s *Server) handleManagedNodeConnectorTest(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	var request connectorTestRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	request.ID = strings.TrimSpace(request.ID)
	if request.ID == "" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("connector id is required"))
		return
	}
	if request.Kind != "channel" && request.Kind != "path-mtu" && request.Kind != "throughput" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("kind must be channel, path-mtu, or throughput"))
		return
	}
	s.proxyManagedNodeJSON(w, r, http.MethodPost, "/api/connectors/test", body, managedNodeDiagnosticTimeout)
}

func (s *Server) handleManagedNodeTrafficReset(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.PathValue("kind"))
	if kind != "clients" && kind != "connectors" && kind != "listeners" {
		writeErrorStatus(w, http.StatusNotFound, fmt.Errorf("unsupported managed object kind %q", kind))
		return
	}
	objectID, err := managedNodeObjectID(r.PathValue("objectID"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	target := "/api/" + kind + "/" + url.PathEscape(objectID) + "/traffic/reset"
	s.proxyManagedNodeJSON(w, r, http.MethodPost, target, nil, managedNodeRequestTimeout)
}

func (s *Server) handleManagedNodeIntegration(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(r.PathValue("provider"))
	action := strings.TrimSpace(r.PathValue("action"))
	wantMethod, ok := managedNodeIntegrationMethod(provider, action)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != wantMethod {
		methodNotAllowed(w, wantMethod)
		return
	}

	target := "/api/integrations/" + provider + "/" + action
	query := r.URL.Query()
	if provider == "nord" && action == "servers" {
		if len(query) != 1 || len(query["countryId"]) != 1 {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("countryId is required"))
			return
		}
		countryID, err := strconv.Atoi(strings.TrimSpace(query.Get("countryId")))
		if err != nil || countryID <= 0 {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("countryId must be a positive integer"))
			return
		}
		target += "?countryId=" + strconv.Itoa(countryID)
	} else if len(query) != 0 {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("query parameters are not supported"))
		return
	}

	var body []byte
	if wantMethod == http.MethodPost {
		var err error
		body, err = readBody(r)
		if err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
	}
	s.proxyManagedNodeJSON(w, r, wantMethod, target, body, managedNodeDiagnosticTimeout)
}

func managedNodeIntegrationMethod(provider, action string) (string, bool) {
	methods := map[string]map[string]string{
		"warp": {
			"data": http.MethodGet, "register": http.MethodPost, "rotate": http.MethodPost,
			"config": http.MethodGet, "license": http.MethodPost, "interval": http.MethodPost,
			"delete": http.MethodDelete,
		},
		"nord": {
			"data": http.MethodGet, "countries": http.MethodGet, "servers": http.MethodGet,
			"login": http.MethodPost, "private-key": http.MethodPost, "delete": http.MethodDelete,
		},
	}
	providerMethods, ok := methods[provider]
	if !ok {
		return "", false
	}
	method, ok := providerMethods[action]
	return method, ok
}

func (s *Server) proxyManagedNodeJSON(w http.ResponseWriter, r *http.Request, method, target string, body []byte, timeout time.Duration) {
	node, _, err := s.findManagedNode(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeManagedNodeError(w, err)
		return
	}
	response, status, err := s.doManagedNodeRequestWithTimeout(r.Context(), node, method, target, body, timeout)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(response)
}

func managedNodeObjectID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("managed object id is required")
	}
	if value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
		return "", fmt.Errorf("managed object id contains an invalid path segment")
	}
	return value, nil
}

func (s *Server) handleManagedNodeUpdate(w http.ResponseWriter, r *http.Request) {
	node, _, err := s.findManagedNode(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeManagedNodeError(w, err)
		return
	}
	body, _, err := s.doManagedNodeRequest(r.Context(), node, http.MethodGet, "/api/updates/panel", nil)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	var catalog updateCatalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		writeErrorStatus(w, http.StatusBadGateway, fmt.Errorf("decode node update catalog: %w", err))
		return
	}
	version := ""
	for _, candidate := range catalog.Versions {
		if candidate.Installable && candidate.Latest {
			version = candidate.Version
			break
		}
	}
	if version == "" {
		for _, candidate := range catalog.Versions {
			if candidate.Installable {
				version = candidate.Version
				break
			}
		}
	}
	if version == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated": false, "message": "node is already current or automatic update is unavailable"})
		return
	}
	payload, err := json.Marshal(componentUpdateRequest{Version: version})
	if err != nil {
		writeError(w, err)
		return
	}
	response, _, err := s.doManagedNodeRequest(r.Context(), node, http.MethodPost, "/api/updates/panel", payload)
	if err != nil {
		writeErrorStatus(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(response)
}

func (s *Server) handleManagedNodeMTLSGet(w http.ResponseWriter, r *http.Request) {
	registry, err := s.loadManagedNodeRegistry(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mtls": registry.MTLS})
}

func (s *Server) handleManagedNodeMTLSPut(w http.ResponseWriter, r *http.Request) {
	var input ManagedNodeMTLS
	if err := decodeLimitedJSON(r, &input); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	if input.Enabled {
		if strings.TrimSpace(input.CertificateFile) == "" || strings.TrimSpace(input.PrivateKeyFile) == "" {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("mTLS certificate and private key are required"))
			return
		}
		if _, err := tls.LoadX509KeyPair(input.CertificateFile, input.PrivateKeyFile); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("load mTLS key pair: %w", err))
			return
		}
		if input.CAFile != "" {
			if _, err := os.ReadFile(input.CAFile); err != nil {
				writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("read mTLS CA: %w", err))
				return
			}
		}
	}
	s.nodesMu.Lock()
	defer s.nodesMu.Unlock()
	registry, err := s.loadManagedNodeRegistry(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	registry.MTLS = input
	if err := s.saveManagedNodeRegistry(r.Context(), registry); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mtls": registry.MTLS})
}

func (s *Server) loadManagedNodeRegistry(ctx context.Context) (managedNodeRegistry, error) {
	raw, err := s.store.GetIntegration(ctx, managedNodesIntegrationName)
	if errors.Is(err, ErrNotFound) {
		return managedNodeRegistry{Nodes: []ManagedNode{}}, nil
	}
	if err != nil {
		return managedNodeRegistry{}, err
	}
	var registry managedNodeRegistry
	if err := json.Unmarshal(raw, &registry); err != nil {
		return managedNodeRegistry{}, fmt.Errorf("decode managed node registry: %w", err)
	}
	if registry.Nodes == nil {
		registry.Nodes = []ManagedNode{}
	}
	return registry, nil
}

func (s *Server) saveManagedNodeRegistry(ctx context.Context, registry managedNodeRegistry) error {
	return s.store.SetIntegration(ctx, managedNodesIntegrationName, registry)
}

func (s *Server) findManagedNode(ctx context.Context, id string) (ManagedNode, managedNodeRegistry, error) {
	registry, err := s.loadManagedNodeRegistry(ctx)
	if err != nil {
		return ManagedNode{}, registry, err
	}
	index := managedNodeIndex(registry.Nodes, id)
	if index < 0 {
		return ManagedNode{}, registry, ErrNotFound
	}
	node := registry.Nodes[index]
	if !node.Enabled {
		return ManagedNode{}, registry, fmt.Errorf("managed node %s is disabled", id)
	}
	return node, registry, nil
}

func (s *Server) replaceManagedNode(ctx context.Context, node ManagedNode) error {
	s.nodesMu.Lock()
	defer s.nodesMu.Unlock()
	current, err := s.loadManagedNodeRegistry(ctx)
	if err != nil {
		return err
	}
	index := managedNodeIndex(current.Nodes, node.ID)
	if index < 0 {
		return ErrNotFound
	}
	// Keep credentials and operator settings from the latest record if an edit
	// raced with an in-flight status probe.
	latest := current.Nodes[index]
	node.Enabled = latest.Enabled
	node.Name = latest.Name
	node.Remark = latest.Remark
	node.Protocol = latest.Protocol
	node.Host = latest.Host
	node.Port = latest.Port
	node.BasePath = latest.BasePath
	node.AllowPrivateAddress = latest.AllowPrivateAddress
	node.TLSVerify = latest.TLSVerify
	node.CertificateSHA256 = latest.CertificateSHA256
	node.APIToken = latest.APIToken
	current.Nodes[index] = node
	return s.saveManagedNodeRegistry(ctx, current)
}

func (s *Server) doManagedNodeRequest(ctx context.Context, node ManagedNode, method, path string, body []byte) ([]byte, int, error) {
	return s.doManagedNodeRequestWithTimeout(ctx, node, method, path, body, managedNodeRequestTimeout)
}

func (s *Server) doManagedNodeRequestWithTimeout(ctx context.Context, node ManagedNode, method, path string, body []byte, timeout time.Duration) ([]byte, int, error) {
	registry, err := s.loadManagedNodeRegistry(ctx)
	if err != nil {
		return nil, 0, err
	}
	client, err := managedNodeHTTPClient(ctx, node, registry.MTLS)
	if err != nil {
		return nil, 0, err
	}
	if timeout <= 0 {
		timeout = managedNodeRequestTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, managedNodeURL(node, path), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+node.APIToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("request managed node %s: %w", node.Name, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, managedNodeResponseLimit+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if len(responseBody) > managedNodeResponseLimit {
		return nil, response.StatusCode, fmt.Errorf("managed node response exceeds %d bytes", managedNodeResponseLimit)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		message := strings.TrimSpace(string(responseBody))
		if len(message) > 512 {
			message = message[:512]
		}
		return nil, response.StatusCode, fmt.Errorf("managed node %s returned %s: %s", node.Name, response.Status, message)
	}
	return responseBody, response.StatusCode, nil
}

func managedNodeHTTPClient(ctx context.Context, node ManagedNode, mtls ManagedNodeMTLS) (*http.Client, error) {
	if err := normalizeManagedNode(&node); err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: strings.Trim(node.Host, "[]")}
	switch node.TLSVerify {
	case "pin":
		fingerprint, err := decodeManagedNodeFingerprint(node.CertificateSHA256)
		if err != nil {
			return nil, err
		}
		tlsConfig.InsecureSkipVerify = true // Verification is replaced by the exact leaf certificate pin below.
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return fmt.Errorf("managed node did not present a certificate")
			}
			digest := sha256.Sum256(state.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(digest[:], fingerprint) != 1 {
				return fmt.Errorf("managed node certificate fingerprint mismatch")
			}
			return nil
		}
	case "skip":
		tlsConfig.InsecureSkipVerify = true
	}
	if mtls.Enabled {
		certificate, err := tls.LoadX509KeyPair(mtls.CertificateFile, mtls.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load managed node mTLS key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	if mtls.CAFile != "" && node.TLSVerify == "system" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate pool: %w", err)
		}
		pem, err := os.ReadFile(mtls.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read managed node CA: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("managed node CA contains no certificates")
		}
		tlsConfig.RootCAs = pool
	}
	dialer := &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSClientConfig:       tlsConfig,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	transport.DialContext = func(dialCtx context.Context, network, address string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolveManagedNodeIPs(dialCtx, strings.Trim(node.Host, "[]"), node.AllowPrivateAddress)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			connection, dialErr := dialer.DialContext(dialCtx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("managed node redirects are not allowed")
		},
	}, nil
}

func resolveManagedNodeIPs(ctx context.Context, host string, allowPrivate bool) ([]net.IP, error) {
	var candidates []net.IP
	if parsed := net.ParseIP(host); parsed != nil {
		candidates = []net.IP{parsed}
	} else {
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve managed node host: %w", err)
		}
		for _, item := range resolved {
			candidates = append(candidates, item.IP)
		}
	}
	allowed := make([]net.IP, 0, len(candidates))
	for _, ip := range candidates {
		if forbiddenManagedNodeIP(ip) || (!allowPrivate && privateManagedNodeIP(ip)) {
			continue
		}
		allowed = append(allowed, ip)
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("managed node host resolves only to private or special-use addresses")
	}
	return allowed, nil
}

func forbiddenManagedNodeIP(ip net.IP) bool {
	return ip == nil || ip.IsUnspecified() || ip.IsMulticast()
}

func privateManagedNodeIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func normalizeManagedNode(node *ManagedNode) error {
	node.ID = strings.TrimSpace(node.ID)
	node.Name = strings.TrimSpace(node.Name)
	node.Remark = strings.TrimSpace(node.Remark)
	node.Protocol = strings.ToLower(strings.TrimSpace(node.Protocol))
	node.Host = strings.Trim(strings.TrimSpace(node.Host), "[]")
	node.BasePath = normalizeManagedNodeBasePath(node.BasePath)
	node.TLSVerify = strings.ToLower(strings.TrimSpace(node.TLSVerify))
	node.CertificateSHA256 = normalizeManagedNodeFingerprint(node.CertificateSHA256)
	node.APIToken = strings.TrimSpace(node.APIToken)
	if !managedNodeIDPattern.MatchString(node.ID) {
		return fmt.Errorf("node id is invalid")
	}
	if node.Name == "" || len(node.Name) > 128 {
		return fmt.Errorf("node name is required and must not exceed 128 characters")
	}
	if node.Protocol != "https" && node.Protocol != "http" {
		return fmt.Errorf("node protocol must be http or https")
	}
	if node.Protocol == "http" && !node.AllowPrivateAddress {
		return fmt.Errorf("plain HTTP requires explicit private-address permission")
	}
	if node.Host == "" || strings.ContainsAny(node.Host, "/\\@?#") || strings.Contains(node.Host, "://") {
		return fmt.Errorf("node host is invalid")
	}
	if node.Port == 0 {
		return fmt.Errorf("node port is required")
	}
	if node.TLSVerify == "" {
		node.TLSVerify = "system"
	}
	if node.TLSVerify != "system" && node.TLSVerify != "pin" && node.TLSVerify != "skip" {
		return fmt.Errorf("node TLS verification mode is invalid")
	}
	if node.Protocol == "https" && node.TLSVerify == "pin" {
		if _, err := decodeManagedNodeFingerprint(node.CertificateSHA256); err != nil {
			return err
		}
	}
	if node.APIToken == "" {
		return fmt.Errorf("node API token is required")
	}
	if node.Status != "online" && node.Status != "checking" {
		node.Status = "offline"
	}
	return nil
}

func managedNodeView(node ManagedNode) ManagedNode {
	node.APITokenConfigured = node.APIToken != ""
	node.APIToken = ""
	return node
}

func managedNodeIndex(nodes []ManagedNode, id string) int {
	for index := range nodes {
		if nodes[index].ID == id {
			return index
		}
	}
	return -1
}

func managedNodeURL(node ManagedNode, path string) string {
	host := node.Host
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	defaultPort := uint16(80)
	if node.Protocol == "https" {
		defaultPort = 443
	}
	port := ""
	if node.Port != defaultPort {
		port = ":" + strconv.Itoa(int(node.Port))
	}
	base := strings.TrimSuffix(normalizeManagedNodeBasePath(node.BasePath), "/")
	return node.Protocol + "://" + host + port + base + "/" + strings.TrimPrefix(path, "/")
}

func normalizeManagedNodeBasePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return "/"
	}
	return "/" + strings.Trim(value, "/") + "/"
}

func normalizeManagedNodeFingerprint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "sha256:")
	return strings.ReplaceAll(value, ":", "")
}

func decodeManagedNodeFingerprint(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(normalizeManagedNodeFingerprint(value))
	if err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("node certificate SHA256 fingerprint must contain 64 hexadecimal characters")
	}
	return decoded, nil
}

func formatManagedNodeUptime(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func decodeLimitedJSON(r *http.Request, target any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		return err
	}
	if len(raw) > maxRequestBody {
		return fmt.Errorf("request body exceeds %d bytes", maxRequestBody)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	return nil
}

func writeManagedNodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeErrorStatus(w, http.StatusNotFound, err)
		return
	}
	writeErrorStatus(w, http.StatusBadRequest, err)
}
