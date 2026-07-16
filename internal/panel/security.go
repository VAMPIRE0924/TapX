package panel

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const panelSecurityIntegration = "panel-security"

type panelSecurityState struct {
	TOTPSecret string          `json:"totpSecret,omitempty"`
	APITokens  []panelAPIToken `json:"apiTokens,omitempty"`
}

type panelAPIToken struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

type panelAPITokenView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	CreatedAt string `json:"createdAt"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

func (s *Server) loadPanelSecurity(ctx context.Context) (panelSecurityState, error) {
	raw, err := s.store.GetIntegration(ctx, panelSecurityIntegration)
	if errors.Is(err, ErrNotFound) {
		return panelSecurityState{}, nil
	}
	if err != nil {
		return panelSecurityState{}, err
	}
	var state panelSecurityState
	if err := json.Unmarshal(raw, &state); err != nil {
		return panelSecurityState{}, fmt.Errorf("decode panel security: %w", err)
	}
	return state, nil
}

func (s *Server) savePanelSecurity(ctx context.Context, state panelSecurityState) error {
	return s.store.SetIntegration(ctx, panelSecurityIntegration, state)
}

func (s *Server) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	state, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	now := time.Now()
	tokens := make([]panelAPITokenView, 0, len(state.APITokens))
	for _, token := range state.APITokens {
		if tokenExpired(token, now) {
			continue
		}
		tokens = append(tokens, panelAPITokenView{
			ID: token.ID, Name: token.Name, Prefix: token.Prefix,
			CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"twoFactorEnabled": state.TOTPSecret != "",
		"apiTokens":        tokens,
	})
}

func (s *Server) handleTOTPPrepare(w http.ResponseWriter, r *http.Request) {
	secret, err := generateTOTPSecret()
	if err != nil {
		writeError(w, err)
		return
	}
	auth, err := s.authConfig(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	account := auth.Username
	if account == "" {
		account = "admin"
	}
	issuer := "TapX-UI"
	uri := "otpauth://totp/" + url.PathEscape(issuer+":"+account) +
		"?secret=" + url.QueryEscape(secret) + "&issuer=" + url.QueryEscape(issuer) +
		"&algorithm=SHA1&digits=6&period=30"
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "uri": uri})
}

type totpEnableRequest struct {
	Secret string
	Code   string
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	var request totpEnableRequest
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	request.Secret = normalizeTOTPSecret(request.Secret)
	if !verifyTOTP(request.Secret, request.Code, time.Now()) {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("invalid verification code"))
		return
	}
	state, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	state.TOTPSecret = request.Secret
	if err := s.savePanelSecurity(r.Context(), state); err != nil {
		writeError(w, err)
		return
	}
	s.sessions.Clear()
	s.log("info", "security.totp.enable", "two-factor authentication enabled")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type totpDisableRequest struct {
	Code string
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	var request totpDisableRequest
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	state, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if state.TOTPSecret == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if !verifyTOTP(state.TOTPSecret, request.Code, time.Now()) {
		writeErrorStatus(w, http.StatusUnauthorized, fmt.Errorf("invalid verification code"))
		return
	}
	state.TOTPSecret = ""
	if err := s.savePanelSecurity(r.Context(), state); err != nil {
		writeError(w, err)
		return
	}
	s.sessions.Clear()
	s.log("info", "security.totp.disable", "two-factor authentication disabled")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type apiTokenCreateRequest struct {
	Name      string
	ExpiresAt string
}

func (s *Server) handleAPITokens(w http.ResponseWriter, r *http.Request) {
	var request apiTokenCreateRequest
	if err := decodeSmallJSON(r, &request); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, err)
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" {
		writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("token name is required"))
		return
	}
	if request.ExpiresAt != "" {
		expires, err := time.Parse(time.RFC3339, request.ExpiresAt)
		if err != nil || !expires.After(time.Now()) {
			writeErrorStatus(w, http.StatusBadRequest, fmt.Errorf("token expiry must be a future RFC3339 time"))
			return
		}
	}
	rawToken, err := randomURLToken(32)
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := randomHex(8)
	if err != nil {
		writeError(w, err)
		return
	}
	state, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := panelAPIToken{
		ID: id, Name: request.Name, Prefix: tokenPrefix(rawToken), Hash: hashAPIToken(rawToken),
		CreatedAt: now, ExpiresAt: request.ExpiresAt,
	}
	state.APITokens = append(state.APITokens, item)
	if err := s.savePanelSecurity(r.Context(), state); err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "security.token.create", fmt.Sprintf("API token %s created", request.Name))
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":    true,
		"token": rawToken,
		"item":  panelAPITokenView{ID: item.ID, Name: item.Name, Prefix: item.Prefix, CreatedAt: item.CreatedAt, ExpiresAt: item.ExpiresAt},
	})
}

func (s *Server) handleAPITokenDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	state, err := s.loadPanelSecurity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	next := state.APITokens[:0]
	found := false
	for _, token := range state.APITokens {
		if token.ID == id {
			found = true
			continue
		}
		next = append(next, token)
	}
	if !found {
		writeErrorStatus(w, http.StatusNotFound, fmt.Errorf("API token not found"))
		return
	}
	state.APITokens = next
	if err := s.savePanelSecurity(r.Context(), state); err != nil {
		writeError(w, err)
		return
	}
	s.log("info", "security.token.delete", fmt.Sprintf("API token %s deleted", id))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiTokenAuthenticated(ctx context.Context, r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
		return false
	}
	candidate := strings.TrimSpace(header[7:])
	if candidate == "" {
		return false
	}
	state, err := s.loadPanelSecurity(ctx)
	if err != nil {
		return false
	}
	candidateHash, err := hex.DecodeString(hashAPIToken(candidate))
	if err != nil {
		return false
	}
	now := time.Now()
	for _, token := range state.APITokens {
		if tokenExpired(token, now) {
			continue
		}
		expected, err := hex.DecodeString(token.Hash)
		if err == nil && len(expected) == len(candidateHash) && subtle.ConstantTimeCompare(candidateHash, expected) == 1 {
			return true
		}
	}
	return false
}

func tokenExpired(token panelAPIToken, now time.Time) bool {
	if token.ExpiresAt == "" {
		return false
	}
	expires, err := time.Parse(time.RFC3339, token.ExpiresAt)
	return err != nil || !expires.After(now)
}

func generateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="), nil
}

func normalizeTOTPSecret(secret string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
}

func verifyTOTP(secret, code string, now time.Time) bool {
	secret = normalizeTOTPSecret(secret)
	code = strings.TrimSpace(code)
	if secret == "" || len(code) != 6 {
		return false
	}
	for offset := int64(-1); offset <= 1; offset++ {
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, now.Unix()/30+offset)), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func totpCode(secret string, counter int64) string {
	if counter < 0 {
		return ""
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(normalizeTOTPSecret(secret))
	if err != nil {
		return ""
	}
	var value [8]byte
	binary.BigEndian.PutUint64(value[:], uint64(counter))
	mac := hmac.New(sha1.New, decoded)
	_, _ = mac.Write(value[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0f
	binaryCode := (uint32(digest[offset])&0x7f)<<24 |
		uint32(digest[offset+1])<<16 |
		uint32(digest[offset+2])<<8 |
		uint32(digest[offset+3])
	return fmt.Sprintf("%06d", binaryCode%1000000)
}

func randomURLToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func randomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func hashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}
