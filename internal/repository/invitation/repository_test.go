package invitation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// seedUserForInvitation inserts a user via raw SQL.
func seedUserForInvitation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role, status, totp_enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, false, $7, $8)`,
		id, email, "hashed", email, "user", domain.UserStatusActive, time.Now().UTC(), time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("seedUserForInvitation: %v", err)
	}
	return id
}

// seedTenantForInvitation inserts a tenant via raw SQL.
func seedTenantForInvitation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	code := "TEN-" + uuid.NewString()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		id, code, name, "active", time.Now().UTC(), time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("seedTenantForInvitation: %v", err)
	}
	return id
}

func TestCreateInvitation(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "invite-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "inviter@test.com")

	inv := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: inviterID,
		Email:     "invitee@test.com",
		Role:      domain.MembershipRoleMember,
		Token:     uuid.New().String(),
		Status:    domain.InvitationStatusPending,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	err := repo.Create(ctx, inv)
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if inv.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Verify we can find it by token
	found, err := repo.FindByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("FindByToken after create: %v", err)
	}
	if found.ID != inv.ID {
		t.Errorf("ID = %s, want %s", found.ID, inv.ID)
	}
	if found.Email != "invitee@test.com" {
		t.Errorf("Email = %s, want invitee@test.com", found.Email)
	}
}

func TestFindByTokenNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	_, err := repo.FindByToken(ctx, "nonexistent-token")
	if err == nil {
		t.Fatal("expected error for unknown token")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestFindPendingByEmail(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "pending-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "pending-inviter@test.com")

	inv := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: inviterID,
		Email:     "pending-invitee@test.com",
		Role:      domain.MembershipRoleAdmin,
		Token:     uuid.New().String(),
		Status:    domain.InvitationStatusPending,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create: %v", err)
	}

	invitations, err := repo.FindPendingByEmail(ctx, "pending-invitee@test.com")
	if err != nil {
		t.Fatalf("FindPendingByEmail: %v", err)
	}
	if len(invitations) != 1 {
		t.Fatalf("len(invitations) = %d, want 1", len(invitations))
	}
	if invitations[0].Role != domain.MembershipRoleAdmin {
		t.Errorf("Role = %s, want admin", invitations[0].Role)
	}
}

func TestFindPendingByEmailIgnoresNonPending(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "accepted-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "accepted-inviter@test.com")

	inv := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: inviterID,
		Email:     "accepted-invitee@test.com",
		Role:      domain.MembershipRoleMember,
		Token:     uuid.New().String(),
		Status:    domain.InvitationStatusAccepted,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create: %v", err)
	}

	invitations, err := repo.FindPendingByEmail(ctx, "accepted-invitee@test.com")
	if err != nil {
		t.Fatalf("FindPendingByEmail: %v", err)
	}
	if len(invitations) != 0 {
		t.Errorf("len(invitations) = %d, want 0 (already accepted)", len(invitations))
	}
}

func TestFindPendingByEmailNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	invitations, err := repo.FindPendingByEmail(ctx, "no-such-email@test.com")
	if err != nil {
		t.Fatalf("FindPendingByEmail: %v", err)
	}
	if len(invitations) != 0 {
		t.Errorf("len(invitations) = %d, want 0", len(invitations))
	}
}

func TestListByTenantID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "list-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "list-inviter@test.com")

	for i := 0; i < 3; i++ {
		inv := &domain.TenantInvitation{
			ID:        uuid.New(),
			TenantID:  tenantID,
			InvitedBy: inviterID,
			Email:     "invitee" + string(rune('0'+i)) + "@test.com",
			Role:      domain.MembershipRoleMember,
			Token:     uuid.New().String(),
			Status:    domain.InvitationStatusPending,
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	invitations, err := repo.ListByTenantID(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListByTenantID: %v", err)
	}
	if len(invitations) != 3 {
		t.Errorf("len(invitations) = %d, want 3", len(invitations))
	}
}

func TestListByTenantIDEmpty(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	invitations, err := repo.ListByTenantID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("ListByTenantID: %v", err)
	}
	if len(invitations) != 0 {
		t.Errorf("len(invitations) = %d, want 0", len(invitations))
	}
}

func TestUpdateStatus(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "cancel-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "cancel-inviter@test.com")

	inv := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: inviterID,
		Email:     "cancel-invitee@test.com",
		Role:      domain.MembershipRoleMember,
		Token:     uuid.New().String(),
		Status:    domain.InvitationStatusPending,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create: %v", err)
	}

	err := repo.UpdateStatus(ctx, inv.ID, domain.InvitationStatusCancelled)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	found, err := repo.FindByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("FindByToken: %v", err)
	}
	if found.Status != domain.InvitationStatusCancelled {
		t.Errorf("Status = %s, want cancelled", found.Status)
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	err := repo.UpdateStatus(ctx, uuid.New(), domain.InvitationStatusCancelled)
	if err == nil {
		t.Fatal("expected ErrNotFound for nonexistent invitation")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestExpirePending(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "expire-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "expire-inviter@test.com")

	// Create an expired pending invitation
	expired := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: inviterID,
		Email:     "expired@test.com",
		Role:      domain.MembershipRoleMember,
		Token:     uuid.New().String(),
		Status:    domain.InvitationStatusPending,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // already expired
	}
	if err := repo.Create(ctx, expired); err != nil {
		t.Fatalf("Create expired: %v", err)
	}

	// Create a valid pending invitation
	valid := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: inviterID,
		Email:     "valid@test.com",
		Role:      domain.MembershipRoleMember,
		Token:     uuid.New().String(),
		Status:    domain.InvitationStatusPending,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}
	if err := repo.Create(ctx, valid); err != nil {
		t.Fatalf("Create valid: %v", err)
	}

	count, err := repo.ExpirePending(ctx)
	if err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Verify expired one is now expired
	found, err := repo.FindByToken(ctx, expired.Token)
	if err != nil {
		t.Fatalf("FindByToken expired: %v", err)
	}
	if found.Status != domain.InvitationStatusExpired {
		t.Errorf("Status = %s, want expired", found.Status)
	}

	// Verify valid one is still pending
	found, err = repo.FindByToken(ctx, valid.Token)
	if err != nil {
		t.Fatalf("FindByToken valid: %v", err)
	}
	if found.Status != domain.InvitationStatusPending {
		t.Errorf("Status = %s, want pending", found.Status)
	}
}

func TestExpirePendingNoExpired(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	tenantID := seedTenantForInvitation(t, ctx, repo.pool, "noexpire-tenant")
	inviterID := seedUserForInvitation(t, ctx, repo.pool, "noexpire-inviter@test.com")

	// Create only valid (non-expired) pending invitations
	for i := 0; i < 2; i++ {
		inv := &domain.TenantInvitation{
			ID:        uuid.New(),
			TenantID:  tenantID,
			InvitedBy: inviterID,
			Email:     "valid" + string(rune('0'+i)) + "@test.com",
			Role:      domain.MembershipRoleMember,
			Token:     uuid.New().String(),
			Status:    domain.InvitationStatusPending,
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}
		if err := repo.Create(ctx, inv); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	count, err := repo.ExpirePending(ctx)
	if err != nil {
		t.Fatalf("ExpirePending: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (none expired)", count)
	}
}
