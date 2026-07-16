package panel

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
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
	maxActiveSessions       = 1024
	loginFailureLimit       = 10
	loginFailureWindow      = 5 * time.Minute
	loginBlockDuration      = 5 * time.Minute
	maxLoginAttemptKeys     = 4096
	passwordHashPrefix      = "pbkdf2-sha256"
	passwordHashIterations  = 120000
	maxPasswordIterations   = 1000000
	passwordSaltBytes       = 16
	passwordKeyBytes        = 32
	maxPasswordSaltBytes    = 64
	maxPasswordKeyBytes     = 64
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
	m.pruneLocked()
	if len(m.tokens) >= maxActiveSessions {
		m.deleteEarliestLocked()
	}
	m.tokens[token] = expires
	m.mu.Unlock()
	return token, expires, nil
}

func (m *SessionManager) pruneLocked() {
	now := m.now()
	for token, expires := range m.tokens {
		if !expires.After(now) {
			delete(m.tokens, token)
		}
	}
}

func (m *SessionManager) deleteEarliestLocked() {
	var earliestToken string
	var earliestExpiry time.Time
	for token, expires := range m.tokens {
		if earliestToken == "" || expires.Before(earliestExpiry) {
			earliestToken = token
			earliestExpiry = expires
		}
	}
	delete(m.tokens, earliestToken)
}

type loginAttempt struct {
	failures     int
	windowStart  time.Time
	blockedUntil time.Time
	lastSeen     time.Time
}

type LoginLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	attempts map[string]loginAttempt
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{now: time.Now, attempts: make(map[string]loginAttempt)}
}

func (l *LoginLimiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	attempt, ok := l.attempts[key]
	if !ok || !attempt.blockedUntil.After(now) {
		return 0
	}
	return attempt.blockedUntil.Sub(now)
}

func (l *LoginLimiter) Failure(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	attempt := l.attempts[key]
	if attempt.windowStart.IsZero() || now.Sub(attempt.windowStart) >= loginFailureWindow {
		attempt.failures = 0
		attempt.windowStart = now
	}
	attempt.failures++
	attempt.lastSeen = now
	if attempt.failures >= loginFailureLimit {
		attempt.blockedUntil = now.Add(loginBlockDuration)
	}
	l.attempts[key] = attempt
	if len(l.attempts) > maxLoginAttemptKeys {
		l.deleteOldestLocked()
	}
	if attempt.blockedUntil.After(now) {
		return attempt.blockedUntil.Sub(now)
	}
	return 0
}

func (l *LoginLimiter) Success(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (l *LoginLimiter) pruneLocked(now time.Time) {
	for key, attempt := range l.attempts {
		windowExpired := now.Sub(attempt.windowStart) >= loginFailureWindow
		blockExpired := !attempt.blockedUntil.After(now)
		if windowExpired && blockExpired {
			delete(l.attempts, key)
		}
	}
}

func (l *LoginLimiter) deleteOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	for key, attempt := range l.attempts {
		if oldestKey == "" || attempt.lastSeen.Before(oldestTime) {
			oldestKey = key
			oldestTime = attempt.lastSeen
		}
	}
	delete(l.attempts, oldestKey)
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

func (m *SessionManager) Clear() {
	m.mu.Lock()
	clear(m.tokens)
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
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordHashIterations, passwordKeyBytes)
	if err != nil {
		return "", err
	}
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
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	if err != nil {
		return false
	}
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
	if err != nil || iterations < 10000 || iterations > maxPasswordIterations {
		return 0, nil, nil, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 8 || len(salt) > maxPasswordSaltBytes {
		return 0, nil, nil, false
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(key) < 16 || len(key) > maxPasswordKeyBytes {
		return 0, nil, nil, false
	}
	return iterations, salt, key, true
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
		if !auth.Enabled {
			next.ServeHTTP(w, r)
			return
		}
		if s.authenticated(r) {
			if !sameOriginSessionRequest(r) {
				writeErrorStatus(w, http.StatusForbidden, fmt.Errorf("cross-site session request rejected"))
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if s.apiTokenAuthenticated(r.Context(), r) {
			next.ServeHTTP(w, r)
			return
		}
		writeErrorStatus(w, http.StatusUnauthorized, fmt.Errorf("panel login required"))
	})
}

func sameOriginSessionRequest(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")), "cross-site") {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host != "" && strings.EqualFold(parsed.Host, r.Host)
}

func authBypass(r *http.Request) bool {
	if r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/api/auth/") {
		return true
	}
	return !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/panel/api/")
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

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
