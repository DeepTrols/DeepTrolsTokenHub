package user

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

// seedUserForTest inserts a user directly via pool and returns the domain.User.
func seedUserForTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, displayName string) *domain.User {
	t.Helper()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "hashed_password",
		DisplayName:  displayName,
		Role:         "user",
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		u.ID, u.Email, u.PasswordHash, u.DisplayName, u.Role,
		u.Status, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		t.Fatalf("seedUserForTest: %v", err)
	}
	return u
}

func TestFindByEmail(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUserForTest(t, ctx, repo.pool, "findbyemail@test.com", "FindByEmail User")

	found, err := repo.FindByEmail(ctx, "findbyemail@test.com")
	if err != nil {
		t.Fatalf("FindByEmail: unexpected error: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("ID = %s, want %s", found.ID, user.ID)
	}
	if found.Email != user.Email {
		t.Errorf("Email = %s, want %s", found.Email, user.Email)
	}
	if found.PasswordHash != user.PasswordHash {
		t.Errorf("PasswordHash = %s, want %s", found.PasswordHash, user.PasswordHash)
	}
	if found.DisplayName != user.DisplayName {
		t.Errorf("DisplayName = %s, want %s", found.DisplayName, user.DisplayName)
	}
	if found.Status != user.Status {
		t.Errorf("Status = %s, want %s", found.Status, user.Status)
	}
}

func TestFindByEmailNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	_, err := repo.FindByEmail(ctx, "nonexistent@test.com")
	if err == nil {
		t.Fatal("expected error for unknown email")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestFindByID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := seedUserForTest(t, ctx, repo.pool, "findbyid@test.com", "FindByID User")

	found, err := repo.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("FindByID: unexpected error: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("ID = %s, want %s", found.ID, user.ID)
	}
	if found.Email != user.Email {
		t.Errorf("Email = %s, want %s", found.Email, user.Email)
	}
}

func TestFindByIDNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	_, err := repo.FindByID(ctx, uuid.New())
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
}

func TestCreate(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := &domain.User{
		ID:           uuid.New(),
		Email:        "create@test.com",
		PasswordHash: "hashed_pw",
		DisplayName:  "Create User",
		Role:         "user",
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}

	// Verify we can retrieve the created user.
	found, err := repo.FindByEmail(ctx, "create@test.com")
	if err != nil {
		t.Fatalf("FindByEmail after create: %v", err)
	}
	if found.ID != user.ID {
		t.Errorf("ID = %s, want %s", found.ID, user.ID)
	}
	if found.Email != user.Email {
		t.Errorf("Email = %s, want %s", found.Email, user.Email)
	}
	if found.Status != domain.UserStatusActive {
		t.Errorf("Status = %s, want active", found.Status)
	}
}

func TestCreateDuplicate(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user1 := &domain.User{
		ID:           uuid.New(),
		Email:        "dup@test.com",
		PasswordHash: "hash1",
		Role:         "user",
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := repo.Create(ctx, user1); err != nil {
		t.Fatalf("Create first: unexpected error: %v", err)
	}

	user2 := &domain.User{
		ID:           uuid.New(),
		Email:        "dup@test.com", // same email
		PasswordHash: "hash2",
		Role:         "user",
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	err := repo.Create(ctx, user2)
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
}

func TestCreateUserBannedStatus(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	user := &domain.User{
		ID:           uuid.New(),
		Email:        "banned@test.com",
		PasswordHash: "hashed_pw",
		Role:         "user",
		Status:       domain.UserStatusBanned,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("Create banned: %v", err)
	}

	found, err := repo.FindByEmail(ctx, "banned@test.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if found.Status != domain.UserStatusBanned {
		t.Errorf("Status = %s, want banned", found.Status)
	}
}

func TestListByUserType(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	now := time.Now().UTC()
	personal := &domain.User{
		ID: uuid.New(), Email: "list-p@test.com", PasswordHash: "h",
		DisplayName: "P", Role: "user", UserType: domain.UserTypePersonal,
		Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	enterprise := &domain.User{
		ID: uuid.New(), Email: "list-e@test.com", PasswordHash: "h",
		DisplayName: "E", Role: "user", UserType: domain.UserTypeEnterprise,
		Status: domain.UserStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, personal); err != nil {
		t.Fatalf("Create personal: %v", err)
	}
	if err := repo.Create(ctx, enterprise); err != nil {
		t.Fatalf("Create enterprise: %v", err)
	}

	users, err := repo.List(ctx, ListFilter{UserType: domain.UserTypePersonal}, 20, 0)
	if err != nil {
		t.Fatalf("List personal: %v", err)
	}
	if len(users) != 1 || users[0].ID != personal.ID {
		t.Errorf("personal filter: got %d users, want only the personal user", len(users))
	}

	users, err = repo.List(ctx, ListFilter{UserType: domain.UserTypeEnterprise}, 20, 0)
	if err != nil {
		t.Fatalf("List enterprise: %v", err)
	}
	if len(users) != 1 || users[0].ID != enterprise.ID {
		t.Errorf("enterprise filter: got %d users, want only the enterprise user", len(users))
	}

	// Zero-value filter returns every user.
	users, err = repo.List(ctx, ListFilter{}, 20, 0)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("no filter: got %d users, want 2", len(users))
	}
}

func TestCountByUserType(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	now := time.Now().UTC()
	seed := func(email string, userType domain.UserType) {
		t.Helper()
		u := &domain.User{
			ID: uuid.New(), Email: email, PasswordHash: "h", DisplayName: "U",
			Role: "user", UserType: userType, Status: domain.UserStatusActive,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := repo.Create(ctx, u); err != nil {
			t.Fatalf("Create %s: %v", email, err)
		}
	}
	seed("count-p1@test.com", domain.UserTypePersonal)
	seed("count-p2@test.com", domain.UserTypePersonal)
	seed("count-e1@test.com", domain.UserTypeEnterprise)

	if n, err := repo.Count(ctx, ListFilter{UserType: domain.UserTypePersonal}); err != nil || n != 2 {
		t.Errorf("Count personal = %d, err = %v; want 2", n, err)
	}
	if n, err := repo.Count(ctx, ListFilter{UserType: domain.UserTypeEnterprise}); err != nil || n != 1 {
		t.Errorf("Count enterprise = %d, err = %v; want 1", n, err)
	}
	if n, err := repo.Count(ctx, ListFilter{}); err != nil || n != 3 {
		t.Errorf("Count all = %d, err = %v; want 3", n, err)
	}
}
