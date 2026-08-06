package encrypt

import (
	"crypto/rand"
	"testing"
)

func makeKey() []byte {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return key
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := makeKey()
	plaintext := "hello, world!"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if ciphertext == "" {
		t.Fatal("ciphertext is empty")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_EmptyString(t *testing.T) {
	key := makeKey()
	ciphertext, err := Encrypt("", key)
	if err != nil {
		t.Fatalf("encrypt empty string: %v", err)
	}
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}
	if decrypted != "" {
		t.Errorf("decrypted = %q, want empty", decrypted)
	}
}

func TestEncryptDecrypt_Unicode(t *testing.T) {
	key := makeKey()
	plaintext := "你好世界 🌍"

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_InvalidKeySize(t *testing.T) {
	_, err := Encrypt("test", []byte("too-short"))
	if err == nil {
		t.Error("expected error for invalid key size")
	}
}

func TestDecrypt_InvalidKeySize(t *testing.T) {
	key := makeKey()
	ciphertext, _ := Encrypt("test", key)
	_, err := Decrypt(ciphertext, []byte("too-short-key-that-is-wrong"))
	if err == nil {
		t.Error("expected error for invalid key size on decrypt")
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key1 := makeKey()
	key2 := makeKey()
	ciphertext, _ := Encrypt("secret", key1)
	_, err := Decrypt(ciphertext, key2)
	if err == nil {
		t.Error("expected error with wrong key")
	}
}

func TestDecrypt_InvalidHex(t *testing.T) {
	key := makeKey()
	_, err := Decrypt("not-hex-data!!!", key)
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	key := makeKey()
	_, err := Decrypt("aabb", key)
	if err == nil {
		t.Error("expected error for too-short ciphertext")
	}
}

func TestEncrypt_DeterministicFailure(t *testing.T) {
	key := makeKey()
	c1, _ := Encrypt("same", key)
	c2, _ := Encrypt("same", key)
	if c1 == c2 {
		t.Error("two encryptions of same plaintext should differ (random nonce)")
	}
}
