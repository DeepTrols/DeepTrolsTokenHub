package totp

import (
	"strings"
	"testing"
)

func TestGenerateSecret_Length(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(secret) != 32 {
		t.Errorf("secret length = %d, want 32", len(secret))
	}
}

func TestGenerateSecret_Unique(t *testing.T) {
	secrets := make(map[string]bool)
	for i := 0; i < 10; i++ {
		s, err := GenerateSecret()
		if err != nil {
			t.Fatalf("GenerateSecret: %v", err)
		}
		if secrets[s] {
			t.Error("duplicate secret generated")
		}
		secrets[s] = true
	}
}

func TestGenerateKeyURI(t *testing.T) {
	uri := GenerateKeyURI("JBSWY3DPEHPK3PXP", "user@example.com", "DeepTrols")
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Errorf("URI should start with otpauth://totp/, got: %s", uri)
	}
	if !strings.Contains(uri, "DeepTrols") {
		t.Error("URI should contain issuer")
	}
	if !strings.Contains(uri, "user@example.com") && !strings.Contains(uri, "user%40example.com") {
		t.Error("URI should contain user email (raw or encoded)")
	}
	if !strings.Contains(uri, "secret=JBSWY3DPEHPK3PXP") {
		t.Error("URI should contain secret")
	}
}

func TestValidate_WrongCode(t *testing.T) {
	secret, _ := GenerateSecret()
	valid, err := Validate(secret, "000000", 1)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if valid {
		t.Error("random code should not validate")
	}
}

func TestValidate_InvalidSecret(t *testing.T) {
	_, err := Validate("!!!invalid-base32!!!", "123456", 1)
	if err == nil {
		t.Error("expected error for invalid base32 secret")
	}
}
