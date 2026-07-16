package panel

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	warpIntegrationName = "warp"
	nordIntegrationName = "nord"
	warpAPIBase         = "https://api.cloudflareclient.com/v0a4005"
	warpClientVersion   = "a-6.30-3596"
	maxIntegrationReply = 10 << 20
)

type integrationScheduler struct {
	mu        sync.Mutex
	warpTimer *time.Timer
}

type warpIntegrationState struct {
	AccessToken        string `json:"access_token"`
	DeviceID           string `json:"device_id"`
	LicenseKey         string `json:"license_key"`
	PrivateKey         string `json:"private_key"`
	ClientID           string `json:"client_id,omitempty"`
	UpdateIntervalDays int    `json:"update_interval_days,omitempty"`
	LastUpdatedAt      int64  `json:"last_updated_at,omitempty"`
}

type nordIntegrationState struct {
	Token      string `json:"token,omitempty"`
	PrivateKey string `json:"private_key"`
}

func (s *Server) handleWarpIntegration(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/integrations/warp/"), "/")
	switch action {
	case "data":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		state, err := s.loadWarpIntegration(r.Context())
		if errors.Is(err, ErrNotFound) {
			writeIntegrationOK(w, nil)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		writeIntegrationOK(w, state)
	case "register", "rotate":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var req struct {
			PrivateKey string `json:"privateKey"`
			PublicKey  string `json:"publicKey"`
		}
		if err := decodeIntegrationRequest(r, &req); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		if err := validateWireguardKey(req.PrivateKey); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("private key: %w", err))
			return
		}
		if err := validateWireguardKey(req.PublicKey); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("public key: %w", err))
			return
		}
		var oldState *warpIntegrationState
		if action == "rotate" {
			oldState, _ = s.loadWarpIntegration(r.Context())
		}
		state, config, err := s.registerWarp(r.Context(), req.PrivateKey, req.PublicKey)
		if err != nil {
			writeErrorStatus(w, http.StatusBadGateway, err)
			return
		}
		if oldState != nil {
			state.UpdateIntervalDays = oldState.UpdateIntervalDays
			if len(oldState.LicenseKey) >= 26 {
				if _, err := s.setWarpLicense(r.Context(), state, oldState.LicenseKey); err != nil {
					writeErrorStatus(w, http.StatusBadGateway, err)
					return
				}
			}
		}
		if err := s.store.SetIntegration(r.Context(), warpIntegrationName, state); err != nil {
			writeError(w, err)
			return
		}
		if action == "rotate" {
			if err := s.syncWarpConnector(r.Context(), state, config); err != nil {
				writeError(w, err)
				return
			}
			s.scheduleWarpRotation(state)
		}
		writeIntegrationOK(w, map[string]any{"data": state, "config": config})
	case "config":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		state, err := s.loadWarpIntegration(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		config, err := s.fetchWarpConfig(r.Context(), state)
		if err != nil {
			writeErrorStatus(w, http.StatusBadGateway, err)
			return
		}
		writeIntegrationOK(w, config)
	case "license":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var req struct {
			License string `json:"license"`
		}
		if err := decodeIntegrationRequest(r, &req); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		if len(strings.TrimSpace(req.License)) < 26 {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("WARP+ license key is invalid"))
			return
		}
		state, err := s.loadWarpIntegration(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		state, err = s.setWarpLicense(r.Context(), state, strings.TrimSpace(req.License))
		if err != nil {
			writeErrorStatus(w, http.StatusBadGateway, err)
			return
		}
		if err := s.store.SetIntegration(r.Context(), warpIntegrationName, state); err != nil {
			writeError(w, err)
			return
		}
		s.scheduleWarpRotation(state)
		writeIntegrationOK(w, state)
	case "interval":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var req struct {
			Days int `json:"days"`
		}
		if err := decodeIntegrationRequest(r, &req); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		if req.Days < 0 || req.Days > 3650 {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("update interval must be between 0 and 3650 days"))
			return
		}
		state, err := s.loadWarpIntegration(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		state.UpdateIntervalDays = req.Days
		if err := s.store.SetIntegration(r.Context(), warpIntegrationName, state); err != nil {
			writeError(w, err)
			return
		}
		writeIntegrationOK(w, state)
	case "delete":
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, http.MethodDelete)
			return
		}
		if err := s.store.DeleteIntegration(r.Context(), warpIntegrationName); err != nil && !errors.Is(err, ErrNotFound) {
			writeError(w, err)
			return
		}
		s.cancelWarpRotation()
		writeIntegrationOK(w, nil)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleNordIntegration(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/integrations/nord/"), "/")
	switch action {
	case "data":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		state, err := s.loadNordIntegration(r.Context())
		if errors.Is(err, ErrNotFound) {
			writeIntegrationOK(w, nil)
			return
		}
		if err != nil {
			writeError(w, err)
			return
		}
		writeIntegrationOK(w, state)
	case "countries":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		data, err := s.getIntegrationJSON(r.Context(), "https://api.nordvpn.com/v1/countries", nil)
		if err != nil {
			writeErrorStatus(w, http.StatusBadGateway, err)
			return
		}
		writeIntegrationOK(w, data)
	case "servers":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		countryID := r.URL.Query().Get("countryId")
		if _, err := strconv.ParseUint(countryID, 10, 64); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("invalid country ID"))
			return
		}
		query := url.Values{}
		query.Set("limit", "0")
		query.Set("filters[servers_technologies][id]", "35")
		query.Set("filters[country_id]", countryID)
		data, err := s.getIntegrationJSON(r.Context(), "https://api.nordvpn.com/v2/servers?"+query.Encode(), nil)
		if err != nil {
			writeErrorStatus(w, http.StatusBadGateway, err)
			return
		}
		writeIntegrationOK(w, data)
	case "login":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := decodeIntegrationRequest(r, &req); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		state, err := s.fetchNordCredentials(r.Context(), strings.TrimSpace(req.Token))
		if err != nil {
			writeErrorStatus(w, http.StatusBadGateway, err)
			return
		}
		if err := s.store.SetIntegration(r.Context(), nordIntegrationName, state); err != nil {
			writeError(w, err)
			return
		}
		writeIntegrationOK(w, state)
	case "private-key":
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		var req struct {
			PrivateKey string `json:"privateKey"`
		}
		if err := decodeIntegrationRequest(r, &req); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		if err := validateWireguardKey(req.PrivateKey); err != nil {
			writeErrorStatus(w, http.StatusBadRequest, err)
			return
		}
		state := &nordIntegrationState{PrivateKey: req.PrivateKey}
		if err := s.store.SetIntegration(r.Context(), nordIntegrationName, state); err != nil {
			writeError(w, err)
			return
		}
		writeIntegrationOK(w, state)
	case "delete":
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, http.MethodDelete)
			return
		}
		if err := s.store.DeleteIntegration(r.Context(), nordIntegrationName); err != nil && !errors.Is(err, ErrNotFound) {
			writeError(w, err)
			return
		}
		writeIntegrationOK(w, nil)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) registerWarp(ctx context.Context, privateKey, publicKey string) (*warpIntegrationState, map[string]any, error) {
	hostname, _ := os.Hostname()
	payload := map[string]any{
		"key":   publicKey,
		"tos":   time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		"type":  "PC",
		"model": "TapX",
		"name":  hostname,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, warpAPIBase+"/reg", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("CF-Client-Version", warpClientVersion)
	req.Header.Set("Content-Type", "application/json")
	var response map[string]any
	if err := s.doIntegrationRequest(req, &response); err != nil {
		return nil, nil, err
	}
	deviceID, _ := response["id"].(string)
	token, _ := response["token"].(string)
	account, _ := response["account"].(map[string]any)
	license, _ := account["license"].(string)
	if deviceID == "" || token == "" || license == "" {
		return nil, nil, fmt.Errorf("WARP registration response is missing account credentials")
	}
	state := &warpIntegrationState{
		AccessToken:   token,
		DeviceID:      deviceID,
		LicenseKey:    license,
		PrivateKey:    privateKey,
		LastUpdatedAt: time.Now().Unix(),
	}
	if config, ok := response["config"].(map[string]any); ok {
		state.ClientID, _ = config["client_id"].(string)
	}
	return state, response, nil
}

func (s *Server) fetchWarpConfig(ctx context.Context, state *warpIntegrationState) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, warpAPIBase+"/reg/"+url.PathEscape(state.DeviceID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+state.AccessToken)
	var response map[string]any
	if err := s.doIntegrationRequest(req, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Server) setWarpLicense(ctx context.Context, state *warpIntegrationState, license string) (*warpIntegrationState, error) {
	body, err := json.Marshal(map[string]string{"license": license})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, warpAPIBase+"/reg/"+url.PathEscape(state.DeviceID)+"/account", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+state.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	var response map[string]any
	if err := s.doIntegrationRequest(req, &response); err != nil {
		return nil, err
	}
	if id, _ := response["id"].(string); id == "" {
		return nil, fmt.Errorf("WARP license response is invalid")
	}
	state.LicenseKey = license
	return state, nil
}

func (s *Server) fetchNordCredentials(ctx context.Context, token string) (*nordIntegrationState, error) {
	if token == "" {
		return nil, fmt.Errorf("NordVPN access token is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.nordvpn.com/v1/users/services/credentials", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("token", token)
	var response struct {
		PrivateKey string `json:"nordlynx_private_key"`
	}
	if err := s.doIntegrationRequest(req, &response); err != nil {
		return nil, err
	}
	if err := validateWireguardKey(response.PrivateKey); err != nil {
		return nil, fmt.Errorf("NordVPN did not return a valid NordLynx private key")
	}
	return &nordIntegrationState{Token: token, PrivateKey: response.PrivateKey}, nil
}

func (s *Server) loadWarpIntegration(ctx context.Context) (*warpIntegrationState, error) {
	raw, err := s.store.GetIntegration(ctx, warpIntegrationName)
	if err != nil {
		return nil, err
	}
	var state warpIntegrationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if state.AccessToken == "" || state.DeviceID == "" || state.PrivateKey == "" {
		return nil, fmt.Errorf("stored WARP account is incomplete")
	}
	return &state, nil
}

func (s *Server) loadNordIntegration(ctx context.Context) (*nordIntegrationState, error) {
	raw, err := s.store.GetIntegration(ctx, nordIntegrationName)
	if err != nil {
		return nil, err
	}
	var state nordIntegrationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if state.PrivateKey == "" {
		return nil, fmt.Errorf("stored NordVPN account is incomplete")
	}
	return &state, nil
}

func (s *Server) getIntegrationJSON(ctx context.Context, endpoint string, headers map[string]string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	var response any
	if err := s.doIntegrationRequest(req, &response); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Server) doIntegrationRequest(req *http.Request, target any) error {
	client, err := s.panelHTTPClient(req.Context())
	if err != nil {
		return err
	}
	client.Timeout = 15 * time.Second
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxIntegrationReply+1))
	if err != nil {
		return err
	}
	if len(body) > maxIntegrationReply {
		return fmt.Errorf("integration response exceeds %d bytes", maxIntegrationReply)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("integration API returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode integration response: %w", err)
	}
	return nil
}

func decodeIntegrationRequest(r *http.Request, target any) error {
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("Content-Type must be application/json")
	}
	body, err := readBody(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func validateWireguardKey(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != 32 {
		return fmt.Errorf("WireGuard key must be a 32-byte base64 value")
	}
	return nil
}

func writeIntegrationOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": data})
}

func (s *Server) restoreIntegrationSchedules() {
	state, err := s.loadWarpIntegration(context.Background())
	if err == nil {
		s.scheduleWarpRotation(state)
	}
}

func (s *Server) scheduleWarpRotation(state *warpIntegrationState) {
	s.integrations.mu.Lock()
	defer s.integrations.mu.Unlock()
	if s.integrations.warpTimer != nil {
		s.integrations.warpTimer.Stop()
		s.integrations.warpTimer = nil
	}
	if state == nil || state.UpdateIntervalDays <= 0 {
		return
	}
	interval := time.Duration(state.UpdateIntervalDays) * 24 * time.Hour
	lastUpdated := time.Unix(state.LastUpdatedAt, 0)
	remaining := interval - time.Since(lastUpdated)
	if state.LastUpdatedAt <= 0 || remaining <= 0 {
		remaining = time.Second
	}
	s.integrations.warpTimer = time.AfterFunc(remaining, s.rotateWarpScheduled)
}

func (s *Server) cancelWarpRotation() {
	s.integrations.mu.Lock()
	defer s.integrations.mu.Unlock()
	if s.integrations.warpTimer != nil {
		s.integrations.warpTimer.Stop()
		s.integrations.warpTimer = nil
	}
}

func (s *Server) rotateWarpScheduled() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	oldState, err := s.loadWarpIntegration(ctx)
	if err != nil {
		return
	}
	privateKey, publicKey, err := generateWireguardKeypair()
	if err != nil {
		s.log("error", "integration.warp.rotate", err.Error())
		s.scheduleWarpRotation(oldState)
		return
	}
	state, deviceConfig, err := s.registerWarp(ctx, privateKey, publicKey)
	if err != nil {
		s.log("error", "integration.warp.rotate", err.Error())
		s.scheduleWarpRotation(oldState)
		return
	}
	state.UpdateIntervalDays = oldState.UpdateIntervalDays
	if len(oldState.LicenseKey) >= 26 {
		state, err = s.setWarpLicense(ctx, state, oldState.LicenseKey)
		if err != nil {
			s.log("error", "integration.warp.rotate", err.Error())
			s.scheduleWarpRotation(oldState)
			return
		}
	}
	if err := s.store.SetIntegration(ctx, warpIntegrationName, state); err != nil {
		s.log("error", "integration.warp.rotate", err.Error())
		s.scheduleWarpRotation(oldState)
		return
	}
	if err := s.syncWarpConnector(ctx, state, deviceConfig); err != nil {
		s.log("error", "integration.warp.rotate", err.Error())
	}
	s.log("info", "integration.warp.rotate", "WARP address and connector keys updated")
	s.scheduleWarpRotation(state)
}

