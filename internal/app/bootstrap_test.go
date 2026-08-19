package app

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
)

// bootstrapApp builds a minimal App with just the repositories the bootstrap
// path touches (users, tenants, memberships) against a real DB.
func bootstrapApp(t *testing.T) *App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	return &App{
		Tenants:     tenant.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Users:       user.NewPostgresRepository(pool),
	}
}

// createBootstrapAdmin mirrors ensureAdminUser's fixed-ID admin so the test
// exercises the exact same user the server bootstraps.
func createBootstrapAdmin(t *testing.T, a *App) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	adminID := uuid.Nil
	if err := a.Users.Create(context.Background(), &domain.User{
		ID:           adminID,
		Email:        "deeptrols@admin.com",
		PasswordHash: "x",
		DisplayName:  "Administrator",
		Role:         "admin",
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	return adminID
}

func TestEnsurePlatformTenant(t *testing.T) {
	t.Run("creates platform tenant and owner membership", func(t *testing.T) {
		a := bootstrapApp(t)
		adminID := createBootstrapAdmin(t, a)
		ctx := context.Background()

		if err := a.EnsurePlatformTenant(ctx, adminID); err != nil {
			t.Fatalf("EnsurePlatformTenant: %v", err)
		}

		pt, err := a.Tenants.FindByCode(ctx, PlatformTenantCode)
		if err != nil {
			t.Fatalf("platform tenant not found: %v", err)
		}
		if pt.Status != domain.TenantStatusActive {
			t.Errorf("platform tenant status = %s, want active", pt.Status)
		}
		if pt.OwnerID == nil || *pt.OwnerID != adminID {
			t.Errorf("platform tenant owner = %v, want %v", pt.OwnerID, adminID)
		}

		m, err := a.Memberships.FindByUserAndTenant(ctx, adminID, pt.ID)
		if err != nil {
			t.Fatalf("admin membership not found: %v", err)
		}
		if m.Role != domain.MembershipRoleOwner {
			t.Errorf("membership role = %s, want owner", m.Role)
		}
		if m.Status != domain.MembershipStatusActive {
			t.Errorf("membership status = %s, want active", m.Status)
		}
	})

	t.Run("is idempotent on repeat runs", func(t *testing.T) {
		a := bootstrapApp(t)
		adminID := createBootstrapAdmin(t, a)
		ctx := context.Background()

		if err := a.EnsurePlatformTenant(ctx, adminID); err != nil {
			t.Fatalf("first run: %v", err)
		}
		first, err := a.Tenants.FindByCode(ctx, PlatformTenantCode)
		if err != nil {
			t.Fatalf("platform tenant not found: %v", err)
		}

		if err := a.EnsurePlatformTenant(ctx, adminID); err != nil {
			t.Fatalf("second run: %v", err)
		}
		second, err := a.Tenants.FindByCode(ctx, PlatformTenantCode)
		if err != nil {
			t.Fatalf("platform tenant not found: %v", err)
		}
		if second.ID != first.ID {
			t.Errorf("second run created a duplicate tenant: %s vs %s", second.ID, first.ID)
		}
	})

	t.Run("repairs degraded membership to owner active", func(t *testing.T) {
		a := bootstrapApp(t)
		adminID := createBootstrapAdmin(t, a)
		ctx := context.Background()

		// Pre-provision a platform tenant whose admin membership is degraded
		// (member + suspended) to simulate a manually-edited row.
		now := time.Now().UTC()
		if err := a.Tenants.Create(ctx, &domain.Tenant{
			ID:        uuid.New(),
			Code:      PlatformTenantCode,
			Name:      PlatformTenantName,
			Status:    domain.TenantStatusActive,
			OwnerID:   &adminID,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			t.Fatalf("pre-create tenant: %v", err)
		}
		pt, err := a.Tenants.FindByCode(ctx, PlatformTenantCode)
		if err != nil {
			t.Fatalf("platform tenant not found: %v", err)
		}
		if err := a.Memberships.Create(ctx, &domain.TenantMembership{
			ID:       uuid.New(),
			TenantID: pt.ID,
			UserID:   adminID,
			Role:     domain.MembershipRoleMember,
			Status:   domain.MembershipStatusSuspended,
		}); err != nil {
			t.Fatalf("pre-create membership: %v", err)
		}

		if err := a.EnsurePlatformTenant(ctx, adminID); err != nil {
			t.Fatalf("EnsurePlatformTenant: %v", err)
		}

		m, err := a.Memberships.FindByUserAndTenant(ctx, adminID, pt.ID)
		if err != nil {
			t.Fatalf("admin membership not found: %v", err)
		}
		if m.Role != domain.MembershipRoleOwner {
			t.Errorf("membership role = %s, want owner", m.Role)
		}
		if m.Status != domain.MembershipStatusActive {
			t.Errorf("membership status = %s, want active", m.Status)
		}
	})

	t.Run("recreates membership when it was deleted", func(t *testing.T) {
		a := bootstrapApp(t)
		adminID := createBootstrapAdmin(t, a)
		ctx := context.Background()

		// First bootstrap creates the tenant and membership.
		if err := a.EnsurePlatformTenant(ctx, adminID); err != nil {
			t.Fatalf("first run: %v", err)
		}
		pt, err := a.Tenants.FindByCode(ctx, PlatformTenantCode)
		if err != nil {
			t.Fatalf("platform tenant not found: %v", err)
		}

		// Simulate a manual DELETE of the admin's membership row.
		m, err := a.Memberships.FindByUserAndTenant(ctx, adminID, pt.ID)
		if err != nil {
			t.Fatalf("membership not found: %v", err)
		}
		if err := a.Memberships.Delete(ctx, m.ID); err != nil {
			t.Fatalf("delete membership: %v", err)
		}

		// Re-bootstrap must recreate the owner membership.
		if err := a.EnsurePlatformTenant(ctx, adminID); err != nil {
			t.Fatalf("re-bootstrap: %v", err)
		}
		re, err := a.Memberships.FindByUserAndTenant(ctx, adminID, pt.ID)
		if err != nil {
			t.Fatalf("recreated membership not found: %v", err)
		}
		if re.Role != domain.MembershipRoleOwner || re.Status != domain.MembershipStatusActive {
			t.Errorf("recreated membership = %s/%s, want owner/active", re.Role, re.Status)
		}
	})
}
