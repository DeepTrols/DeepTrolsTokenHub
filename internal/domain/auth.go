package domain

import (
	"github.com/google/uuid"
)

// RequestIdentity is the stable request context established at the gateway entry.
type RequestIdentity struct {
	APIKeyID    uuid.UUID
	UserID      uuid.UUID
	TenantID    *uuid.UUID
	RequestID   string
	RequestType string
	UserLevel   string
	APIKey      *APIKey
}
