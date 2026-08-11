package membership

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// scanMembership scans a tenant_memberships row into a domain.TenantMembership.
// Column order: id, tenant_id, user_id, role, status, joined_at, created_at, updated_at
func scanMembership(row pgx.Row) (*domain.TenantMembership, error) {
	var m domain.TenantMembership
	err := row.Scan(
		&m.ID,
		&m.TenantID,
		&m.UserID,
		&m.Role,
		&m.Status,
		&m.JoinedAt,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("membership scan: %w", err)
	}
	return &m, nil
}

const membershipSelectClause = `
	id, tenant_id, user_id, role, status, joined_at, created_at, updated_at
`

// FindByUserID returns the active membership for a user.
func (r *PostgresRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.TenantMembership, error) {
	query := "SELECT " + membershipSelectClause + " FROM tenant_memberships WHERE user_id = $1 AND status = 'active'"
	m, err := scanMembership(r.pool.QueryRow(ctx, query, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("membership find by user: %w", err)
	}
	return m, nil
}

// FindByTenantID returns all memberships for a tenant.
func (r *PostgresRepository) FindByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantMembership, error) {
	query := "SELECT " + membershipSelectClause + " FROM tenant_memberships WHERE tenant_id = $1 ORDER BY joined_at ASC"
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("membership list by tenant: %w", err)
	}
	defer rows.Close()

	var memberships []domain.TenantMembership
	for rows.Next() {
		m, err := scanMembership(rows)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, *m)
	}
	return memberships, rows.Err()
}

// FindByUserAndTenant returns a specific membership.
func (r *PostgresRepository) FindByUserAndTenant(ctx context.Context, userID, tenantID uuid.UUID) (*domain.TenantMembership, error) {
	query := "SELECT " + membershipSelectClause + " FROM tenant_memberships WHERE user_id = $1 AND tenant_id = $2"
	m, err := scanMembership(r.pool.QueryRow(ctx, query, userID, tenantID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("membership find by user and tenant: %w", err)
	}
	return m, nil
}

// Create inserts a new membership.
func (r *PostgresRepository) Create(ctx context.Context, m *domain.TenantMembership) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = now
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = now
	}

	const query = `
		INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status, joined_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.pool.Exec(ctx, query,
		m.ID, m.TenantID, m.UserID, m.Role, m.Status,
		m.JoinedAt, m.CreatedAt, m.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrAlreadyExists
		}
		return fmt.Errorf("membership create: %w", err)
	}
	return nil
}

// UpdateRole changes the membership role.
func (r *PostgresRepository) UpdateRole(ctx context.Context, id uuid.UUID, role domain.MembershipRole) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tenant_memberships SET role = $1, updated_at = NOW() WHERE id = $2`,
		string(role), id)
	if err != nil {
		return fmt.Errorf("membership update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStatus changes the membership status.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.MembershipStatus) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tenant_memberships SET status = $1, updated_at = NOW() WHERE id = $2`,
		string(status), id)
	if err != nil {
		return fmt.Errorf("membership update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a membership permanently.
func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tenant_memberships WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("membership delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
