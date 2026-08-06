package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/usage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// Logger persists usage records: usage_log, charge_lines, and provider_evidence.
type Logger struct {
	usageRepo usage.Repository
	pool      *pgxpool.Pool
}

// NewLogger creates a new Logger (for tests — no transaction support).
func NewLogger(usageRepo usage.Repository) *Logger {
	return &Logger{usageRepo: usageRepo}
}

// NewLoggerWithPool creates a Logger with DB transaction support.
func NewLoggerWithPool(usageRepo usage.Repository, pool *pgxpool.Pool) *Logger {
	return &Logger{usageRepo: usageRepo, pool: pool}
}

// LogUsageParams carries all data needed to record a completed request.
type LogUsageParams struct {
	TenantID          *uuid.UUID
	UserID            uuid.UUID
	APIKeyID          uuid.UUID
	RequestID         string
	RequestType       string
	PublicModelCode   string
	UpstreamModelCode string
	ChannelID         *uuid.UUID
	InstanceID        *uuid.UUID
	RoutePolicyID     *uuid.UUID
	ProviderRequestID string
	UsageSource       domain.UsageSource
	UsageRaw          map[string]any
	UsageNormalized   map[string]any
	EstimatedCost     decimal.Decimal
	ListCost          decimal.Decimal
	DiscountAmount    decimal.Decimal
	FinalCost         decimal.Decimal
	UpstreamCost      decimal.Decimal
	Currency          string
	PriceSnapshot     map[string]any
	QuotaDeducted     int64
	WalletCharged     decimal.Decimal
	Status            domain.UsageLogStatus
	ErrorCode         string
	ErrorMessage      string
	RequestSummary    string
	ResponseSummary   string

	// Charge lines to persist
	ChargeLines      []ChargeLineInput
	ChargeLineSource string
	ChargeLineVer    int

	// Provider evidence
	Provider         string
	ProviderReqID    string
	RequestBody      map[string]any
	ResponseBody     map[string]any
	StatusCode       int
	DurationMs       int
	ProviderCost     decimal.Decimal
	ProviderCurrency string
	ProviderErrMsg   string
}

// LogResult holds the IDs of persisted records.
type LogResult struct {
	UsageLogID uuid.UUID
}

// Record persists a usage log together with its charge lines and provider evidence.
// When a pool is configured, all three writes happen in a single DB transaction.
func (l *Logger) Record(ctx context.Context, params LogUsageParams) (*LogResult, error) {
	now := time.Now().UTC()
	logID := uuid.New()

	// Use a DB transaction when pool is available (prevents orphaned records).
	if l.pool != nil {
		tx, err := l.pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("logger record: begin tx: %w", err)
		}
		defer tx.Rollback(ctx)

		if err := l.usageRepo.CreateUsageLog(ctx, buildUsageLog(params, logID, now)); err != nil {
			return nil, fmt.Errorf("logger record: %w", err)
		}
		if err := l.insertChargeLines(ctx, params, logID, now); err != nil {
			return nil, err
		}
		if err := l.usageRepo.CreateProviderEvidence(ctx, buildEvidence(params, logID, now)); err != nil {
			return nil, fmt.Errorf("logger provider evidence: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("logger record: commit: %w", err)
		}
		return &LogResult{UsageLogID: logID}, nil
	}

	// Fallback: sequential writes (used in tests with mock repos).
	if err := l.usageRepo.CreateUsageLog(ctx, buildUsageLog(params, logID, now)); err != nil {
		return nil, fmt.Errorf("logger record: %w", err)
	}
	if err := l.insertChargeLines(ctx, params, logID, now); err != nil {
		return nil, err
	}
	if err := l.usageRepo.CreateProviderEvidence(ctx, buildEvidence(params, logID, now)); err != nil {
		return nil, fmt.Errorf("logger provider evidence: %w", err)
	}

	return &LogResult{UsageLogID: logID}, nil
}

func buildUsageLog(params LogUsageParams, logID uuid.UUID, now time.Time) *domain.UsageLog {
	return &domain.UsageLog{
		ID:                logID,
		TenantID:          params.TenantID,
		UserID:            params.UserID,
		APIKeyID:          params.APIKeyID,
		RequestID:         params.RequestID,
		RequestType:       params.RequestType,
		PublicModelCode:   params.PublicModelCode,
		UpstreamModelCode: params.UpstreamModelCode,
		ChannelID:         params.ChannelID,
		InstanceID:        params.InstanceID,
		RoutePolicyID:     params.RoutePolicyID,
		ProviderRequestID: params.ProviderRequestID,
		UsageSource:       params.UsageSource,
		UsageRaw:          params.UsageRaw,
		UsageNormalized:   params.UsageNormalized,
		EstimatedCost:     params.EstimatedCost,
		ListCost:          params.ListCost,
		DiscountAmount:    params.DiscountAmount,
		FinalCost:         params.FinalCost,
		UpstreamCost:      params.UpstreamCost,
		Currency:          params.Currency,
		PriceSnapshot:     params.PriceSnapshot,
		QuotaDeducted:     params.QuotaDeducted,
		WalletCharged:     params.WalletCharged,
		Status:            params.Status,
		ErrorCode:         params.ErrorCode,
		ErrorMessage:      params.ErrorMessage,
		RequestSummary:    params.RequestSummary,
		ResponseSummary:   params.ResponseSummary,
		CreatedAt:         now,
	}
}

func buildEvidence(params LogUsageParams, logID uuid.UUID, now time.Time) *domain.ProviderEvidence {
	return &domain.ProviderEvidence{
		ID:                uuid.New(),
		UsageLogID:        &logID,
		Provider:          params.Provider,
		ProviderRequestID: params.ProviderReqID,
		RequestBody:       params.RequestBody,
		ResponseBody:      params.ResponseBody,
		StatusCode:        params.StatusCode,
		DurationMs:        params.DurationMs,
		UsageRaw:          params.UsageRaw,
		ProviderCost:      params.ProviderCost,
		ProviderCurrency:  params.ProviderCurrency,
		ErrorMessage:      params.ProviderErrMsg,
		CreatedAt:         now,
	}
}

func (l *Logger) insertChargeLines(ctx context.Context, params LogUsageParams, logID uuid.UUID, now time.Time) error {
	if len(params.ChargeLines) == 0 {
		return nil
	}
	lines := make([]domain.ChargeLine, len(params.ChargeLines))
	for i, cl := range params.ChargeLines {
		lines[i] = domain.ChargeLine{
			ID:              uuid.New(),
			UsageLogID:      logID,
			Dimension:       cl.Dimension,
			UnitName:        cl.UnitName,
			Quantity:        cl.Quantity,
			UnitPrice:       cl.UnitPrice,
			LineCost:        cl.LineCost,
			DiscountApplied: cl.DiscountApplied,
			PriceSource:     params.ChargeLineSource,
			PriceVersion:    params.ChargeLineVer,
			CreatedAt:       now,
		}
	}
	if err := l.usageRepo.CreateChargeLines(ctx, lines); err != nil {
		return fmt.Errorf("logger charge lines: %w", err)
	}
	return nil
}
