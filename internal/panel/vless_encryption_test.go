package panel

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateVlessEncryptionAuths(t *testing.T) {
	auths, err := generateVlessEncryptionAuths()
	if err != nil {
		t.Fatal(err)
	}
	if len(auths) != 6 {
		t.Fatalf("auth count = %d, want 6", len(auths))
	}

	byID := make(map[string]vlessEncryptionAuth, len(auths))
	for _, auth := range auths {
		byID[auth.ID] = auth
		if !strings.HasPrefix(auth.Decryption, "mlkem768x25519plus.") || !strings.Contains(auth.Decryption, ".600s.") {
			t.Fatalf("invalid decryption for %s: %q", auth.ID, auth.Decryption)
		}
		if !strings.HasPrefix(auth.Encryption, "mlkem768x25519plus.") || !strings.Contains(auth.Encryption, ".0rtt.") {
			t.Fatalf("invalid encryption for %s: %q", auth.ID, auth.Encryption)
		}
	}

	wantIDs := []string{"x25519", "mlkem768", "x25519_xorpub", "x25519_random", "mlkem768_xorpub", "mlkem768_random"}
	for _, id := range wantIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing auth %q: %+v", id, auths)
		}
	}
	assertVlessKeySize(t, byID["x25519"].Decryption, 32)
	assertVlessKeySize(t, byID["x25519"].Encryption, 32)
	assertVlessKeySize(t, byID["mlkem768"].Decryption, 64)
	assertVlessKeySize(t, byID["mlkem768"].Encryption, 1184)
}

func TestServerVlessEncryptionAPI(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)
	for _, path := range []string{"/api/xray/vless-encryption", "/panel/api/server/getNewVlessEnc"} {
		resp := getJSON(t, server.URL+path, http.StatusOK)
		if resp["success"] != true {
			t.Fatalf("GET %s missing success: %+v", path, resp)
		}
		obj, ok := resp["obj"].(map[string]any)
		if !ok || len(obj["auths"].([]any)) != 6 {
			t.Fatalf("GET %s returned invalid auths: %+v", path, resp)
		}
	}
}

func assertVlessKeySize(t *testing.T, value string, want int) {
	t.Helper()
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		t.Fatalf("VLESS encryption value has %d parts: %q", len(parts), value)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatalf("decode VLESS key: %v", err)
	}
	if len(decoded) != want {
		t.Fatalf("VLESS key length = %d, want %d", len(decoded), want)
	}
}
