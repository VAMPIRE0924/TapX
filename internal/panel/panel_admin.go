package panel

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"tapx/internal/model"
)

type adminCredentialsRequest struct {
	OldUsername string
	OldPassword string
	NewUsername string
	NewPassword string
}

func (s *Server) handleAdminCredentials(w http.ResponseWriter, r *http.Request) {
	var request adminCredentialsRequest
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	request.NewUsername = strings.TrimSpace(request.NewUsername)
	if request.NewUsername == "" || request.NewPassword == "" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("new username and password are required"))
		return
	}

	cfg, err := s.store.LoadConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	index := -1
	for i := range cfg.Settings {
		if cfg.Settings[i].Enabled {
			index = i
			break
		}
	}
	if index < 0 {
		cfg.Settings = append(cfg.Settings, model.Settings{ID: "global", Enabled: true, Name: "TapX"})
		index = len(cfg.Settings) - 1
	}
	settings := cfg.Settings[index]
	if settings.PanelAuthEnabled {
		if subtleStringCompare(settings.AdminUsername, strings.TrimSpace(request.OldUsername)) == 0 ||
			!VerifyPanelPassword(settings.AdminPasswordHash, request.OldPassword) {
			writeErrorStatus(w, http.StatusUnauthorized, fmt.Errorf("current administrator credentials are invalid"))
			return
		}
	}
	hash, err := HashPanelPassword(request.NewPassword)
	if err != nil {
		writeError(w, err)
		return
	}
	settings.PanelAuthEnabled = true
	settings.AdminUsername = request.NewUsername
	settings.AdminPasswordHash = hash
	if settings.SessionTTLSecond <= 0 {
		settings.SessionTTLSecond = defaultSessionTTLSecond
	}
	cfg.Settings[index] = settings
	if err := s.store.ReplaceConfig(r.Context(), cfg); err != nil {
		writeError(w, err)
		return
	}
	s.sessions.Clear()
	token, expires, err := s.sessions.Create(sessionTTL(authConfigFromSettings(settings)))
	if err != nil {
		writeError(w, err)
		return
	}
	setSessionCookie(w, token, expires, settings.PanelHTTPS)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handlePanelRestart(w http.ResponseWriter, _ *http.Request) {
	if s.restart == nil {
		writeErrorStatus(w, http.StatusNotImplemented, fmt.Errorf("panel restart is unavailable without a service supervisor"))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "restarting": true})
	time.AfterFunc(150*time.Millisecond, func() {
		_ = s.restart()
	})
}

func subtleStringCompare(left, right string) int {
	if len(left) != len(right) {
		return 0
	}
	var diff byte
	for i := range left {
		diff |= left[i] ^ right[i]
	}
	if diff == 0 {
		return 1
	}
	return 0
}
