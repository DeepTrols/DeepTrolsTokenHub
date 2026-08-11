package membership

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

// seedUser inserts a user via raw SQL and returns the user.
func seedUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string) *domain.User {
	t.Helper()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "hashed",
		DisplayName:  email,
		Role:         "user",
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role, status, totp_enabled, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, false, $7, $8)`,
		u.ID, u.Email, u.PasswordHash, u.DisplayName, u.Role, u.Status, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("seedUser: %v", err)
	}
	return u
}

// seedTenant inserts a tenant via raw SQL and returns the tenant.
func seedTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) *domain.Tenant {
	t.Helper()
	ten := &domain.Tenant{
		ID:        uuid.New(),
		Code:      "TEN-" + uuid.NewString(),
		Name:      name,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		ten.ID, ten.Code, ten.Name, ten.Status, ten.CreatedAt, ten.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("seedTenant: %v", err)
	}
	return ten
}

// seedMembership inserts a membership via raw SQL and returns it.
func seedMembership(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID, userID uuid.UUID, role domain.MembershipRole) *domain.TenantMembership {
	t.Helper()
	m := &domain.TenantMembership{
		ID:        uuid.New(),
		TenantID:  tenantID,
		UserID:    userID,
		Role:      role,
		Status:    domain.MembershipStatusActive,
		JoinedAt:  time.Now().UTC(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		m.ID, m.TenantID, m.UserID, m.Role, m.Status, m.JoinedAt, m.CreatedAt, m.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("seedMembership: %v", err)
	}
	return m
}

func TestCreateMembership(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "create-mem@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "create-mem-tenant")

	m := &domain.TenantMembership{
		ID:       uuid.New(),
		TenantID: tenant.ID,
		UserID:   user.ID,
		Role:     domain.MembershipRoleMember,
		Status:   domain.MembershipStatusActive,
	}
	err := repo.Create(ctx, m)
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if m.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if m.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}

	// Verify we can find it
	found, err := repo.FindByUserAndTenant(ctx, user.ID, tenant.ID)
	if err != nil {
		t.Fatalf("FindByUserAndTenant after create: %v", err)
	}
	if found.ID != m.ID {
		t.Errorf("ID = %s, want %s", found.ID, m.ID)
	}
}

func TestCreateMembershipDuplicate(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "dup-mem@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "dup-mem-tenant")

	m := &domain.TenantMembership{
		ID:       uuid.New(),
		TenantID: tenant.ID,
		UserID:   user.ID,
		Role:     domain.MembershipRoleMember,
		Status:   domain.MembershipStatusActive,
	}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	dup := &domain.TenantMembership{
		ID:       uuid.New(),
		TenantID: tenant.ID,
		UserID:   user.ID,
		Role:     domain.MembershipRoleAdmin,
		Status:   domain.MembershipStatusActive,
	}
	err := repo.Create(ctx, dup)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got: %v", err)
	}
}

func TestFindByUserID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "findbyuser@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "findbyuser-tenant")
	m := seedMembership(t, ctx, repo.pool, tenant.ID, user.ID, domain.MembershipRoleAdmin)

	found, err := repo.FindByUserID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByUserID: %v", err)
	}
	if found.ID != m.ID {
		t.Errorf("ID = %s, want %s", found.ID, m.ID)
	}
	if found.Role != domain.MembershipRoleAdmin {
		t.Errorf("Role = %s, want admin", found.Role)
	}
}

func TestFindByUserIDNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	_, err := repo.FindByUserID(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestFindByUserIDIgnoresNonActive(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "suspended@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "suspended-tenant")
	m := seedMembership(t, ctx, repo.pool, tenant.ID, user.ID, domain.MembershipRoleMember)
	// Set status to suspended
	_, err := repo.pool.Exec(ctx, `UPDATE tenant_memberships SET status = 'suspended' WHERE id = $1`, m.ID)
	if err != nil {
		t.Fatalf("update status: %v", err)
	}

	_, err = repo.FindByUserID(ctx, user.ID)
	if err == nil {
		t.Fatal("expected ErrNotFound for suspended membership")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestFindByTenantID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user1 := seedUser(t, ctx, repo.pool, "member1@test.com")
	user2 := seedUser(t, ctx, repo.pool, "member2@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "list-tenant")

	m1 := seedMembership(t, ctx, repo.pool, tenant.ID, user1.ID, domain.MembershipRoleOwner)
	m2 := seedMembership(t, ctx, repo.pool, tenant.ID, user2.ID, domain.MembershipRoleMember)

	members, err := repo.FindByTenantID(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("FindByTenantID: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("len(members) = %d, want 2", len(members))
	}
	// Verify content, not just count
	byUserID := map[uuid.UUID]domain.TenantMembership{}
	for _, m := range members {
		byUserID[m.UserID] = m
	}
	if byUserID[user1.ID].ID != m1.ID {
		t.Errorf("user1 membership ID = %s, want %s", byUserID[user1.ID].ID, m1.ID)
	}
	if byUserID[user1.ID].Role != domain.MembershipRoleOwner {
		t.Errorf("user1 role = %s, want owner", byUserID[user1.ID].Role)
	}
	if byUserID[user2.ID].ID != m2.ID {
		t.Errorf("user2 membership ID = %s, want %s", byUserID[user2.ID].ID, m2.ID)
	}
}

func TestFindByTenantIDEmpty(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	members, err := repo.FindByTenantID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("FindByTenantID: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("len(members) = %d, want 0", len(members))
	}
}

func TestFindByUserAndTenant(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "specific@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "specific-tenant")
	m := seedMembership(t, ctx, repo.pool, tenant.ID, user.ID, domain.MembershipRoleMember)

	found, err := repo.FindByUserAndTenant(ctx, user.ID, tenant.ID)
	if err != nil {
		t.Fatalf("FindByUserAndTenant: %v", err)
	}
	if found.ID != m.ID {
		t.Errorf("ID = %s, want %s", found.ID, m.ID)
	}
}

func TestFindByUserAndTenantNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	_, err := repo.FindByUserAndTenant(ctx, uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error for nonexistent membership")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateRole(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "updaterole@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "updaterole-tenant")
	m := seedMembership(t, ctx, repo.pool, tenant.ID, user.ID, domain.MembershipRoleMember)

	err := repo.UpdateRole(ctx, m.ID, domain.MembershipRoleAdmin)
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}

	found, err := repo.FindByUserAndTenant(ctx, user.ID, tenant.ID)
	if err != nil {
		t.Fatalf("FindByUserAndTenant: %v", err)
	}
	if found.Role != domain.MembershipRoleAdmin {
		t.Errorf("Role = %s, want admin", found.Role)
	}
}

func TestUpdateRoleNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	err := repo.UpdateRole(ctx, uuid.New(), domain.MembershipRoleAdmin)
	if err == nil {
		t.Fatal("expected ErrNotFound for nonexistent membership")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateStatus(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "updatestatus@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "updatestatus-tenant")
	m := seedMembership(t, ctx, repo.pool, tenant.ID, user.ID, domain.MembershipRoleMember)

	err := repo.UpdateStatus(ctx, m.ID, domain.MembershipStatusSuspended)
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	// Should not be found by FindByUserID (which filters for active only)
	_, err = repo.FindByUserID(ctx, user.ID)
	if err == nil {
		t.Fatal("expected ErrNotFound for suspended membership")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	err := repo.UpdateStatus(ctx, uuid.New(), domain.MembershipStatusLeft)
	if err == nil {
		t.Fatal("expected ErrNotFound for nonexistent membership")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDelete(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUser(t, ctx, repo.pool, "delete@test.com")
	tenant := seedTenant(t, ctx, repo.pool, "delete-tenant")
	m := seedMembership(t, ctx, repo.pool, tenant.ID, user.ID, domain.MembershipRoleMember)

	err := repo.Delete(ctx, m.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = repo.FindByUserAndTenant(ctx, user.ID, tenant.ID)
	if err == nil {
		t.Fatal("expected ErrNotFound after delete")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	err := repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected ErrNotFound for nonexistent membership")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}
