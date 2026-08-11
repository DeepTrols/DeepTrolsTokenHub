package invitation

import (
	"context"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// ErrNotFound is returned when an invitation is not found.
var ErrNotFound = fmt.Errorf("invitation not found")

// ErrTokenExpired is returned when an invitation token has expired.
var ErrTokenExpired = fmt.Errorf("invitation token has expired")

// Repository defines the tenant invitation data access interface.
type Repository interface {
	// FindByToken looks up an invitation by its unique token.
	FindByToken(ctx context.Context, token string) (*domain.TenantInvitation, error)
	// FindPendingByEmail returns pending invitations for an email address.
	FindPendingByEmail(ctx context.Context, email string) ([]domain.TenantInvitation, error)
	// ListByTenantID returns all invitations for a tenant.
	ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantInvitation, error)
	// Create inserts a new invitation.
	Create(ctx context.Context, inv *domain.TenantInvitation) error
	// UpdateStatus changes the invitation status.
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.InvitationStatus) error
	// ExpirePending sets expired status on all pending invitations past their expiry.
	ExpirePending(ctx context.Context) (int64, error)
}
