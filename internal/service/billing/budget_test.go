package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/budget"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type fakeBudgetRepo struct {
	budget  *domain.Budget
	accrued decimal.Decimal
}

func (f *fakeBudgetRepo) FindMonthly(context.Context, uuid.UUID) (*domain.Budget, error) {
	if f.budget == nil {
		return nil, budget.ErrNotFound
	}
	return f.budget, nil
}

func (f *fakeBudgetRepo) AccrueSpend(_ context.Context, _ uuid.UUID, amount decimal.Decimal) error {
	f.accrued = f.accrued.Add(amount)
	return nil
}

func TestBudgetChecker_CheckAndAccrue(t *testing.T) {
	tenantID := uuid.New()

	// No budget = unlimited.
	checker := NewBudgetChecker(&fakeBudgetRepo{})
	if err := checker.Check(context.Background(), &tenantID, decimal.NewFromInt(100)); err != nil {
		t.Fatalf("no-budget check should pass: %v", err)
	}
	// Nil tenant = unlimited.
	if err := checker.Check(context.Background(), nil, decimal.NewFromInt(100)); err != nil {
		t.Fatalf("nil-tenant check should pass: %v", err)
	}

	// Over budget.
	checker = NewBudgetChecker(&fakeBudgetRepo{
		budget: &domain.Budget{
			TenantID: tenantID, LimitAmount: decimal.NewFromInt(10), SpentAmount: decimal.NewFromInt(9),
			Status: domain.BudgetStatusActive,
		},
	})
	if err := checker.Check(context.Background(), &tenantID, decimal.NewFromInt(2)); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("over-budget check = %v, want ErrBudgetExceeded", err)
	}
	if err := checker.Check(context.Background(), &tenantID, decimal.NewFromInt(1)); err != nil {
		t.Fatalf("at-limit check should pass: %v", err)
	}

	repo := &fakeBudgetRepo{
		budget: &domain.Budget{
			TenantID: tenantID, LimitAmount: decimal.NewFromInt(100), SpentAmount: decimal.Zero,
			Status: domain.BudgetStatusActive,
		},
	}
	checker = NewBudgetChecker(repo)
	checker.Accrue(context.Background(), &tenantID, decimal.NewFromInt(7))
	checker.Accrue(context.Background(), &tenantID, decimal.NewFromInt(3))
	if !repo.accrued.Equal(decimal.NewFromInt(10)) {
		t.Errorf("accrued = %s, want 10", repo.accrued)
	}
}
