package panel

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

type vlessEncryptionAuth struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Decryption string `json:"decryption"`
	Encryption string `json:"encryption"`
}

func (s *Server) handleVlessEncryption(w http.ResponseWriter, _ *http.Request) {
	auths, err := generateVlessEncryptionAuths()
	if err != nil {
		writeErrorStatus(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"obj":     map[string]any{"auths": auths},
	})
}

func generateVlessEncryptionAuths() ([]vlessEncryptionAuth, error) {
	x25519, err := generateX25519VlessAuth()
	if err != nil {
		return nil, err
	}
	mlkem768, err := generateMLKEM768VlessAuth()
	if err != nil {
		return nil, err
	}

	base := []vlessEncryptionAuth{x25519, mlkem768}
	auths := append([]vlessEncryptionAuth(nil), base...)
	for _, auth := range base {
		for _, mode := range []string{"xorpub", "random"} {
			auths = append(auths, vlessEncryptionAuth{
				ID:         auth.ID + "_" + mode,
				Label:      auth.Label + " (" + mode + ")",
				Decryption: replaceVlessMode(auth.Decryption, mode),
				Encryption: replaceVlessMode(auth.Encryption, mode),
			})
		}
	}
	return auths, nil
}

func generateX25519VlessAuth() (vlessEncryptionAuth, error) {
	privateBytes := make([]byte, 32)
	if _, err := rand.Read(privateBytes); err != nil {
		return vlessEncryptionAuth{}, fmt.Errorf("generate X25519 private key: %w", err)
	}
	privateBytes[0] &= 248
	privateBytes[31] &= 127
	privateBytes[31] |= 64
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return vlessEncryptionAuth{}, fmt.Errorf("load X25519 private key: %w", err)
	}
	return newVlessEncryptionAuth(
		"x25519",
		"X25519, not Post-Quantum",
		privateKey.Bytes(),
		privateKey.PublicKey().Bytes(),
	), nil
}

func generateMLKEM768VlessAuth() (vlessEncryptionAuth, error) {
	seed := make([]byte, 64)
	if _, err := rand.Read(seed); err != nil {
		return vlessEncryptionAuth{}, fmt.Errorf("generate ML-KEM-768 seed: %w", err)
	}
	privateKey, err := mlkem.NewDecapsulationKey768(seed)
	if err != nil {
		return vlessEncryptionAuth{}, fmt.Errorf("load ML-KEM-768 seed: %w", err)
	}
	return newVlessEncryptionAuth(
		"mlkem768",
		"ML-KEM-768, Post-Quantum",
		seed,
		privateKey.EncapsulationKey().Bytes(),
	), nil
}

func newVlessEncryptionAuth(id, label string, serverKey, clientKey []byte) vlessEncryptionAuth {
	return vlessEncryptionAuth{
		ID:         id,
		Label:      label,
		Decryption: strings.Join([]string{"mlkem768x25519plus", "native", "600s", base64.RawURLEncoding.EncodeToString(serverKey)}, "."),
		Encryption: strings.Join([]string{"mlkem768x25519plus", "native", "0rtt", base64.RawURLEncoding.EncodeToString(clientKey)}, "."),
	}
}

func replaceVlessMode(value, mode string) string {
	return strings.Replace(value, ".native.", "."+mode+".", 1)
}
