package apikey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
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

// Create inserts a new API key row.
// pgx handles Go []string -> PG TEXT[] natively in binary mode.
func (r *PostgresRepository) Create(ctx context.Context, key *domain.APIKey) error {
	allowedModels := ensureStringSlice(key.AllowedModels)
	sourceWhitelist := ensureStringSlice(key.SourceWhitelist)

	const query = `
		INSERT INTO api_keys (
			id, user_id, tenant_id, key_prefix, key_hash, encrypted_key, masked_key,
			name, status, allowed_models, source_whitelist,
			cumulative_limit, weekly_limit, monthly_limit,
			over_limit_action, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16, $17
		)
	`

	_, err := r.pool.Exec(ctx, query,
		key.ID, key.UserID, key.TenantID, key.KeyPrefix, key.KeyHash, key.EncryptedKey, key.MaskedKey,
		key.Name, key.Status, allowedModels, sourceWhitelist,
		key.CumulativeLimit, key.WeeklyLimit, key.MonthlyLimit,
		key.OverLimitAction, key.CreatedAt, key.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("apikey create: %w", err)
	}
	return nil
}

// ensureStringSlice returns a non-nil []string so pgx binds it properly.
func ensureStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// scanKey scans an api_keys row. Arrays are returned as JSON text via array_to_json
// and scanned into strings, then unmarshaled back to []string.
//
// Column order (20 columns):
//
//	id, user_id, tenant_id, key_prefix, key_hash, encrypted_key, masked_key,
//	name, status, allowed_models_json, source_whitelist_json,
//	cumulative_limit, weekly_limit, monthly_limit,
//	over_limit_action, last_used_at, last_7d_active,
//	created_at, updated_at, revoked_at
//
// Nullable columns are COALESCE'd in apiKeySelectClause so a key with no
// configured limits (or no name/tenant) scans into its zero value instead of
// failing with "cannot scan NULL into *string".
func scanKey(row pgx.Row) (*domain.APIKey, error) {
	var k domain.APIKey
	var allowedJSON, whitelistJSON string
	var cumulativeLimitStr, weeklyLimitStr, monthlyLimitStr string

	err := row.Scan(
		&k.ID, &k.UserID, &k.TenantID,
		&k.KeyPrefix, &k.KeyHash, &k.EncryptedKey, &k.MaskedKey,
		&k.Name, &k.Status,
		&allowedJSON, &whitelistJSON,
		&cumulativeLimitStr, &weeklyLimitStr, &monthlyLimitStr,
		&k.OverLimitAction, &k.LastUsedAt, &k.Last7dActive,
		&k.CreatedAt, &k.UpdatedAt, &k.RevokedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("apikey scan: %w", err)
	}

	k.AllowedModels = parseJSONStringArray(allowedJSON)
	k.SourceWhitelist = parseJSONStringArray(whitelistJSON)
	k.CumulativeLimit = parseDecimal(cumulativeLimitStr)
	k.WeeklyLimit = parseDecimal(weeklyLimitStr)
	k.MonthlyLimit = parseDecimal(monthlyLimitStr)

	return &k, nil
}

// parseJSONStringArray parses a JSON array of strings.
func parseJSONStringArray(raw string) []string {
	if raw == "" || raw == "[]" || raw == "null" {
		return []string{}
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return []string{}
	}
	return arr
}

// apiKeySelectClause is the common SELECT clause used by all read queries.
// Uses array_to_json to convert TEXT[] to JSON text for reliable scanning.
// name/limits are nullable in the schema, so they are COALESCE'd to keep the
// scan NULL-safe (a key created without limits legitimately has NULL here).
const apiKeySelectClause = `
		id, user_id, tenant_id, key_prefix, key_hash,
		COALESCE(encrypted_key, ''), masked_key,
		COALESCE(name, ''), status,
		COALESCE(array_to_json(allowed_models)::text, '[]'),
		COALESCE(array_to_json(source_whitelist)::text, '[]'),
		COALESCE(cumulative_limit::text, ''), COALESCE(weekly_limit::text, ''), COALESCE(monthly_limit::text, ''),
		over_limit_action, last_used_at, last_7d_active,
		created_at, updated_at, revoked_at
	`

// FindByHash retrieves an API key by its hash.
func (r *PostgresRepository) FindByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	query := "SELECT " + apiKeySelectClause + " FROM api_keys WHERE key_hash = $1"
	return scanKey(r.pool.QueryRow(ctx, query, keyHash))
}

// FindByID retrieves an API key by its ID.
func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	query := "SELECT " + apiKeySelectClause + " FROM api_keys WHERE id = $1"
	return scanKey(r.pool.QueryRow(ctx, query, id))
}

