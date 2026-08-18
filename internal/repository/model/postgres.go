package model

import (
	"context"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements both Repository and PricingRepository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var (
	_ Repository        = (*PostgresRepository)(nil)
	_ PricingRepository = (*PostgresRepository)(nil)
)

// ListActive returns all active (including beta) models.
func (r *PostgresRepository) ListActive(ctx context.Context) ([]domain.Model, error) {
	const query = `
		SELECT id, code, provider, category, display_name, description,
			context_window, max_output_tokens, capabilities,
			status, release_stage, created_at, updated_at
		FROM models WHERE status IN ('active', 'beta') ORDER BY code
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("model list active: %w", err)
	}
	defer rows.Close()

	return scanModelRows(rows)
}

// FindByID retrieves a single model by its UUID primary key.
func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Model, error) {
	const query = `
		SELECT id, code, provider, category, display_name, description,
			context_window, max_output_tokens, capabilities,
			status, release_stage, created_at, updated_at
		FROM models WHERE id = $1
	`
	return scanModelRow(r.pool.QueryRow(ctx, query, id))
}

// FindByCode retrieves a single model by its unique code.
func (r *PostgresRepository) FindByCode(ctx context.Context, code string) (*domain.Model, error) {
	const query = `
		SELECT id, code, provider, category, display_name, description,
			context_window, max_output_tokens, capabilities,
			status, release_stage, created_at, updated_at
		FROM models WHERE code = $1
	`
	return scanModelRow(r.pool.QueryRow(ctx, query, code))
}

// ListByTenant returns models with their tenant-specific configuration using LEFT JOIN.
func (r *PostgresRepository) ListByTenant(ctx context.Context, tenantID *uuid.UUID) ([]TenantModelView, error) {
	baseQuery := `
		SELECT
			m.id, m.code, m.provider, m.category, m.display_name, m.description,
			m.context_window, m.max_output_tokens, m.capabilities,
			m.status, m.release_stage, m.created_at, m.updated_at,
			tm.id, tm.tenant_id, tm.model_id,
			tm.is_listed, tm.allow_payg, tm.quota_enabled,
			tm.created_at, tm.updated_at
		FROM models m
		LEFT JOIN tenant_models tm ON tm.model_id = m.id
	`
	var args []any
	if tenantID != nil {
		baseQuery += " AND tm.tenant_id = $1"
		args = append(args, *tenantID)
	}
	baseQuery += " WHERE m.status IN ('active', 'beta') ORDER BY m.code"

	rows, err := r.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("model list by tenant: %w", err)
	}
	defer rows.Close()

	views := make([]TenantModelView, 0)
	for rows.Next() {
		m, tm, err := scanModelWithTenant(rows)
		if err != nil {
			return nil, fmt.Errorf("model list by tenant scan: %w", err)
		}
		views = append(views, TenantModelView{
			Model:       *m,
			TenantModel: tm,
		})
	}
	return views, rows.Err()
}

// GetTenantModel retrieves the tenant_model link for a specific tenant and model code.
func (r *PostgresRepository) GetTenantModel(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error) {
	const query = `
		SELECT tm.id, tm.tenant_id, tm.model_id, tm.is_listed, tm.allow_payg,
			tm.quota_enabled, tm.created_at, tm.updated_at
		FROM tenant_models tm
		JOIN models m ON m.id = tm.model_id
		WHERE tm.tenant_id = $1 AND m.code = $2
	`
	row := r.pool.QueryRow(ctx, query, tenantID, modelCode)
	var tm domain.TenantModel
	err := row.Scan(&tm.ID, &tm.TenantID, &tm.ModelID, &tm.IsListed,
		&tm.AllowPayg, &tm.QuotaEnabled, &tm.CreatedAt, &tm.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("model tenant model not found: tenant=%s code=%s: %w", tenantID, modelCode, err)
		}
		return nil, fmt.Errorf("model get tenant model: %w", err)
	}
	return &tm, nil
}

// FindByModel returns pricing rows for a model, optionally filtered by tenant.
func (r *PostgresRepository) FindByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
	query := `
		SELECT id, model_id, tenant_id, request_type, pricing_dimension,
			unit_name, unit_price, currency, upstream_cost, price_version,
			conditions, is_active, created_at, updated_at
		FROM model_pricing
		WHERE model_id = $1 AND is_active = TRUE
	`
	args := []any{modelID}
	if tenantID != nil {
		query += " AND (tenant_id = $2 OR tenant_id IS NULL)"
		args = append(args, *tenantID)
	}
	query += " ORDER BY request_type, pricing_dimension"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("model find by model: %w", err)
	}
	defer rows.Close()

	var pricing []domain.ModelPricing
	for rows.Next() {
		var p domain.ModelPricing
		var unitPriceStr, currencyStr string
		var upstreamCostStr *string
		err := rows.Scan(&p.ID, &p.ModelID, &p.TenantID, &p.RequestType,
			&p.PricingDimension, &p.UnitName, &unitPriceStr, &currencyStr,
			&upstreamCostStr, &p.PriceVersion, &p.Conditions, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("model pricing scan: %w", err)
		}
		p.UnitPrice = unitPriceStr
		p.Currency = currencyStr
		if upstreamCostStr != nil {
			p.UpstreamCost = *upstreamCostStr
		}
		pricing = append(pricing, p)
	}
	return pricing, rows.Err()
}

// --- scan helpers ---

func scanModelRow(row pgx.Row) (*domain.Model, error) {
	var m domain.Model
	var description, displayName *string
	var contextWindow, maxOutputTokens *int
	err := row.Scan(
		&m.ID, &m.Code, &m.Provider, &m.Category, &displayName,
		&description, &contextWindow, &maxOutputTokens,
		&m.Capabilities, &m.Status, &m.ReleaseStage, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("model scan: %w", err)
	}
	if description != nil {
		m.Description = *description
	}
	if displayName != nil {
		m.DisplayName = *displayName
	}
	if contextWindow != nil {
		m.ContextWindow = *contextWindow
	}
	if maxOutputTokens != nil {
		m.MaxOutputTokens = *maxOutputTokens
	}
	return &m, nil
}

func scanModelRows(rows pgx.Rows) ([]domain.Model, error) {
	var models []domain.Model
	for rows.Next() {
		m, err := scanModelRow(rows)
		if err != nil {
			return nil, err
		}
		models = append(models, *m)
	}
	return models, rows.Err()
}

func scanModelWithTenant(row pgx.Row) (*domain.Model, *domain.TenantModel, error) {
	var m domain.Model
	var description, displayName *string
	var contextWindow, maxOutputTokens *int
	var tmID, tmTenantID, tmModelID *uuid.UUID
	var isListed, allowPayg, quotaEnabled *bool
	var tmCreatedAt, tmUpdatedAt *time.Time

	err := row.Scan(
		&m.ID, &m.Code, &m.Provider, &m.Category, &displayName,
		&description, &contextWindow, &maxOutputTokens,
		&m.Capabilities, &m.Status, &m.ReleaseStage, &m.CreatedAt, &m.UpdatedAt,
		&tmID, &tmTenantID, &tmModelID,
		&isListed, &allowPayg, &quotaEnabled,
		&tmCreatedAt, &tmUpdatedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("model scan with tenant: %w", err)
	}
	if description != nil {
		m.Description = *description
	}
	if displayName != nil {
		m.DisplayName = *displayName
	}
	if contextWindow != nil {
		m.ContextWindow = *contextWindow
	}
	if maxOutputTokens != nil {
		m.MaxOutputTokens = *maxOutputTokens
	}

	var tm *domain.TenantModel
	if tmID != nil {
		tm = &domain.TenantModel{
			ID:           *tmID,
			TenantID:     *tmTenantID,
			ModelID:      *tmModelID,
			IsListed:     coalesceBool(isListed),
			AllowPayg:    coalesceBool(allowPayg),
			QuotaEnabled: coalesceBool(quotaEnabled),
			CreatedAt:    coalesceTime(tmCreatedAt),
			UpdatedAt:    coalesceTime(tmUpdatedAt),
		}
	}
	return &m, tm, nil
}

func coalesceBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func coalesceTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
