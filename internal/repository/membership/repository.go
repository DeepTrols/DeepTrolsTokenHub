package membership

import (
	"context"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a membership is not found.
var ErrNotFound = fmt.Errorf("membership not found")

// ErrAlreadyExists is returned when a membership already exists for the user+tenant pair.
var ErrAlreadyExists = fmt.Errorf("membership already exists")

// Repository defines the tenant membership data access interface.
type Repository interface {
	// FindByUserID returns the active membership for a user (one user, one tenant in Phase 1).
	FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.TenantMembership, error)
	// FindByTenantID returns all memberships for a tenant.
	FindByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantMembership, error)
	// FindByUserAndTenant returns a specific membership.
	FindByUserAndTenant(ctx context.Context, userID, tenantID uuid.UUID) (*domain.TenantMembership, error)
	// Create inserts a new membership.
	Create(ctx context.Context, m *domain.TenantMembership) error
	// UpdateRole changes the membership role.
	UpdateRole(ctx context.Context, id uuid.UUID, role domain.MembershipRole) error
	// UpdateStatus changes the membership status.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.MembershipStatus) error
	// Delete removes a membership permanently.
	Delete(ctx context.Context, id uuid.UUID) error
}
