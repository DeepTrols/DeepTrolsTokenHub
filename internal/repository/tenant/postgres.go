package tenant

import (
	"context"
	"fmt"

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

var _ Repository = (*PostgresRepository)(nil)

// FindByID retrieves a tenant by its primary key.
func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	query := "SELECT " + tenantSelectClause + " FROM tenants WHERE id = $1"
	return scanTenant(r.pool.QueryRow(ctx, query, id))
}

// FindByCode retrieves a tenant by its unique code.
func (r *PostgresRepository) FindByCode(ctx context.Context, code string) (*domain.Tenant, error) {
	query := "SELECT " + tenantSelectClause + " FROM tenants WHERE code = $1"
	return scanTenant(r.pool.QueryRow(ctx, query, code))
}

// Create inserts a new tenant row.
func (r *PostgresRepository) Create(ctx context.Context, t *domain.Tenant) error {
	brandConfig := marshalJSONB(t.BrandConfig)
	runtimeConfig := marshalJSONB(t.RuntimeConfig)
	settlementConfig := marshalJSONB(t.SettlementConfig)

	const query = `
		INSERT INTO tenants (
			id, code, name, status, owner_id,
			brand_config, runtime_config, settlement_config,
			status_reason,
			credit_code, contact_email, contact_phone, business_license,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9,
			$10, $11, $12, $13,
			$14, $15
		)
	`

	_, err := r.pool.Exec(ctx, query,
		t.ID, t.Code, t.Name, t.Status, t.OwnerID,
		brandConfig, runtimeConfig, settlementConfig,
		t.StatusReason,
		t.CreditCode, t.ContactEmail, t.ContactPhone, t.BusinessLicense,
		t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("tenant create: %w", err)
	}
	return nil
}

// Update modifies an existing tenant.
func (r *PostgresRepository) Update(ctx context.Context, t *domain.Tenant) error {
	brandConfig := marshalJSONB(t.BrandConfig)
	runtimeConfig := marshalJSONB(t.RuntimeConfig)
	settlementConfig := marshalJSONB(t.SettlementConfig)

	const query = `
		UPDATE tenants SET
			name = $1, status = $2,
			brand_config = $3, runtime_config = $4, settlement_config = $5,
			status_reason = $6,
			credit_code = $7, contact_email = $8, contact_phone = $9, business_license = $10,
			updated_at = $11
		WHERE id = $12
	`

	_, err := r.pool.Exec(ctx, query,
		t.Name, t.Status,
		brandConfig, runtimeConfig, settlementConfig,
		t.StatusReason,
		t.CreditCode, t.ContactEmail, t.ContactPhone, t.BusinessLicense,
		t.UpdatedAt,
		t.ID,
	)
	if err != nil {
		return fmt.Errorf("tenant update: %w", err)
	}
	return nil
}

// List returns all tenants ordered by creation time.
func (r *PostgresRepository) List(ctx context.Context) ([]domain.Tenant, error) {
	query := "SELECT " + tenantSelectClause + " FROM tenants ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("tenant list: %w", err)
	}
	defer rows.Close()

	var tenants []domain.Tenant
	for rows.Next() {
		t, err := scanTenant(rows)
		if err != nil {
			return nil, fmt.Errorf("tenant list scan: %w", err)
		}
		tenants = append(tenants, *t)
	}
	return tenants, rows.Err()
}

// --- helpers ---

const tenantSelectClause = `
	id, code, name, status, owner_id,
	COALESCE(brand_config::text, '{}'),
	COALESCE(runtime_config::text, '{}'),
	COALESCE(settlement_config::text, '{}'),
	COALESCE(status_reason, ''),
	COALESCE(credit_code, ''),
	COALESCE(contact_email, ''),
	COALESCE(contact_phone, ''),
	COALESCE(business_license, ''),
	created_at, updated_at
`

func scanTenant(row pgx.Row) (*domain.Tenant, error) {
	var t domain.Tenant
	var brandJSON, runtimeJSON, settlementJSON string

	err := row.Scan(
		&t.ID, &t.Code, &t.Name, &t.Status, &t.OwnerID,
		&brandJSON, &runtimeJSON, &settlementJSON,
		&t.StatusReason,
		&t.CreditCode, &t.ContactEmail, &t.ContactPhone, &t.BusinessLicense,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("tenant scan: %w", err)
	}

	t.BrandConfig = unmarshalJSONB(brandJSON)
	t.RuntimeConfig = unmarshalJSONB(runtimeJSON)
	t.SettlementConfig = unmarshalJSONB(settlementJSON)

	return &t, nil
}