func (s *Server) syncWarpConnector(ctx context.Context, state *warpIntegrationState, deviceConfig map[string]any) error {
	cfg, err := s.store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	connectorIndex := -1
	for index := range cfg.Connectors {
		if cfg.Connectors[index].Name == "warp" {
			connectorIndex = index
			break
		}
	}
	if connectorIndex < 0 {
		return nil
	}
	settings, endpoint, err := buildWarpOutboundSettings(state, deviceConfig)
	if err != nil {
		return err
	}
	connector := &cfg.Connectors[connectorIndex]
	host, port, err := splitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("WARP endpoint: %w", err)
	}
	connector.Remote = host
	connector.Port = port
	for index := range cfg.XrayProfiles {
		if cfg.XrayProfiles[index].ID != connector.XrayProfileID {
			continue
		}
		cfg.XrayProfiles[index].OutboundProtocol = "wireguard"
		cfg.XrayProfiles[index].OutboundSettingsJSON = string(settings)
		cfg.XrayProfiles[index].Network = ""
		cfg.XrayProfiles[index].Security = "none"
		return s.store.ReplaceConfig(ctx, cfg)
	}
	return fmt.Errorf("WARP connector %q references missing Xray profile %q", connector.ID, connector.XrayProfileID)
}

func buildWarpOutboundSettings(state *warpIntegrationState, deviceConfig map[string]any) ([]byte, string, error) {
	raw, err := json.Marshal(deviceConfig)
	if err != nil {
		return nil, "", err
	}
	var config struct {
		Config struct {
			ClientID  string `json:"client_id"`
			Interface struct {
				Addresses struct {
					V4 string `json:"v4"`
					V6 string `json:"v6"`
				} `json:"addresses"`
			} `json:"interface"`
			Peers []struct {
				PublicKey string `json:"public_key"`
				Endpoint  struct {
					Host string `json:"host"`
				} `json:"endpoint"`
			} `json:"peers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, "", err
	}
	if len(config.Config.Peers) == 0 || config.Config.Peers[0].PublicKey == "" || config.Config.Peers[0].Endpoint.Host == "" {
		return nil, "", fmt.Errorf("WARP configuration is missing peer data")
	}
	addresses := make([]string, 0, 2)
	if config.Config.Interface.Addresses.V4 != "" {
		addresses = append(addresses, config.Config.Interface.Addresses.V4+"/32")
	}
	if config.Config.Interface.Addresses.V6 != "" {
		addresses = append(addresses, config.Config.Interface.Addresses.V6+"/128")
	}
	clientID := config.Config.ClientID
	if clientID == "" {
		clientID = state.ClientID
	}
	reserved := []int(nil)
	if clientID != "" {
		decoded, _ := base64.StdEncoding.DecodeString(clientID)
		reserved = make([]int, len(decoded))
		for index, value := range decoded {
			reserved[index] = int(value)
		}
	}
	settings, err := json.Marshal(map[string]any{
		"mtu":            1420,
		"secretKey":      state.PrivateKey,
		"address":        addresses,
		"reserved":       reserved,
		"domainStrategy": "ForceIPv4v6",
		"peers": []map[string]string{{
			"publicKey": config.Config.Peers[0].PublicKey,
			"endpoint":  config.Config.Peers[0].Endpoint.Host,
		}},
		"noKernelTun": true,
	})
	return settings, config.Config.Peers[0].Endpoint.Host, err
}

func generateWireguardKeypair() (string, string, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(privateKey.Bytes()), base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes()), nil
}

func splitHostPort(endpoint string) (string, uint16, error) {
	host, rawPort, err := net.SplitHostPort(strings.TrimSpace(endpoint))
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil || port == 0 {
		return "", 0, fmt.Errorf("port %q is outside 1..65535", rawPort)
	}
	return host, uint16(port), nil
}
