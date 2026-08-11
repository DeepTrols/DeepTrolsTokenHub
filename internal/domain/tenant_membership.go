package domain

import (
	"time"

	"github.com/google/uuid"
)

// MembershipRole is the role a user has within a tenant.
type MembershipRole string

const (
	MembershipRoleOwner  MembershipRole = "owner"
	MembershipRoleAdmin  MembershipRole = "admin"
	MembershipRoleMember MembershipRole = "member"
)

// MembershipStatus is the state of a tenant membership.
type MembershipStatus string

const (
	MembershipStatusActive    MembershipStatus = "active"
	MembershipStatusSuspended MembershipStatus = "suspended"
	MembershipStatusLeft      MembershipStatus = "left"
)

// TenantMembership links a user to a tenant with a role and status.
type TenantMembership struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	Role      MembershipRole
	Status    MembershipStatus
	JoinedAt  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsAdminOrOwner returns true when the role grants management rights.
func (m TenantMembership) IsAdminOrOwner() bool {
	return m.Role == MembershipRoleOwner || m.Role == MembershipRoleAdmin
}
