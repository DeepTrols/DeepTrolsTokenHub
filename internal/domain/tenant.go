package domain

import (
	"time"

	"github.com/google/uuid"
)

type Tenant struct {
	ID               uuid.UUID
	Code             string
	Name             string
	Status           TenantStatus
	OwnerID          *uuid.UUID
	CreditCode       string
	ContactEmail     string
	ContactPhone     string
	BusinessLicense  string
	BrandConfig      map[string]any
	RuntimeConfig    map[string]any
	SettlementConfig map[string]any
	StatusReason     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TenantStatus string

const (
	TenantStatusPendingReview TenantStatus = "pending_review"
	TenantStatusActive        TenantStatus = "active"
	TenantStatusSuspended     TenantStatus = "suspended"
	TenantStatusTerminated    TenantStatus = "terminated"
	TenantStatusRejected      TenantStatus = "rejected"
)

func (t Tenant) AllowTraffic() bool {
	return t.Status == TenantStatusActive
}

func (t Tenant) ValidTransitions() []TenantStatus {
	switch t.Status {
	case TenantStatusPendingReview:
		return []TenantStatus{TenantStatusActive, TenantStatusRejected}
	case TenantStatusActive:
		return []TenantStatus{TenantStatusSuspended, TenantStatusTerminated}
	case TenantStatusSuspended:
		return []TenantStatus{TenantStatusActive, TenantStatusTerminated}
	default:
		return nil
	}
}

type TenantDomain struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Domain    string
	IsPrimary bool
	CreatedAt time.Time
}
