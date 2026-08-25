package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type BudgetPeriod string

const (
	BudgetPeriodMonthly BudgetPeriod = "monthly"
	BudgetPeriodTotal   BudgetPeriod = "total"
)

type BudgetStatus string

const (
	BudgetStatusActive   BudgetStatus = "active"
	BudgetStatusDisabled BudgetStatus = "disabled"
)

type BudgetRequestStatus string

const (
	BudgetRequestPending  BudgetRequestStatus = "pending"
	BudgetRequestApproved BudgetRequestStatus = "approved"
	BudgetRequestRejected BudgetRequestStatus = "rejected"
)

// Budget is a tenant's spend limit for a period. spent_amount accumulates
// usage spend; enforcement is wired at the gateway (next Phase 1 batch).
type Budget struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	Period      BudgetPeriod
	LimitAmount decimal.Decimal
	SpentAmount decimal.Decimal
	Status      BudgetStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// BudgetRequest is an enterprise admin's request to increase a budget.
type BudgetRequest struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	RequestedAmount decimal.Decimal
	Reason          string
	Status          BudgetRequestStatus
	ReviewerID      *uuid.UUID
	ReviewedAt      *time.Time
	CreatedAt       time.Time
}
