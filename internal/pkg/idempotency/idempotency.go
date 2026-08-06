package idempotency

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

// Key generates a deterministic idempotency key from components.
// The composite key prevents cross-tenant request_id collisions.
func Key(tenantID, userID, apiKeyID uuid.UUID, requestType, requestID string) string {
	raw := fmt.Sprintf("%s:%s:%s:%s:%s", tenantID, userID, apiKeyID, requestType, requestID)
	hash := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("idem_%x", hash[:16])
}

// NewRequestID generates a new request ID when the client does not provide one.
func NewRequestID() string {
	return uuid.New().String()
}

// IsValidRequestID checks if the given string is a valid request ID format.
func IsValidRequestID(id string) bool {
	_, err := uuid.Parse(id)
	return err == nil
}
