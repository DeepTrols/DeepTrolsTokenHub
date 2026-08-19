package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository with PostgreSQL via pgx/v5.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// compile-time interface check
var _ Repository = (*PostgresRepository)(nil)

// scanUser scans a users row into a domain.User.
// Column order: id, email, password_hash, display_name, role, status,
//
//	user_type, phone, avatar_url, created_at, updated_at
func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	var displayName, role, phone, avatarURL *string
	var userType string
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&displayName,
		&role,
		&u.Status,
		&userType,
		&phone,
		&avatarURL,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("user scan: %w", err)
	}
	if displayName != nil {
		u.DisplayName = *displayName
	}
	if role != nil {
		u.Role = *role
	}
	u.UserType = domain.UserType(userType)
	if phone != nil {
		u.Phone = *phone
	}
	if avatarURL != nil {
		u.AvatarURL = *avatarURL
	}
	return &u, nil
}

// FindByEmail retrieves a user by email.
// Returns ErrNotFound if no user exists with the given email.
func (r *PostgresRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	const query = `
		SELECT id, email, password_hash, display_name, role, status,
		       user_type, phone, avatar_url,
		       created_at, updated_at
		FROM users WHERE email = $1
	`
	u, err := scanUser(r.pool.QueryRow(ctx, query, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user find by email: %w", err)
	}
	return u, nil
}

// FindByID retrieves a user by ID.
// Returns ErrNotFound if no user exists with the given ID.
func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	const query = `
		SELECT id, email, password_hash, display_name, role, status,
		       user_type, phone, avatar_url,
		       created_at, updated_at
		FROM users WHERE id = $1
	`
	u, err := scanUser(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("user find by id: %w", err)
	}
	return u, nil
}

// Create inserts a new user row.
func (r *PostgresRepository) Create(ctx context.Context, user *domain.User) error {
	// Normalize to the platform default when the caller did not specify a type,
	// so an explicit "" never violates the users_user_type_check constraint.
	// The normalized value is used only for the INSERT below; the caller's
	// struct is left untouched (immutability).
	userType := user.UserType
	if userType == "" {
		userType = domain.UserTypePersonal
	}

	const query = `
		INSERT INTO users (id, email, password_hash, display_name, role, status,
		                   user_type, phone, avatar_url,
		                   created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := r.pool.Exec(ctx, query,
		user.ID, user.Email, user.PasswordHash, user.DisplayName, user.Role,
		user.Status,
		string(userType), user.Phone, user.AvatarURL,
		user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("user create: %w", err)
	}
	return nil
}

// List returns users ordered by creation date, with pagination and an
// optional user_type filter. A zero-value filter lists every user.
func (r *PostgresRepository) List(ctx context.Context, filter ListFilter, limit, offset int) ([]domain.User, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `
		SELECT id, email, password_hash, display_name, role, status,
		       user_type, phone, avatar_url,
		       created_at, updated_at
		FROM users`
	args := make([]any, 0, 4)
	conds := make([]string, 0, 2)
	if filter.UserType != "" {
		args = append(args, string(filter.UserType))
		conds = append(conds, fmt.Sprintf(`user_type = $%d`, len(args)))
	}
	if filter.ExcludeDeleted {
		conds = append(conds, `status <> 'deleted'`)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit, offset)
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("user list: %w", err)
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

// UpdateStatus sets a user's status (active/banned/deleted).
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET status = $1, updated_at = NOW() WHERE id = $2`,
		string(status), id)
	if err != nil {
		return fmt.Errorf("user update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateRole sets a user's role (user/admin).
func (r *PostgresRepository) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`,
		role, id)
	if err != nil {
		return fmt.Errorf("user update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateProfile updates the user's display name, phone, and avatar URL.
func (r *PostgresRepository) UpdateProfile(ctx context.Context, id uuid.UUID, displayName, phone, avatarURL string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET display_name = $1, phone = $2, avatar_url = $3, updated_at = NOW() WHERE id = $4`,
		displayName, phone, avatarURL, id)
	if err != nil {
		return fmt.Errorf("user update profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePassword replaces the user's password hash.
func (r *PostgresRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		passwordHash, id)
	if err != nil {
		return fmt.Errorf("user update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the number of users, optionally narrowed by a user_type filter.
func (r *PostgresRepository) Count(ctx context.Context, filter ListFilter) (int, error) {
	query := `SELECT COUNT(*) FROM users`
	args := make([]any, 0, 1)
	conds := make([]string, 0, 2)
	if filter.UserType != "" {
		args = append(args, string(filter.UserType))
		conds = append(conds, fmt.Sprintf(`user_type = $%d`, len(args)))
	}
	if filter.ExcludeDeleted {
		conds = append(conds, `status <> 'deleted'`)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	var count int
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("user count: %w", err)
	}
	return count, nil
}
