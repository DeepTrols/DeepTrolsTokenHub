package tenant

import (
	"context"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

type Repository interface {
	FindByDomain(ctx context.Context, domain string) (*domain.Tenant, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	FindByCode(ctx context.Context, code string) (*domain.Tenant, error)
	Create(ctx context.Context, tenant *domain.Tenant) error
	Update(ctx context.Context, tenant *domain.Tenant) error
	List(ctx context.Context) ([]domain.Tenant, error)
}
