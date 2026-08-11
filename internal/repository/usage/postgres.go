package usage

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jsonb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

func (r *PostgresRepository) CreateUsageLog(ctx context.Context, log *domain.UsageLog) error {
	const query = `
		INSERT INTO usage_logs (
			id, tenant_id, user_id, api_key_id, request_id, request_type,
			public_model_code, upstream_model_code, channel_id, instance_id,
			route_policy_id, provider_request_id, usage_source, usage_raw,
			usage_normalized, estimated_cost, list_cost, discount_amount,
			final_cost, upstream_cost, currency, price_snapshot,
			quota_deducted, wallet_charged, status, error_code, error_message,
			request_summary, response_summary, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26, $27,
			$28, $29, $30
		)
	`
	_, err := r.pool.Exec(ctx, query,
		log.ID, log.TenantID, log.UserID, log.APIKeyID, log.RequestID, log.RequestType,
		log.PublicModelCode, log.UpstreamModelCode, log.ChannelID, log.InstanceID,
		log.RoutePolicyID, log.ProviderRequestID, log.UsageSource,
		jsonb.Marshal(log.UsageRaw), jsonb.Marshal(log.UsageNormalized),
		log.EstimatedCost, log.ListCost, log.DiscountAmount,
		log.FinalCost, log.UpstreamCost, log.Currency, jsonb.Marshal(log.PriceSnapshot),
		log.QuotaDeducted, log.WalletCharged, log.Status, log.ErrorCode, log.ErrorMessage,
		log.RequestSummary, log.ResponseSummary, log.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("usage log create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CreateChargeLines(ctx context.Context, lines []domain.ChargeLine) error {
	if len(lines) == 0 {
		return nil
	}
	const query = `
		INSERT INTO charge_lines (
			id, usage_log_id, dimension, unit_name, quantity,
			unit_price, line_cost, discount_applied, price_source,
			price_version, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	batch := &pgx.Batch{}
	for i := range lines {
		l := &lines[i]
		batch.Queue(query,
			l.ID, l.UsageLogID, l.Dimension, l.UnitName, l.Quantity,
			l.UnitPrice, l.LineCost, l.DiscountApplied, l.PriceSource,
			l.PriceVersion, l.CreatedAt,
		)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range lines {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("usage charge lines create: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) CreateProviderEvidence(ctx context.Context, evidence *domain.ProviderEvidence) error {
	const query = `
		INSERT INTO provider_evidence (
			id, usage_log_id, provider, provider_request_id,
			request_body, response_body, status_code, duration_ms,
			usage_raw, provider_cost, provider_currency, error_message, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err := r.pool.Exec(ctx, query,
		evidence.ID, evidence.UsageLogID, evidence.Provider, evidence.ProviderRequestID,
		jsonb.Marshal(evidence.RequestBody), jsonb.Marshal(evidence.ResponseBody),
		evidence.StatusCode, evidence.DurationMs,
		jsonb.Marshal(evidence.UsageRaw),
		evidence.ProviderCost, evidence.ProviderCurrency, evidence.ErrorMessage,
		evidence.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("provider evidence create: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByRequestID(ctx context.Context, requestID string) (*domain.UsageLog, error) {
	query := "SELECT " + usageLogSelectClause + " FROM usage_logs WHERE request_id = $1"
	return scanUsageLog(r.pool.QueryRow(ctx, query, requestID))
}

func (r *PostgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, filter UsageFilter) ([]domain.UsageLog, int, error) {
	return listUsageLogs(ctx, r.pool, "user_id", userID, filter)
}

func (r *PostgresRepository) ListByAPIKey(ctx context.Context, apiKeyID uuid.UUID, filter UsageFilter) ([]domain.UsageLog, int, error) {
	return listUsageLogs(ctx, r.pool, "api_key_id", apiKeyID, filter)
}

// usageLogSelectClause selects all columns scanned by scanUsageLog.
// Nullable string/decimal columns (upstream_model_code, provider_request_id,
// estimated_cost, upstream_cost, error_code/message, request/response_summary)
// are COALESCE'd so a log row with routing/error details unrecorded scans into
// zero values instead of failing with "cannot scan NULL into *string".
// channel_id/instance_id/route_policy_id/tenant_id are *uuid.UUID pointers and
// tolerate NULL natively.
const usageLogSelectClause = `
	id, tenant_id, user_id, api_key_id, request_id, request_type,
	public_model_code, COALESCE(upstream_model_code, ''), channel_id, instance_id,
	route_policy_id, COALESCE(provider_request_id, ''), usage_source,
	COALESCE(usage_raw::text, '{}'), COALESCE(usage_normalized::text, '{}'),
	COALESCE(estimated_cost::text, ''), list_cost, discount_amount,
	final_cost, COALESCE(upstream_cost::text, ''), currency,
	COALESCE(price_snapshot::text, '{}'),
	quota_deducted, wallet_charged, status,
	COALESCE(error_code, ''), COALESCE(error_message, ''),
	COALESCE(request_summary, ''), COALESCE(response_summary, ''), created_at
`

func scanUsageLog(row pgx.Row) (*domain.UsageLog, error) {
	var l domain.UsageLog
	var usageRawJSON, usageNormJSON, priceSnapshotJSON string
	var estimatedCostStr, listCostStr, discountStr, finalCostStr, upstreamCostStr string
	var walletChargedStr string

	err := row.Scan(
		&l.ID, &l.TenantID, &l.UserID, &l.APIKeyID, &l.RequestID, &l.RequestType,
		&l.PublicModelCode, &l.UpstreamModelCode, &l.ChannelID, &l.InstanceID,
		&l.RoutePolicyID, &l.ProviderRequestID, &l.UsageSource,
		&usageRawJSON, &usageNormJSON,
		&estimatedCostStr, &listCostStr, &discountStr,
		&finalCostStr, &upstreamCostStr, &l.Currency,
		&priceSnapshotJSON,
		&l.QuotaDeducted, &walletChargedStr, &l.Status, &l.ErrorCode, &l.ErrorMessage,
		&l.RequestSummary, &l.ResponseSummary, &l.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("usage log scan: %w", err)
	}

	l.UsageRaw = jsonb.Unmarshal(usageRawJSON)
	l.UsageNormalized = jsonb.Unmarshal(usageNormJSON)
	l.PriceSnapshot = jsonb.Unmarshal(priceSnapshotJSON)
	l.EstimatedCost = parseDecimalStr(estimatedCostStr)
	l.ListCost = parseDecimalStr(listCostStr)
	l.DiscountAmount = parseDecimalStr(discountStr)
	l.FinalCost = parseDecimalStr(finalCostStr)
	l.UpstreamCost = parseDecimalStr(upstreamCostStr)
	l.WalletCharged = parseDecimalStr(walletChargedStr)

	return &l, nil
}

var allowedFilterColumns = map[string]bool{
	"user_id":    true,
	"api_key_id": true,
}

func listUsageLogs(ctx context.Context, pool *pgxpool.Pool, filterColumn string, filterValue any, f UsageFilter) ([]domain.UsageLog, int, error) {
	if !allowedFilterColumns[filterColumn] {
		return nil, 0, fmt.Errorf("usage list: invalid filter column: %s", filterColumn)
	}

	where := fmt.Sprintf("WHERE %s = $1", filterColumn)
	args := []any{filterValue}
	argIdx := 2

	if f.ModelCode != "" {
		where += fmt.Sprintf(" AND public_model_code = $%d", argIdx)
		args = append(args, f.ModelCode)
		argIdx++
	}
	if f.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.RequestID != "" {
		where += fmt.Sprintf(" AND request_id = $%d", argIdx)
		args = append(args, f.RequestID)
		argIdx++
	}
	if f.APIKeyID != "" {
		where += fmt.Sprintf(" AND api_key_id = $%d", argIdx)
		args = append(args, f.APIKeyID)
		argIdx++
	}
	if f.From != "" {
		t, err := time.Parse(time.RFC3339, f.From)
		if err == nil {
			where += fmt.Sprintf(" AND created_at >= $%d", argIdx)
			args = append(args, t)
			argIdx++
		}
	}
	if f.To != "" {
		t, err := time.Parse(time.RFC3339, f.To)
		if err == nil {
			where += fmt.Sprintf(" AND created_at <= $%d", argIdx)
			args = append(args, t)
			argIdx++
		}
	}

	countQuery := "SELECT COUNT(*) FROM usage_logs " + where
	var total int
	if err := pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("usage list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	dataQuery := fmt.Sprintf("SELECT %s FROM usage_logs %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		usageLogSelectClause, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("usage list query: %w", err)
	}
	defer rows.Close()

	var logs []domain.UsageLog
	for rows.Next() {
		l, err := scanUsageLog(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("usage list scan: %w", err)
		}
		logs = append(logs, *l)
	}
	return logs, total, rows.Err()
}

func parseDecimalStr(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		log.Printf("usage: failed to parse decimal %q, returning zero", v)
		return decimal.Zero
	}
	return d
}
