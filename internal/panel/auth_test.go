package panel

import "testing"

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
