package panel

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tapx/internal/config"
	"tapx/internal/core"
	"tapx/internal/model"
)

const (
	maxRequestBody = 8 << 20
	maxBackupBody  = 64 << 20
)

type Server struct {
	store        *Store
	runtime      *RuntimeManager
	logs         *LogRecorder
	sessions     *SessionManager
	loginLimiter *LoginLimiter
	started      time.Time
	basePath     string
	dashboard    dashboardRateTracker
	system       systemSampler
	integrations integrationScheduler
	restart      func() error
	nodesMu      sync.Mutex
}

type ServerOptions struct {
	BasePath string
	Restart  func() error
}

func NewServer(store *Store, runtime ...*RuntimeManager) *Server {
	return NewServerWithOptions(store, ServerOptions{}, runtime...)
}

func NewServerWithOptions(store *Store, opts ServerOptions, runtime ...*RuntimeManager) *Server {
	manager := NewRuntimeManager()
	if len(runtime) > 0 && runtime[0] != nil {
		manager = runtime[0]
	}
	server := &Server{store: store, runtime: manager, logs: NewPersistentLogRecorder(store, defaultLogLimit), sessions: NewSessionManager(), loginLimiter: NewLoginLimiter(), started: time.Now(), basePath: normalizeBasePath(opts.BasePath), restart: opts.Restart}
	manager.SetLimitConfigRefresher(func(state RuntimeState, now time.Time) (config.RuntimeConfig, error) {
		return refreshLimitConfig(store, state, now)
	})
	server.restoreIntegrationSchedules()
	return server
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/auth/session", s.handleAuthSession)
	mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("POST /api/panel/credentials", s.handleAdminCredentials)
	mux.HandleFunc("POST /api/panel/restart", s.handlePanelRestart)
	mux.HandleFunc("GET /api/security", s.handleSecurityStatus)
	mux.HandleFunc("POST /api/security/totp/prepare", s.handleTOTPPrepare)
	mux.HandleFunc("POST /api/security/totp/enable", s.handleTOTPEnable)
	mux.HandleFunc("POST /api/security/totp/disable", s.handleTOTPDisable)
	mux.HandleFunc("POST /api/security/tokens", s.handleAPITokens)
	mux.HandleFunc("DELETE /api/security/tokens/{id}", s.handleAPITokenDelete)
	mux.HandleFunc("GET /api/nodes", s.handleManagedNodes)
	mux.HandleFunc("POST /api/nodes/test", s.handleManagedNodeDraftTest)
	mux.HandleFunc("PUT /api/nodes/{id}", s.handleManagedNodePut)
	mux.HandleFunc("DELETE /api/nodes/{id}", s.handleManagedNodeDelete)
	mux.HandleFunc("POST /api/nodes/{id}/test", s.handleManagedNodeTest)
	mux.HandleFunc("POST /api/nodes/{id}/update", s.handleManagedNodeUpdate)
	mux.HandleFunc("GET /api/nodes/{id}/config", s.handleManagedNodeConfig)
	mux.HandleFunc("PUT /api/nodes/{id}/config", s.handleManagedNodeConfig)
	mux.HandleFunc("POST /api/nodes/{id}/runtime/apply", s.handleManagedNodeRuntimeApply)
	mux.HandleFunc("GET /api/nodes/{id}/stats", s.handleManagedNodeStats)
	mux.HandleFunc("GET /api/nodes/{id}/system/interfaces", s.handleManagedNodeSystemInterfaces)
	mux.HandleFunc("GET /api/nodes/{id}/share/clients/{objectID}", s.handleManagedNodeClientShare)
	mux.HandleFunc("POST /api/nodes/{id}/connectors/test", s.handleManagedNodeConnectorTest)
	mux.HandleFunc("POST /api/nodes/{id}/{kind}/{objectID}/traffic/reset", s.handleManagedNodeTrafficReset)
	mux.HandleFunc("/api/nodes/{id}/integrations/{provider}/{action}", s.handleManagedNodeIntegration)
	mux.HandleFunc("GET /api/nodes/mtls", s.handleManagedNodeMTLSGet)
	mux.HandleFunc("PUT /api/nodes/mtls", s.handleManagedNodeMTLSPut)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/config/validate", s.handleValidate)
	mux.HandleFunc("GET /api/runtime/state", s.handleRuntimeState)
	mux.HandleFunc("POST /api/runtime/apply", s.handleRuntimeApply)
	mux.HandleFunc("POST /api/runtime/enforce", s.handleRuntimeEnforce)
	mux.HandleFunc("POST /api/runtime/stop", s.handleRuntimeStop)
	mux.HandleFunc("POST /api/runtime/components/{component}/{action}", s.handleRuntimeComponentAction)
	mux.HandleFunc("/api/runtime", s.handleRuntime)
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/server/interfaces", s.handleSystemInterfaces)
	mux.HandleFunc("GET /api/system/interfaces", s.handleSystemInterfaces)
	mux.HandleFunc("POST /api/connectors/test", s.handleConnectorTest)
	mux.HandleFunc("GET /panel/api/server/interfaces", s.handleSystemInterfaces)
	mux.HandleFunc("GET /api/templates/raw-pair", s.handleRawPairTemplate)
	mux.HandleFunc("/api/share/clients/", s.handleClientShare)
	mux.HandleFunc("/api/clients/", s.handleClientTraffic)
	mux.HandleFunc("/api/connectors/", s.handleConnectorTraffic)
	mux.HandleFunc("/api/listeners/", s.handleListenerTraffic)
	mux.HandleFunc("GET /api/xray/external/status", s.handleXrayExternalStatus)
	mux.HandleFunc("GET /api/xray/vless-encryption", s.handleVlessEncryption)
	mux.HandleFunc("GET /panel/api/server/getNewVlessEnc", s.handleVlessEncryption)
	mux.HandleFunc("GET /api/xray/reality/x25519", s.handleRealityX25519)
	mux.HandleFunc("GET /panel/api/server/getNewX25519Cert", s.handleRealityX25519)
	mux.HandleFunc("GET /api/xray/reality/mldsa65", s.handleRealityMLDSA65)
	mux.HandleFunc("GET /panel/api/server/getNewmldsa65", s.handleRealityMLDSA65)
	mux.HandleFunc("POST /api/xray/tls/ech", s.handleTLSECH)
	mux.HandleFunc("POST /panel/api/server/getNewEchCert", s.handleTLSECH)
	mux.HandleFunc("POST /api/xray/tls/cert-hash", s.handleTLSCertHash)
	mux.HandleFunc("POST /panel/api/server/getCertHash", s.handleTLSCertHash)
	mux.HandleFunc("POST /api/xray/tls/remote-cert-hash", s.handleTLSRemoteCertHash)
	mux.HandleFunc("POST /panel/api/server/getRemoteCertHash", s.handleTLSRemoteCertHash)
	mux.HandleFunc("POST /api/xray/reality/scan", s.handleRealityScanTarget)
	mux.HandleFunc("POST /panel/api/server/scanRealityTarget", s.handleRealityScanTarget)
	mux.HandleFunc("POST /api/xray/reality/scan-many", s.handleRealityScanTargets)
	mux.HandleFunc("POST /panel/api/server/scanRealityTargets", s.handleRealityScanTargets)
	mux.HandleFunc("/api/integrations/warp/", s.handleWarpIntegration)
	mux.HandleFunc("/api/integrations/nord/", s.handleNordIntegration)
	mux.HandleFunc("POST /api/xray/external/upload", s.handleXrayExternalUpload)
	mux.HandleFunc("POST /api/xray/external/download", s.handleXrayExternalDownload)
	mux.HandleFunc("GET /api/updates/{component}", s.handleUpdateCatalog)
	mux.HandleFunc("POST /api/updates/{component}", s.handleComponentUpdate)
	mux.HandleFunc("/api/backup", s.handleBackup)
	mux.HandleFunc("POST /api/backup/restore", s.handleBackupRestore)
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/diagnostics", s.handleDiagnostics)
	mux.HandleFunc("/api/objects/", s.handleObjects)
	mux.Handle("/", staticHandler(s.basePath))
	handler := s.authMiddleware(mux)
	if s.basePath == "" {
		return handler
	}
	return basePathHandler(s.basePath, handler)
}

