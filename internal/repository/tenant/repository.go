package tenant

import (
	"context"
	"errors"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a tenant does not exist.
var ErrNotFound = errors.New("tenant: not found")

type Repository interface {
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error)
	FindByCode(ctx context.Context, code string) (*domain.Tenant, error)
	Create(ctx context.Context, tenant *domain.Tenant) error
	Update(ctx context.Context, tenant *domain.Tenant) error
	List(ctx context.Context) ([]domain.Tenant, error)
	// Delete permanently removes the tenant and all tenant-owned rows
	// (tenant models, memberships, invitations).
	// Returns ErrNotFound when no tenant with the id exists.
	Delete(ctx context.Context, id uuid.UUID) error
}
