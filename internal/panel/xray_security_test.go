package panel

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

func TestServerXraySecurityGenerators(t *testing.T) {
	store := newTestStore(t)
	server := httptest.NewServer(NewServer(store).Handler())
	t.Cleanup(server.Close)

	x25519 := getJSON(t, server.URL+"/api/xray/reality/x25519", http.StatusOK)["obj"].(map[string]any)
	privateBytes := decodeRawURL(t, x25519["privateKey"].(string))
	publicBytes := decodeRawURL(t, x25519["publicKey"].(string))
	if len(privateBytes) != 32 || len(publicBytes) != 32 {
		t.Fatalf("unexpected X25519 key sizes: private=%d public=%d", len(privateBytes), len(publicBytes))
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatal(err)
	}
	if string(privateKey.PublicKey().Bytes()) != string(publicBytes) {
		t.Fatal("X25519 public key does not match private key")
	}

	mldsa := getJSON(t, server.URL+"/api/xray/reality/mldsa65", http.StatusOK)["obj"].(map[string]any)
	if got := len(decodeRawURL(t, mldsa["seed"].(string))); got != mldsa65.SeedSize {
		t.Fatalf("ML-DSA-65 seed size = %d, want %d", got, mldsa65.SeedSize)
	}
	if got := len(decodeRawURL(t, mldsa["verify"].(string))); got != mldsa65.PublicKeySize {
		t.Fatalf("ML-DSA-65 verify size = %d, want %d", got, mldsa65.PublicKeySize)
	}
}

func TestGenerateECHProducesXrayWireFormat(t *testing.T) {
	serverKeysText, configListText, err := generateECH("example.com")
	if err != nil {
		t.Fatal(err)
	}
	serverKeys, err := base64.StdEncoding.DecodeString(serverKeysText)
	if err != nil {
		t.Fatal(err)
	}
	configList, err := base64.StdEncoding.DecodeString(configListText)
	if err != nil {
		t.Fatal(err)
	}
	if len(serverKeys) < 36 || binary.BigEndian.Uint16(serverKeys[:2]) != 32 {
		t.Fatalf("invalid ECH server key envelope: %x", serverKeys)
	}
	configLength := int(binary.BigEndian.Uint16(configList[:2]))
	if configLength != len(configList)-2 || binary.BigEndian.Uint16(configList[2:4]) != 0xfe0d {
		t.Fatalf("invalid ECH config list envelope: %x", configList)
	}
}

func TestGenerateECHRejectsInvalidServerNames(t *testing.T) {
	for _, name := range []string{"", strings.Repeat("a", 256), "bad/name"} {
		if _, _, err := generateECH(name); err == nil {
			t.Fatalf("generateECH(%q) accepted invalid server name", name)
		}
	}
}

func TestCertificateHashesUsesCertificateDER(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		DNSNames:     []string{"example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, privateKey.Public(), privateKey)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := certificateHashes(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 1 {
		t.Fatalf("hash count = %d, want 1", len(hashes))
	}
	decoded, err := hex.DecodeString(hashes[0])
	if err != nil || len(decoded) != 32 {
		t.Fatalf("invalid SHA-256 hash %q: %v", hashes[0], err)
	}
}

func decodeRawURL(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