func (s *Server) handleRawPairTemplate(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	port, err := queryUint16(query.Get("port"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("port: %w", err))
		return
	}
	mtu, err := queryInt(query.Get("mtu"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("mtu: %w", err))
		return
	}
	template, err := config.BuildRawPairTemplate(config.RawPairTemplateOptions{
		Transport: modelTransport(query.Get("transport")),
		HostA:     query.Get("hostA"),
		HostB:     query.Get("hostB"),
		Port:      port,
		TunA:      query.Get("tunA"),
		TunB:      query.Get("tunB"),
		IfNameA:   query.Get("ifNameA"),
		IfNameB:   query.Get("ifNameB"),
		MTU:       mtu,
		VKey:      query.Get("vkey"),
	})
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"template": template})
}

func (s *Server) handleClientShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/share/clients/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("client id is required"))
		return
	}
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	share, err := BuildClientShare(cfg, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"share": share})
}

func normalizeBasePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" || path == "/" {
		return ""
	}
	return path
}

func basePathHandler(basePath string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == basePath {
			http.Redirect(w, r, basePath+"/", http.StatusMovedPermanently)
			return
		}
		if !strings.HasPrefix(r.URL.Path, basePath+"/") {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix(basePath, next).ServeHTTP(w, r)
	})
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	auth, err := s.authConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	authenticated := !auth.Enabled || s.authenticated(r)
	username := ""
	if authenticated {
		username = auth.Username
	}
	security, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"authEnabled":      auth.Enabled,
		"authenticated":    authenticated,
		"username":         username,
		"twoFactorEnabled": security.TOTPSecret != "",
	})
}

