package budget

import (
	"context"
	"errors"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrNotFound is returned when no budget exists for the tenant/period.
var ErrNotFound = errors.New("budget: not found")

type Repository interface {
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.Budget, error)
	ListRequestsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.BudgetRequest, error)
	ListRequests(ctx context.Context) ([]domain.BudgetRequest, error)
	CreateRequest(ctx context.Context, req *domain.BudgetRequest) error
	// ApproveRequest approves a pending request and increases the tenant's
	// matching budget limit by the requested amount (creating it if absent).
	ApproveRequest(ctx context.Context, requestID, reviewerID uuid.UUID) (*domain.Budget, error)
	RejectRequest(ctx context.Context, requestID, reviewerID uuid.UUID) error
	// FindMonthly returns the tenant's active monthly budget (if any).
	FindMonthly(ctx context.Context, tenantID uuid.UUID) (*domain.Budget, error)
	// AccrueSpend adds spent amount to the tenant's monthly budget (if any).
	AccrueSpend(ctx context.Context, tenantID uuid.UUID, amount decimal.Decimal) error
}
