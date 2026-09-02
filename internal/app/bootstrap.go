package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PlatformTenantCode is the stable, reserved code of the auto-provisioned
// platform enterprise. It guarantees the system administrator always has an
// active owner membership so enterprise settings and team management are
// reachable even before any external tenant is onboarded.
const PlatformTenantCode = "deeptrols-platform"

// PlatformTenantName is the display name shown for the platform enterprise.
const PlatformTenantName = "智曜TokenHub 平台企业"

// EnsurePlatformTenant finds or creates the platform tenant and ensures the
// system administrator is its active owner. It is idempotent and safe to call
// on every boot: it creates the tenant and membership once, and repairs a
// degraded owner role/status on later runs. A timeout is applied so a dead
// database cannot hang server startup.
func (a *App) EnsurePlatformTenant(ctx context.Context, adminID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	t, err := a.Tenants.FindByCode(ctx, PlatformTenantCode)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ensure platform tenant: find by code: %w", err)
		}
		t, err = a.createPlatformTenant(ctx, adminID)
		if err != nil {
			return err
		}
	}

	// Ensure the admin is the active owner of the platform tenant.
	m, err := a.Memberships.FindByUserAndTenant(ctx, adminID, t.ID)
	if err != nil {
		if !errors.Is(err, membership.ErrNotFound) {
			return fmt.Errorf("ensure platform tenant: find membership: %w", err)
		}
		m, err = a.createOwnerMembership(ctx, t.ID, adminID)
		if err != nil {
			return err
		}
	}

	if m.Role != domain.MembershipRoleOwner {
		if updateErr := a.Memberships.UpdateRole(ctx, m.ID, domain.MembershipRoleOwner); updateErr != nil {
			return fmt.Errorf("ensure platform tenant: promote to owner: %w", updateErr)
		}
	}
	if m.Status != domain.MembershipStatusActive {
		if updateErr := a.Memberships.UpdateStatus(ctx, m.ID, domain.MembershipStatusActive); updateErr != nil {
			return fmt.Errorf("ensure platform tenant: reactivate: %w", updateErr)
		}
	}
	return nil
}

// createOwnerMembership inserts the admin as the tenant's owner. A concurrent
// boot may win the (tenant_id, user_id) unique-constraint race; in that case
// the winner's membership is re-fetched so the caller can still repair it.
func (a *App) createOwnerMembership(ctx context.Context, tenantID, adminID uuid.UUID) (*domain.TenantMembership, error) {
	ownerMembership := &domain.TenantMembership{
		ID:       uuid.New(),
		TenantID: tenantID,
		UserID:   adminID,
		Role:     domain.MembershipRoleOwner,
		Status:   domain.MembershipStatusActive,
	}
	err := a.Memberships.Create(ctx, ownerMembership)
	if err == nil {
		return ownerMembership, nil
	}
	if errors.Is(err, membership.ErrAlreadyExists) {
		winner, findErr := a.Memberships.FindByUserAndTenant(ctx, adminID, tenantID)
		if findErr != nil {
			return nil, fmt.Errorf("ensure platform tenant: create membership (race, refetch failed): %w", findErr)
		}
		return winner, nil
	}
	return nil, fmt.Errorf("ensure platform tenant: create membership: %w", err)
}

// createPlatformTenant inserts the platform tenant. A concurrent boot may win
// the unique-code insert race; in that case the winner's row is re-fetched.
func (a *App) createPlatformTenant(ctx context.Context, adminID uuid.UUID) (*domain.Tenant, error) {
	now := time.Now().UTC()
	t := &domain.Tenant{
		ID:        uuid.New(),
		Code:      PlatformTenantCode,
		Name:      PlatformTenantName,
		Status:    domain.TenantStatusActive,
		OwnerID:   &adminID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := a.Tenants.Create(ctx, t)
	if err == nil {
		return t, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		winner, findErr := a.Tenants.FindByCode(ctx, PlatformTenantCode)
		if findErr != nil {
			return nil, fmt.Errorf("ensure platform tenant: create (unique violation, refetch failed): %w", findErr)
		}
		return winner, nil
	}
	return nil, fmt.Errorf("ensure platform tenant: create: %w", err)
}
