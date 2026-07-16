package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPanelPasswordHashVerify(t *testing.T) {
	hash, err := HashPanelPassword("secret")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := ValidatePanelPasswordHash(hash); err != nil {
		t.Fatalf("validate password hash: %v", err)
	}
	if !VerifyPanelPassword(hash, "secret") {
		t.Fatalf("expected password to verify")
	}
	if VerifyPanelPassword(hash, "wrong") {
		t.Fatalf("wrong password verified")
	}
}

func TestPanelPasswordHashRejectsUnboundedWorkFactors(t *testing.T) {
	tests := []string{
		"pbkdf2-sha256$1000001$MTIzNDU2Nzg$MTIzNDU2Nzg5MDEyMzQ1Ng",
		"pbkdf2-sha256$120000$MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDEyMzQ1Njc4OTA$MTIzNDU2Nzg5MDEyMzQ1Ng",
	}
	for _, encoded := range tests {
		if VerifyPanelPassword(encoded, "secret") {
			t.Fatalf("accepted unbounded password hash %q", encoded)
		}
		if err := ValidatePanelPasswordHash(encoded); err == nil {
			t.Fatalf("validated unbounded password hash %q", encoded)
		}
	}
}

func TestSessionManagerPrunesExpiredAndCapsActiveSessions(t *testing.T) {
	now := time.Unix(1000, 0)
	manager := NewSessionManager()
	manager.now = func() time.Time { return now }

	expired, _, err := manager.Create(time.Second)
	if err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	now = now.Add(2 * time.Second)
	for i := 0; i < maxActiveSessions+1; i++ {
		if _, _, err := manager.Create(time.Hour + time.Duration(i)); err != nil {
			t.Fatalf("create active session %d: %v", i, err)
		}
	}
	if manager.Valid(expired) {
		t.Fatal("expired session remained valid")
	}
	if got := len(manager.tokens); got != maxActiveSessions {
		t.Fatalf("active session count = %d, want %d", got, maxActiveSessions)
	}
}

func TestLoginLimiterBlocksAndResets(t *testing.T) {
	now := time.Unix(2000, 0)
	limiter := NewLoginLimiter()
	limiter.now = func() time.Time { return now }
	for i := 1; i < loginFailureLimit; i++ {
		if retry := limiter.Failure("192.0.2.1"); retry != 0 {
			t.Fatalf("failure %d blocked early for %s", i, retry)
		}
	}
	if retry := limiter.Failure("192.0.2.1"); retry != loginBlockDuration {
		t.Fatalf("block duration = %s, want %s", retry, loginBlockDuration)
	}
	if retry := limiter.RetryAfter("192.0.2.1"); retry != loginBlockDuration {
		t.Fatalf("retry duration = %s, want %s", retry, loginBlockDuration)
	}
	limiter.Success("192.0.2.1")
	if retry := limiter.RetryAfter("192.0.2.1"); retry != 0 {
		t.Fatalf("successful login did not reset limiter: %s", retry)
	}
}

func TestAuthBypassProtectsNativeAndCompatibilityAPIs(t *testing.T) {
	tests := []struct {
		path   string
		bypass bool
	}{
		{path: "/api/health", bypass: true},
		{path: "/api/auth/login", bypass: true},
		{path: "/api/config", bypass: false},
		{path: "/panel/api/server/interfaces", bypass: false},
		{path: "/panel/api/server/getNewX25519Cert", bypass: false},
		{path: "/panel/", bypass: true},
		{path: "/assets/panel.js", bypass: true},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest("GET", test.path, nil)
			if got := authBypass(request); got != test.bypass {
				t.Fatalf("authBypass(%q) = %t, want %t", test.path, got, test.bypass)
			}
		})
	}
}

func TestSameOriginSessionRequest(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		host      string
		origin    string
		fetchSite string
		allowed   bool
	}{
		{name: "safe method", method: http.MethodGet, host: "panel.example", origin: "https://other.example", allowed: true},
		{name: "same origin", method: http.MethodPost, host: "panel.example:443", origin: "https://panel.example:443", allowed: true},
		{name: "cross origin", method: http.MethodPost, host: "panel.example", origin: "https://other.example", allowed: false},
		{name: "explicit cross site", method: http.MethodPut, host: "panel.example", fetchSite: "cross-site", allowed: false},
		{name: "non browser client", method: http.MethodDelete, host: "panel.example", allowed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://"+test.host+"/api/config", nil)
			request.Host = test.host
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", test.fetchSite)
			if got := sameOriginSessionRequest(request); got != test.allowed {
				t.Fatalf("sameOriginSessionRequest() = %t, want %t", got, test.allowed)
			}
		})
	}
}

func TestClearSessionCookieMatchesTransportSecurity(t *testing.T) {
	for _, secure := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		clearSessionCookie(recorder, secure)
		response := recorder.Result()
		cookies := response.Cookies()
		if len(cookies) != 1 {
			t.Fatalf("secure=%t cookie count = %d, want 1", secure, len(cookies))
		}
		cookie := cookies[0]
		if cookie.Name != sessionCookieName || cookie.MaxAge != -1 || cookie.Value != "" {
			t.Fatalf("secure=%t invalid clearing cookie: %#v", secure, cookie)
		}
		if cookie.Secure != secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("secure=%t invalid cookie security attributes: %#v", secure, cookie)
		}
	}
}