type loginRequest struct {
	Username      string
	Password      string
	TwoFactorCode string
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	loginKey := remoteHost(r.RemoteAddr)
	if retryAfter := s.loginLimiter.RetryAfter(loginKey); retryAfter > 0 {
		writeLoginRateLimit(w, retryAfter)
		return
	}
	auth, err := s.authConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if !auth.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "authEnabled": false, "authenticated": true})
		return
	}

	raw, err := readBody(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var req loginRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, err)
		return
	}
	if subtleString(req.Username, auth.Username) != 1 || !VerifyPanelPassword(auth.PasswordHash, req.Password) {
		s.log("warn", "auth.login", "invalid login")
		if retryAfter := s.loginLimiter.Failure(loginKey); retryAfter > 0 {
			writeLoginRateLimit(w, retryAfter)
			return
		}
		writeErrorStatus(w, http.StatusUnauthorized, fmt.Errorf("invalid username or password"))
		return
	}
	security, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if security.TOTPSecret != "" && !verifyTOTP(security.TOTPSecret, req.TwoFactorCode, time.Now()) {
		s.log("warn", "auth.login", "invalid two-factor code")
		if retryAfter := s.loginLimiter.Failure(loginKey); retryAfter > 0 {
			writeLoginRateLimit(w, retryAfter)
			return
		}
		writeErrorStatus(w, http.StatusUnauthorized, fmt.Errorf("invalid two-factor code"))
		return
	}
	token, expires, err := s.sessions.Create(sessionTTL(auth))
	if err != nil {
		writeError(w, err)
		return
	}
	setSessionCookie(w, token, expires, auth.SecureCookie)
	s.loginLimiter.Success(loginKey)
	s.log("info", "auth.login", "login succeeded")
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"authEnabled":   true,
		"authenticated": true,
		"expiresAt":     expires.UTC().Format(time.RFC3339Nano),
	})
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	if remoteAddr == "" {
		return "unknown"
	}
	return remoteAddr
}

func writeLoginRateLimit(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := int64((retryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	writeErrorStatus(w, http.StatusTooManyRequests, fmt.Errorf("too many login attempts; try again later"))
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.Delete(cookie.Value)
	}
	secure := r.TLS != nil
	if auth, err := s.authConfig(r.Context()); err == nil {
		secure = auth.SecureCookie
	}
	clearSessionCookie(w, secure)
	s.log("info", "auth.logout", "session cleared")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRuntimeState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"state": s.runtime.State()})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	report := BuildStatsReport(cfg, s.runtime.State(), time.Now())
	writeJSON(w, http.StatusOK, report)
}

type systemInterfaceInfo struct {
	Name         string   `json:"name"`
	Index        int      `json:"index"`
	MTU          int      `json:"mtu"`
	HardwareAddr string   `json:"hardwareAddr,omitempty"`
	Flags        []string `json:"flags"`
	Up           bool     `json:"up"`
	Loopback     bool     `json:"loopback"`
}

func (s *Server) handleSystemInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	items := make([]systemInterfaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		flags := strings.Fields(iface.Flags.String())
		items = append(items, systemInterfaceInfo{
			Name:         iface.Name,
			Index:        iface.Index,
			MTU:          iface.MTU,
			HardwareAddr: iface.HardwareAddr.String(),
			Flags:        flags,
			Up:           iface.Flags&net.FlagUp != 0,
			Loopback:     iface.Flags&net.FlagLoopback != 0,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "obj": items})
}

