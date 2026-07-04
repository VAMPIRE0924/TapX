package panel

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"tapx/internal/config"
	"tapx/internal/model"
)

const (
	sessionCookieName       = "tapx_session"
	defaultSessionTTLSecond = 86400
	passwordHashPrefix      = "pbkdf2-sha256"
	passwordHashIterations  = 120000
	passwordSaltBytes       = 16
	passwordKeyBytes        = 32
)

type panelAuthConfig struct {
	Enabled          bool
	Username         string
	PasswordHash     string
	SessionTTLSecond int
	SecureCookie     bool
}

type SessionManager struct {
	mu     sync.Mutex
	now    func() time.Time
	tokens map[string]time.Time
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		now:    time.Now,
		tokens: make(map[string]time.Time),
	}
}

func (m *SessionManager) Create(ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = time.Duration(defaultSessionTTLSecond) * time.Second
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := m.now().Add(ttl)

	m.mu.Lock()
	m.tokens[token] = expires
	m.mu.Unlock()
	return token, expires, nil
}

func (m *SessionManager) Valid(token string) bool {
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	expires, ok := m.tokens[token]
	if !ok {
		return false
	}
	if !expires.After(m.now()) {
		delete(m.tokens, token)
		return false
	}
	return true
}

func (m *SessionManager) Delete(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.tokens, token)
	m.mu.Unlock()
}

func HashPanelPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password is required")
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(password), salt, passwordHashIterations, passwordKeyBytes)
	return strings.Join([]string{
		passwordHashPrefix,
		strconv.Itoa(passwordHashIterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

func VerifyPanelPassword(encoded, password string) bool {
	iterations, salt, expected, ok := parsePanelPasswordHash(encoded)
	if !ok || password == "" {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(got, expected) == 1
}

func ValidatePanelPasswordHash(encoded string) error {
	if _, _, _, ok := parsePanelPasswordHash(encoded); !ok {
		return fmt.Errorf("must be %s$iterations$salt$hash", passwordHashPrefix)
	}
	return nil
}

func parsePanelPasswordHash(encoded string) (int, []byte, []byte, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashPrefix {
		return 0, nil, nil, false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 {
		return 0, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 8 {
		return 0, nil, nil, false
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(key) < 16 {
		return 0, nil, nil, false
	}
	return iterations, salt, key, true
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hashLen := sha256.Size
	blocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, blocks*hashLen)
	for block := 1; block <= blocks; block++ {
		out = append(out, pbkdf2Block(password, salt, iterations, uint32(block))...)
	}
	return out[:keyLen]
}

func pbkdf2Block(password, salt []byte, iterations int, block uint32) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	var counter [4]byte
	binary.BigEndian.PutUint32(counter[:], block)
	mac.Write(counter[:])
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}

func (s *Server) authConfig(ctx context.Context) (panelAuthConfig, error) {
	cfg, err := s.store.LoadConfig(ctx)
	if err != nil {
		return panelAuthConfig{}, err
	}
	return authConfigFromRuntimeConfig(cfg), nil
}

func authConfigFromRuntimeConfig(cfg config.RuntimeConfig) panelAuthConfig {
	for _, item := range cfg.Settings {
		if !item.Enabled || !item.PanelAuthEnabled {
			continue
		}
		ttl := item.SessionTTLSecond
		if ttl == 0 {
			ttl = defaultSessionTTLSecond
		}
		return panelAuthConfig{
			Enabled:          true,
			Username:         item.AdminUsername,
			PasswordHash:     item.AdminPasswordHash,
			SessionTTLSecond: ttl,
			SecureCookie:     item.PanelHTTPS,
		}
	}
	return panelAuthConfig{}
}

func authConfigFromSettings(item model.Settings) panelAuthConfig {
	return authConfigFromRuntimeConfig(config.RuntimeConfig{Settings: []model.Settings{item}})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authBypass(r) {
			next.ServeHTTP(w, r)
			return
		}
		auth, err := s.authConfig(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		if !auth.Enabled || s.authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeErrorStatus(w, http.StatusUnauthorized, fmt.Errorf("panel login required"))
	})
}

func authBypass(r *http.Request) bool {
	if r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/api/auth/") {
		return true
	}
	return !strings.HasPrefix(r.URL.Path, "/api/")
}

func (s *Server) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return s.sessions.Valid(cookie.Value)
}

func sessionTTL(auth panelAuthConfig) time.Duration {
	if auth.SessionTTLSecond <= 0 {
		return time.Duration(defaultSessionTTLSecond) * time.Second
	}
	return time.Duration(auth.SessionTTLSecond) * time.Second
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
