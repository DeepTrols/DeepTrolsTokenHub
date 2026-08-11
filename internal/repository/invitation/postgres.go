package invitation

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// scanInvitation scans a tenant_invitations row into a domain.TenantInvitation.
// Column order: id, tenant_id, invited_by, email, role, token, status, expires_at, created_at
func scanInvitation(row pgx.Row) (*domain.TenantInvitation, error) {
	var inv domain.TenantInvitation
	err := row.Scan(
		&inv.ID,
		&inv.TenantID,
		&inv.InvitedBy,
		&inv.Email,
		&inv.Role,
		&inv.Token,
		&inv.Status,
		&inv.ExpiresAt,
		&inv.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("invitation scan: %w", err)
	}
	return &inv, nil
}

const invitationSelectClause = `
	id, tenant_id, invited_by, email, role, token, status, expires_at, created_at
`

// FindByToken looks up an invitation by its unique token.
func (r *PostgresRepository) FindByToken(ctx context.Context, token string) (*domain.TenantInvitation, error) {
	query := "SELECT " + invitationSelectClause + " FROM tenant_invitations WHERE token = $1"
	inv, err := scanInvitation(r.pool.QueryRow(ctx, query, token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("invitation find by token: %w", err)
	}
	return inv, nil
}

// FindPendingByEmail returns pending invitations for an email address.
func (r *PostgresRepository) FindPendingByEmail(ctx context.Context, email string) ([]domain.TenantInvitation, error) {
	query := "SELECT " + invitationSelectClause + ` FROM tenant_invitations WHERE email = $1 AND status = 'pending' ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, email)
	if err != nil {
		return nil, fmt.Errorf("invitation list by email: %w", err)
	}
	defer rows.Close()

	var invitations []domain.TenantInvitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, *inv)
	}
	return invitations, rows.Err()
}

// ListByTenantID returns all invitations for a tenant.
func (r *PostgresRepository) ListByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantInvitation, error) {
	query := "SELECT " + invitationSelectClause + " FROM tenant_invitations WHERE tenant_id = $1 ORDER BY created_at DESC"
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("invitation list by tenant: %w", err)
	}
	defer rows.Close()

	var invitations []domain.TenantInvitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		invitations = append(invitations, *inv)
	}
	return invitations, rows.Err()
}

// Create inserts a new invitation.
func (r *PostgresRepository) Create(ctx context.Context, inv *domain.TenantInvitation) error {
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now()
	}

	const query = `
		INSERT INTO tenant_invitations (id, tenant_id, invited_by, email, role, token, status, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.pool.Exec(ctx, query,
		inv.ID, inv.TenantID, inv.InvitedBy, inv.Email, inv.Role,
		inv.Token, inv.Status, inv.ExpiresAt, inv.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("invitation create: %w", err)
	}
	return nil
}

// UpdateStatus changes the invitation status.
func (r *PostgresRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.InvitationStatus) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tenant_invitations SET status = $1 WHERE id = $2`,
		string(status), id)
	if err != nil {
		return fmt.Errorf("invitation update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ExpirePending sets expired status on all pending invitations past their expiry.
func (r *PostgresRepository) ExpirePending(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE tenant_invitations SET status = 'expired' WHERE status = 'pending' AND expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("invitation expire pending: %w", err)
	}
	return tag.RowsAffected(), nil
}
