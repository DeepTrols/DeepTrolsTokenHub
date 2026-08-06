package idempotency

import (
	"testing"

	"github.com/google/uuid"
)

func TestKey_Deterministic(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	apiKeyID := uuid.New()

	k1 := Key(tenantID, userID, apiKeyID, "chat", "req-123")
	k2 := Key(tenantID, userID, apiKeyID, "chat", "req-123")

	if k1 != k2 {
		t.Errorf("Key() should be deterministic: %s != %s", k1, k2)
	}
}

func TestKey_DifferentInputs(t *testing.T) {
	tenantA := uuid.New()
	tenantB := uuid.New()
	userID := uuid.New()
	apiKeyID := uuid.New()

	k1 := Key(tenantA, userID, apiKeyID, "chat", "req-1")
	k2 := Key(tenantB, userID, apiKeyID, "chat", "req-1")

	if k1 == k2 {
		t.Error("different tenants should produce different keys")
	}
}

func TestKey_HasPrefix(t *testing.T) {
	k := Key(uuid.New(), uuid.New(), uuid.New(), "chat", "request-abc")
	if len(k) < 5 || k[:5] != "idem_" {
		t.Errorf("Key should have 'idem_' prefix, got: %s", k)
	}
}

func TestNewRequestID_IsUUID(t *testing.T) {
	id := NewRequestID()
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("NewRequestID() = %q is not a valid UUID: %v", id, err)
	}
}

func TestNewRequestID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewRequestID()
		if ids[id] {
			t.Errorf("duplicate request ID: %s", id)
		}
		ids[id] = true
	}
}

func TestIsValidRequestID_Valid(t *testing.T) {
	id := uuid.New().String()
	if !IsValidRequestID(id) {
		t.Errorf("%s should be valid", id)
	}
}

func TestIsValidRequestID_Invalid(t *testing.T) {
	if IsValidRequestID("not-a-uuid") {
		t.Error("'not-a-uuid' should be invalid")
	}
	if IsValidRequestID("") {
		t.Error("empty string should be invalid")
	}
}
