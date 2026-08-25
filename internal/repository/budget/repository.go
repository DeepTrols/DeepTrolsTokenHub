package budget

import (
	"context"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.Budget, error)
	ListRequestsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.BudgetRequest, error)
	ListRequests(ctx context.Context) ([]domain.BudgetRequest, error)
	CreateRequest(ctx context.Context, req *domain.BudgetRequest) error
	// ApproveRequest approves a pending request and increases the tenant's
	// matching budget limit by the requested amount (creating it if absent).
	ApproveRequest(ctx context.Context, requestID, reviewerID uuid.UUID) (*domain.Budget, error)
	RejectRequest(ctx context.Context, requestID, reviewerID uuid.UUID) error
}
