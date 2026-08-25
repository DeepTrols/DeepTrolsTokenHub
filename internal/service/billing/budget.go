package billing

import (
	"context"
	"errors"
	"log"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/budget"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrBudgetExceeded is returned when a tenant's monthly budget would be
// exceeded by the estimated cost of a request.
var ErrBudgetExceeded = errors.New("tenant budget exceeded")

// budgetRepo is the slice of budget.Repository the checker needs.
type budgetRepo interface {
	FindMonthly(ctx context.Context, tenantID uuid.UUID) (*domain.Budget, error)
	AccrueSpend(ctx context.Context, tenantID uuid.UUID, amount decimal.Decimal) error
}

// BudgetChecker enforces tenant monthly budgets at the gateway: check the
// estimated cost before upstream, accrue the actual cost after settlement.
// No budget row = unlimited; errors fail open with a log so a budget outage
// never blocks billable traffic silently.
type BudgetChecker struct {
	budgets budgetRepo
}

func NewBudgetChecker(repo budgetRepo) *BudgetChecker {
	return &BudgetChecker{budgets: repo}
}

// Check returns ErrBudgetExceeded when the tenant's active monthly budget
// cannot cover the estimated cost. Non-tenant requests are unlimited.
func (c *BudgetChecker) Check(ctx context.Context, tenantID *uuid.UUID, estimatedCost decimal.Decimal) error {
	if c == nil || tenantID == nil {
		return nil
	}
	b, err := c.budgets.FindMonthly(ctx, *tenantID)
	if err != nil {
		if errors.Is(err, budget.ErrNotFound) {
			return nil // no budget = unlimited
		}
		log.Printf("billing: budget check degraded for tenant %s: %v", *tenantID, err)
		return nil
	}
	if b.SpentAmount.Add(estimatedCost).GreaterThan(b.LimitAmount) {
		return ErrBudgetExceeded
	}
	return nil
}

// Accrue adds actual spend to the tenant's monthly budget. Best-effort.
func (c *BudgetChecker) Accrue(ctx context.Context, tenantID *uuid.UUID, amount decimal.Decimal) {
	if c == nil || tenantID == nil || amount.LessThanOrEqual(decimal.Zero) {
		return
	}
	if err := c.budgets.AccrueSpend(ctx, *tenantID, amount); err != nil {
		log.Printf("billing: budget accrue degraded for tenant %s: %v", *tenantID, err)
	}
}
