package domain

import (
	"time"

	"github.com/google/uuid"
)

// QuotaPool is a tenant-scoped pool of tokens/quota.
// A nil ModelID means the pool is global (applies to all models for that tenant).
type QuotaPool struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	ModelID         *uuid.UUID
	Dimension       string
	TotalAmount     int64
	AllocatedAmount int64
	UsedAmount      int64
	UnitName        string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// QuotaAllocation assigns a portion of a pool to a specific user.
type QuotaAllocation struct {
	ID              uuid.UUID
	PoolID          uuid.UUID
	UserID          uuid.UUID
	AllocatedAmount int64
	UsedAmount      int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Remaining returns the remaining quota available for consumption.
func (a QuotaAllocation) Remaining() int64 {
	remaining := a.AllocatedAmount - a.UsedAmount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// QuotaLedgerEntry records a single quota mutation for audit trail.
type QuotaLedgerEntry struct {
	ID             uuid.UUID
	AllocationID   uuid.UUID
	IdempotencyKey string
	Action         QuotaLedgerAction
	Amount         int64
	BalanceAfter   int64
	ReferenceID    *uuid.UUID
	CreatedAt      time.Time
}

// QuotaLedgerAction is the type of quota operation.
type QuotaLedgerAction string

const (
	QuotaActionGrant    QuotaLedgerAction = "grant"
	QuotaActionAllocate QuotaLedgerAction = "allocate"
	QuotaActionReclaim  QuotaLedgerAction = "reclaim"
	QuotaActionConsume  QuotaLedgerAction = "consume"
	QuotaActionRestore  QuotaLedgerAction = "restore"
)
