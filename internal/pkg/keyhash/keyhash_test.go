package keyhash

import "testing"

func TestHash_Deterministic(t *testing.T) {
	h1 := Hash("hello", "secret")
	h2 := Hash("hello", "secret")
	if h1 != h2 {
		t.Errorf("Hash should be deterministic: %s != %s", h1, h2)
	}
}

func TestHash_DifferentInput(t *testing.T) {
	h1 := Hash("hello", "secret")
	h2 := Hash("world", "secret")
	if h1 == h2 {
		t.Error("different input should produce different hash")
	}
}

func TestHash_DifferentSecret(t *testing.T) {
	h1 := Hash("hello", "secret1")
	h2 := Hash("hello", "secret2")
	if h1 == h2 {
		t.Error("different secret should produce different hash")
	}
}

func TestHash_NotEmpty(t *testing.T) {
	h := Hash("test", "key")
	if h == "" {
		t.Error("hash should not be empty")
	}
}

func TestHash_EmptyInput(t *testing.T) {
	h := Hash("", "secret")
	if h == "" {
		t.Error("hash of empty input should not be empty")
	}
}

func TestHash_EmptySecret(t *testing.T) {
	h := Hash("hello", "")
	if h == "" {
		t.Error("hash with empty secret should not be empty")
	}
}

func TestHash_HexEncoded(t *testing.T) {
	h := Hash("test", "key")
	if len(h) != 64 {
		t.Errorf("hash length = %d, want 64 (HMAC-SHA256 hex)", len(h))
	}
}
