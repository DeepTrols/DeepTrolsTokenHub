package domain

import (
	"time"

	"github.com/google/uuid"
)

// InvitationStatus is the state of a tenant invitation.
type InvitationStatus string

const (
	InvitationStatusPending   InvitationStatus = "pending"
	InvitationStatusAccepted  InvitationStatus = "accepted"
	InvitationStatusExpired   InvitationStatus = "expired"
	InvitationStatusCancelled InvitationStatus = "cancelled"
)

// TenantInvitation represents an invitation for a user to join a tenant.
type TenantInvitation struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	InvitedBy uuid.UUID
	Email     string
	Role      MembershipRole
	Token     string
	Status    InvitationStatus
	ExpiresAt time.Time
	CreatedAt time.Time
}