func (s *Server) handleRuntimeApply(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	runtime, err := config.GenerateRuntime(cfg)
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := s.runtime.Apply(runtime, cfg)
	if err != nil {
		s.log("error", "runtime.apply", err.Error())
		writeRuntimeApplyError(w, err, state)
		return
	}
	s.log("info", "runtime.apply", fmt.Sprintf("applied generation %d", state.Generation))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": state})
}

func (s *Server) handleRuntimeEnforce(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	state, events, err := s.runtime.EnforceLimits(cfg, time.Now())
	if err != nil {
		s.log("error", "runtime.enforce", err.Error())
		writeError(w, err)
		return
	}
	if len(events) > 0 {
		s.log("warn", "runtime.enforce", fmt.Sprintf("closed %d client binding(s)", len(events)))
	} else {
		s.log("info", "runtime.enforce", "no client limits triggered")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": state, "events": events})
}

func (s *Server) handleRuntimeStop(w http.ResponseWriter, r *http.Request) {
	state, err := s.runtime.Stop()
	if err != nil {
		s.log("error", "runtime.stop", err.Error())
		writeError(w, err)
		return
	}
	s.log("info", "runtime.stop", "runtime stopped")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": state})
}

func (s *Server) handleRuntimeComponentAction(w http.ResponseWriter, r *http.Request) {
	component := strings.TrimSpace(r.PathValue("component"))
	switch component {
	case core.RuntimeComponentTapX, core.RuntimeComponentEmbeddedXray, core.RuntimeComponentExternalXray:
	default:
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("unsupported runtime component %q", component))
		return
	}
	action := strings.TrimSpace(r.PathValue("action"))
	var (
		state RuntimeState
		err   error
	)
	switch action {
	case "restart":
		state, err = s.runtime.RestartComponent(component)
	case "stop":
		state, err = s.runtime.StopComponent(component)
	default:
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("unsupported runtime component action %q", action))
		return
	}
	if err != nil {
		s.log("error", "runtime.component."+action, component+": "+err.Error())
		writeRuntimeApplyError(w, err, state)
		return
	}
	s.log("info", "runtime.component."+action, component)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state": state})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": "ok",
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.store.LoadConfig(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": cfg})
	case http.MethodPut:
		cfg, err := decodeConfig(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := s.store.ReplaceConfig(r.Context(), cfg); err != nil {
			s.log("error", "config.save", err.Error())
			writeError(w, err)
			return
		}
		s.log("info", "config.save", "configuration saved")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	cfg, err := decodeConfig(r)
	if err != nil {
		writeError(w, err)
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "save"
	}
	switch mode {
	case "save":
		err = config.ValidateForSave(cfg)
	case "apply":
		err = config.ValidateForApply(cfg)
	default:
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("unknown validation mode %q", mode))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "mode": mode})
}