// ListByUser returns all API keys belonging to a user, optionally filtered by tenant.
func (r *PostgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) ([]domain.APIKey, error) {
	query := "SELECT " + apiKeySelectClause + " FROM api_keys WHERE user_id = $1"
	args := []any{userID}
	if tenantID != nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("apikey list: %w", err)
	}
	defer rows.Close()

	var keys []domain.APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, fmt.Errorf("apikey list scan: %w", err)
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

// Update modifies an existing API key.
func (r *PostgresRepository) Update(ctx context.Context, key *domain.APIKey) error {
	allowedModels := ensureStringSlice(key.AllowedModels)
	sourceWhitelist := ensureStringSlice(key.SourceWhitelist)

	const query = `
		UPDATE api_keys SET
			name = $1, status = $2, allowed_models = $3, source_whitelist = $4,
			cumulative_limit = $5, weekly_limit = $6, monthly_limit = $7,
			over_limit_action = $8, key_hash = $12, encrypted_key = $13, updated_at = $9, revoked_at = $10
		WHERE id = $11
	`
	_, err := r.pool.Exec(ctx, query,
		key.Name, key.Status, allowedModels, sourceWhitelist,
		key.CumulativeLimit, key.WeeklyLimit, key.MonthlyLimit,
		key.OverLimitAction, key.UpdatedAt, key.RevokedAt,
		key.ID, key.KeyHash, key.EncryptedKey,
	)
	if err != nil {
		return fmt.Errorf("apikey update: %w", err)
	}
	return nil
}

// UpdateLastUsed updates the last_used_at and last_7d_active fields.
func (r *PostgresRepository) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	const query = `
		UPDATE api_keys SET last_used_at = NOW(), last_7d_active = TRUE, updated_at = NOW()
		WHERE id = $1
	`
	_, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("apikey update last used: %w", err)
	}
	return nil
}

// GetSpend retrieves aggregated spend for a key and period type.
// Returns zero spend when no records exist (not an error).
func (r *PostgresRepository) GetSpend(ctx context.Context, keyID uuid.UUID, periodType string) (*domain.APIKeySpend, error) {
	const query = `
		SELECT id, api_key_id, period_type, period_start, period_end,
			total_cost, updated_at
		FROM api_key_spend
		WHERE api_key_id = $1 AND period_type = $2
		ORDER BY period_start DESC NULLS LAST
		LIMIT 1
	`
	row := r.pool.QueryRow(ctx, query, keyID, periodType)
	var s domain.APIKeySpend
	var totalStr string
	err := row.Scan(&s.ID, &s.APIKeyID, &s.PeriodType, &s.PeriodStart, &s.PeriodEnd,
		&totalStr, &s.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &domain.APIKeySpend{
				APIKeyID:   keyID,
				PeriodType: periodType,
				TotalCost:  decimal.Zero,
			}, nil
		}
		return nil, fmt.Errorf("apikey get spend: %w", err)
	}
	s.TotalCost = parseDecimal(totalStr)
	return &s, nil
}

// UpdateSpend upserts spend data: adds the total_cost to the existing cumulative value.
// Uses a sentinel period_start (epoch) for NULL values so the unique constraint always fires.
func (r *PostgresRepository) UpdateSpend(ctx context.Context, spend *domain.APIKeySpend) error {
	periodStart := spend.PeriodStart
	if periodStart == nil {
		sentinel := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		periodStart = &sentinel
	}

	const query = `
		INSERT INTO api_key_spend (api_key_id, period_type, period_start, period_end, total_cost, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (api_key_id, period_type, period_start)
		DO UPDATE SET total_cost = api_key_spend.total_cost + EXCLUDED.total_cost,
		              updated_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query,
		spend.APIKeyID, spend.PeriodType, periodStart, spend.PeriodEnd,
		spend.TotalCost,
	)
	if err != nil {
		return fmt.Errorf("apikey update spend: %w", err)
	}
	return nil
}

// parseDecimal converts a pgx decimal string/numeric to decimal.Decimal.
func parseDecimal(v any) decimal.Decimal {
	switch val := v.(type) {
	case string:
		d, err := decimal.NewFromString(val)
		if err != nil {
			return decimal.Zero
		}
		return d
	case []byte:
		d, err := decimal.NewFromString(string(val))
		if err != nil {
			return decimal.Zero
		}
		return d
	case float64:
		return decimal.NewFromFloat(val)
	case int64:
		return decimal.NewFromInt(val)
	default:
		return decimal.Zero
	}
}

// nowUTC returns the current UTC time. Extracted for testing if needed.
func nowUTC() time.Time {
	return time.Now().UTC()
}