func (s *Server) handleRuntime(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.store.LoadConfig(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		runtime, err := config.GenerateRuntime(cfg)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runtime": runtime})
	case http.MethodPost:
		cfg, err := decodeConfig(r)
		if err != nil {
			writeError(w, err)
			return
		}
		runtime, err := config.GenerateRuntime(cfg)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runtime": runtime})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleObjects(w http.ResponseWriter, r *http.Request) {
	kind, id, err := objectPath(strings.TrimPrefix(r.URL.Path, "/api/objects/"))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id == "" {
			items, err := s.store.ListObjects(r.Context(), kind)
			if err != nil {
				writeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "items": items})
			return
		}
		item, err := s.store.GetObject(r.Context(), kind, id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"kind": kind, "id": id, "item": item})
	case http.MethodPut:
		if id == "" {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("object id is required"))
			return
		}
		raw, err := readBody(r)
		if err != nil {
			writeError(w, err)
			return
		}
		cfg, err := s.store.UpsertObject(r.Context(), kind, id, raw)
		if err != nil {
			s.log("error", "object.upsert", fmt.Sprintf("%s/%s: %s", kind, id, err.Error()))
			writeError(w, err)
			return
		}
		s.log("info", "object.upsert", fmt.Sprintf("%s/%s saved", kind, id))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
	case http.MethodDelete:
		if id == "" {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("object id is required"))
			return
		}
		cfg, err := s.store.DeleteObject(r.Context(), kind, id)
		if err != nil {
			s.log("error", "object.delete", fmt.Sprintf("%s/%s: %s", kind, id, err.Error()))
			writeError(w, err)
			return
		}
		s.log("info", "object.delete", fmt.Sprintf("%s/%s deleted", kind, id))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": cfg})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	s.log("info", "backup.export", "database backup exported")
	file, size, cleanup, err := s.store.OpenDatabaseBackup(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	defer cleanup()
	filename := "tapx-backup-" + time.Now().UTC().Format("20060102-150405") + ".db"
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	path, cleanup, err := readBackupFile(r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer cleanup()
	if err := s.store.RestoreDatabaseFile(r.Context(), path); err != nil {
		s.log("error", "backup.restore", err.Error())
		writeError(w, err)
		return
	}
	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	persistedLogs, err := s.store.LoadLogs(r.Context(), defaultLogLimit)
	if err != nil {
		writeError(w, err)
		return
	}
	s.logs.Replace(persistedLogs)
	s.sessions.Clear()
	state, stopErr := s.runtime.Stop()
	s.dashboard.Reset()
	s.log("info", "backup.restore", "database restored from backup")
	warnings := make([]string, 0, 1)
	if stopErr != nil {
		warning := "configuration was restored, but the previous runtime could not be stopped: " + stopErr.Error()
		warnings = append(warnings, warning)
		s.log("error", "backup.restore.runtime-stop", warning)
	}
	s.restoreIntegrationSchedules()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"config":          cfg,
		"runtimeState":    state,
		"restartRequired": true,
		"warnings":        warnings,
	})
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"events": s.logs.List()})
	case http.MethodDelete:
		s.logs.Clear()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodDelete)
	}
}

func (s *Server) log(level, action, message string) {
	if s.logs == nil {
		return
	}
	s.logs.Add(level, action, message)
}

func objectPath(path string) (kind string, id string, err error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", fmt.Errorf("object kind is required")
	}
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return "", "", fmt.Errorf("object path must be /api/objects/{kind} or /api/objects/{kind}/{id}")
	}
	kind = parts[0]
	if !knownKind(kind) {
		return "", "", ErrUnknownKind
	}
	if len(parts) == 2 {
		id = parts[1]
	}
	return kind, id, nil
}

func decodeConfig(r *http.Request) (config.RuntimeConfig, error) {
	raw, err := readBody(r)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	var cfg config.RuntimeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return config.RuntimeConfig{}, err
	}
	return cfg, nil
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRequestBody {
		return nil, fmt.Errorf("request body is too large")
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}
	return raw, nil
}

func readBackupFile(r *http.Request) (string, func(), error) {
	defer r.Body.Close()
	if r.ContentLength > maxBackupBody {
		return "", nil, fmt.Errorf("backup database is too large")
	}
	file, err := os.CreateTemp("", "tapx-db-upload-*.db")
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(r.Body, maxBackupBody+1))
	closeErr := file.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", nil, closeErr
	}
	if written > maxBackupBody {
		cleanup()
		return "", nil, fmt.Errorf("backup database is too large")
	}
	if written == 0 {
		cleanup()
		return "", nil, fmt.Errorf("backup database is empty")
	}
	return path, cleanup, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	writeErrorStatus(w, errorStatus(err), err)
}

func errorStatus(err error) int {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrUnknownKind), errors.Is(err, ErrIDMismatch):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case config.IsValidationError(err):
		status = http.StatusUnprocessableEntity
	}
	return status
}

func writeErrorStatus(w http.ResponseWriter, status int, err error) {
	payload := map[string]any{
		"ok":    false,
		"error": err.Error(),
	}
	var validation *config.ValidationError
	if errors.As(err, &validation) {
		payload["problems"] = validation.Problems
	}
	writeJSON(w, status, payload)
}

func writeRuntimeApplyError(w http.ResponseWriter, err error, state RuntimeState) {
	payload := map[string]any{
		"ok":    false,
		"error": err.Error(),
		"state": state,
	}
	writeJSON(w, errorStatus(err), payload)
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeErrorStatus(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func subtleString(a, b string) int {
	if len(a) != len(b) {
		return 0
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b))
}

func modelTransport(value string) model.Transport {
	return model.Transport(strings.ToLower(strings.TrimSpace(value)))
}

func queryUint16(value string) (uint16, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(parsed), nil
}

func queryInt(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}
