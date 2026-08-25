package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestLogger_Record_Success(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()

	var capturedLog *domain.UsageLog
	var capturedLines []domain.ChargeLine
	var capturedEvidence *domain.ProviderEvidence

	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error {
			capturedLog = log
			return nil
		},
		createChargeLinesFn: func(ctx context.Context, lines []domain.ChargeLine) error {
			capturedLines = lines
			return nil
		},
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error {
			capturedEvidence = evidence
			return nil
		},
	}

	l := NewLogger(repo)
	params := LogUsageParams{
		UserID:          userID,
		APIKeyID:        apiKeyID,
		RequestID:       "req-123",
		PublicModelCode: "gpt-4o",
		ChargeLines: []ChargeLineInput{
			{Dimension: "input", UnitName: "token", Quantity: 1000, UnitPrice: decimal.NewFromFloat(0.01), LineCost: decimal.NewFromFloat(10.0)},
			{Dimension: "output", UnitName: "token", Quantity: 500, UnitPrice: decimal.NewFromFloat(0.02), LineCost: decimal.NewFromFloat(10.0)},
		},
		ChargeLineSource: "platform",
		ChargeLineVer:    1,
		Provider:         "openai",
		ProviderReqID:    "prov-456",
		StatusCode:       200,
		DurationMs:       350,
		Status:           domain.UsageLogStatusCompleted,
	}

	result, err := l.Record(context.Background(), params)
	if err != nil {
		t.Fatalf("Record unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Record returned nil result")
	}
	if result.UsageLogID == uuid.Nil {
		t.Error("UsageLogID should not be nil UUID")
	}

	// Verify usage log
	if capturedLog == nil {
		t.Fatal("CreateUsageLog was not called")
	}
	if capturedLog.UserID != userID {
		t.Errorf("UsageLog.UserID = %s, want %s", capturedLog.UserID, userID)
	}
	if capturedLog.APIKeyID != apiKeyID {
		t.Errorf("UsageLog.APIKeyID = %s, want %s", capturedLog.APIKeyID, apiKeyID)
	}
	if capturedLog.PublicModelCode != "gpt-4o" {
		t.Errorf("UsageLog.PublicModelCode = %s, want gpt-4o", capturedLog.PublicModelCode)
	}
	if capturedLog.Status != domain.UsageLogStatusCompleted {
		t.Errorf("UsageLog.Status = %s, want %s", capturedLog.Status, domain.UsageLogStatusCompleted)
	}
	if capturedLog.CreatedAt.IsZero() {
		t.Error("UsageLog.CreatedAt should not be zero")
	}
	if capturedLog.CreatedAt.Location() != time.UTC {
		t.Error("UsageLog.CreatedAt should be UTC")
	}

	// Verify charge lines
	if len(capturedLines) != 2 {
		t.Fatalf("charge lines count = %d, want 2", len(capturedLines))
	}
	if capturedLines[0].Dimension != "input" {
		t.Errorf("charge line[0] dimension = %s, want input", capturedLines[0].Dimension)
	}
	if !capturedLines[0].LineCost.Equal(decimal.NewFromFloat(10.0)) {
		t.Errorf("charge line[0] line cost = %s, want 10", capturedLines[0].LineCost)
	}
	if capturedLines[0].PriceSource != "platform" {
		t.Errorf("charge line[0] price source = %s, want platform", capturedLines[0].PriceSource)
	}
	if capturedLines[0].PriceVersion != 1 {
		t.Errorf("charge line[0] price version = %d, want 1", capturedLines[0].PriceVersion)
	}
	if capturedLines[0].UsageLogID != result.UsageLogID {
		t.Error("charge line UsageLogID should match the created usage log")
	}

	// Verify provider evidence
	if capturedEvidence == nil {
		t.Fatal("CreateProviderEvidence was not called")
	}
	if capturedEvidence.Provider != "openai" {
		t.Errorf("evidence.Provider = %s, want openai", capturedEvidence.Provider)
	}
	if capturedEvidence.StatusCode != 200 {
		t.Errorf("evidence.StatusCode = %d, want 200", capturedEvidence.StatusCode)
	}
	if capturedEvidence.DurationMs != 350 {
		t.Errorf("evidence.DurationMs = %d, want 350", capturedEvidence.DurationMs)
	}
	if capturedEvidence.UsageLogID == nil || *capturedEvidence.UsageLogID != result.UsageLogID {
		t.Error("evidence.UsageLogID should point to the created usage log")
	}
}

func TestLogger_Record_NoChargeLines(t *testing.T) {
	chargeLinesCalled := false
	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error { return nil },
		createChargeLinesFn: func(ctx context.Context, lines []domain.ChargeLine) error {
			chargeLinesCalled = true
			return nil
		},
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error { return nil },
	}

	l := NewLogger(repo)
	_, err := l.Record(context.Background(), LogUsageParams{
		UserID:          uuid.New(),
		APIKeyID:        uuid.New(),
		PublicModelCode: "claude-sonnet",
		Status:          domain.UsageLogStatusCompleted,
	})
	if err != nil {
		t.Fatalf("Record unexpected error: %v", err)
	}
	if chargeLinesCalled {
		t.Error("CreateChargeLines should NOT be called when ChargeLines is empty")
	}
}

func TestLogger_Record_CreateUsageLogFails(t *testing.T) {
	chargeLinesCalled := false
	evidenceCalled := false
	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error {
			return errors.New("db connection lost")
		},
		createChargeLinesFn: func(ctx context.Context, lines []domain.ChargeLine) error {
			chargeLinesCalled = true
			return nil
		},
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error {
			evidenceCalled = true
			return nil
		},
	}

	l := NewLogger(repo)
	_, err := l.Record(context.Background(), LogUsageParams{
		UserID:   uuid.New(),
		APIKeyID: uuid.New(),
		ChargeLines: []ChargeLineInput{
			{Dimension: "input", UnitName: "token", Quantity: 100, UnitPrice: decimal.NewFromFloat(0.01), LineCost: decimal.NewFromFloat(1.0)},
		},
	})
	if err == nil {
		t.Fatal("expected error from CreateUsageLog")
	}
	if chargeLinesCalled {
		t.Error("CreateChargeLines should NOT be called after CreateUsageLog fails")
	}
	if evidenceCalled {
		t.Error("CreateProviderEvidence should NOT be called after CreateUsageLog fails")
	}
}

func TestLogger_Record_CreateChargeLinesFails(t *testing.T) {
	evidenceCalled := false
	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error { return nil },
		createChargeLinesFn: func(ctx context.Context, lines []domain.ChargeLine) error {
			return errors.New("charge lines insert failed")
		},
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error {
			evidenceCalled = true
			return nil
		},
	}

	l := NewLogger(repo)
	_, err := l.Record(context.Background(), LogUsageParams{
		UserID:   uuid.New(),
		APIKeyID: uuid.New(),
		ChargeLines: []ChargeLineInput{
			{Dimension: "input", UnitName: "token", Quantity: 100, UnitPrice: decimal.NewFromFloat(0.01), LineCost: decimal.NewFromFloat(1.0)},
		},
	})
	if err == nil {
		t.Fatal("expected error from CreateChargeLines")
	}
	// Known: usage log is orphaned — no transactional boundary in Logger.Record.
	if evidenceCalled {
		t.Error("CreateProviderEvidence should NOT be called after CreateChargeLines fails")
	}
}

func TestLogger_Record_CreateProviderEvidenceFails(t *testing.T) {
	repo := &mockUsageRepo{
		createUsageLogFn:    func(ctx context.Context, log *domain.UsageLog) error { return nil },
		createChargeLinesFn: func(ctx context.Context, lines []domain.ChargeLine) error { return nil },
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error {
			return errors.New("evidence insert failed")
		},
	}

	l := NewLogger(repo)
	_, err := l.Record(context.Background(), LogUsageParams{
		UserID:   uuid.New(),
		APIKeyID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected error from CreateProviderEvidence")
	}
}

func TestLogger_Record_FailedStatus(t *testing.T) {
	var capturedLog *domain.UsageLog
	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error {
			capturedLog = log
			return nil
		},
		createChargeLinesFn:      func(ctx context.Context, lines []domain.ChargeLine) error { return nil },
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error { return nil },
	}

	l := NewLogger(repo)
	_, err := l.Record(context.Background(), LogUsageParams{
		UserID:       uuid.New(),
		APIKeyID:     uuid.New(),
		Status:       domain.UsageLogStatusFailed,
		ErrorCode:    "UPSTREAM_500",
		ErrorMessage: "upstream returned 500",
	})
	if err != nil {
		t.Fatalf("Record unexpected error: %v", err)
	}
	if capturedLog.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", capturedLog.Status, domain.UsageLogStatusFailed)
	}
	if capturedLog.ErrorCode != "UPSTREAM_500" {
		t.Errorf("ErrorCode = %s, want UPSTREAM_500", capturedLog.ErrorCode)
	}
}

func TestLogger_Record_DomainMapping(t *testing.T) {
	var capturedLog *domain.UsageLog
	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error {
			capturedLog = log
			return nil
		},
		createChargeLinesFn:      func(ctx context.Context, lines []domain.ChargeLine) error { return nil },
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error { return nil },
	}

	tenantID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()

	l := NewLogger(repo)
	params := LogUsageParams{
		TenantID:          &tenantID,
		UserID:            uuid.New(),
		APIKeyID:          uuid.New(),
		RequestID:         "req-mapping",
		RequestType:       "chat",
		PublicModelCode:   "gpt-4o",
		UpstreamModelCode: "gpt-4o-azure",
		ChannelID:         &channelID,
		InstanceID:        &instanceID,
		ProviderRequestID: "prov-map-001",
		UsageSource:       domain.UsageSourceUpstream,
		UsageRaw:          map[string]any{"prompt_tokens": float64(500)},
		UsageNormalized:   map[string]any{"input_tokens": float64(500)},
		EstimatedCost:     decimal.NewFromFloat(0.005),
		ListCost:          decimal.NewFromFloat(0.005),
		DiscountAmount:    decimal.Zero,
		FinalCost:         decimal.NewFromFloat(0.005),
		UpstreamCost:      decimal.NewFromFloat(0.003),
		Currency:          "CNY",
		PriceSnapshot:     map[string]any{"model": "gpt-4o"},
		QuotaDeducted:     500,
		WalletCharged:     decimal.NewFromFloat(0.005),
		Status:            domain.UsageLogStatusCompleted,
		RequestSummary:    "Hello, how are you?",
		ResponseSummary:   "I'm fine, thank you!",
	}

	_, err := l.Record(context.Background(), params)
	if err != nil {
		t.Fatalf("Record unexpected error: %v", err)
	}

	if capturedLog.TenantID == nil || *capturedLog.TenantID != tenantID {
		t.Error("TenantID not mapped correctly")
	}
	if capturedLog.RequestType != "chat" {
		t.Errorf("RequestType = %s, want chat", capturedLog.RequestType)
	}
	if capturedLog.UpstreamModelCode != "gpt-4o-azure" {
		t.Errorf("UpstreamModelCode = %s, want gpt-4o-azure", capturedLog.UpstreamModelCode)
	}
	if capturedLog.ChannelID == nil || *capturedLog.ChannelID != channelID {
		t.Error("ChannelID not mapped correctly")
	}
	if capturedLog.InstanceID == nil || *capturedLog.InstanceID != instanceID {
		t.Error("InstanceID not mapped correctly")
	}
	if capturedLog.UsageSource != domain.UsageSourceUpstream {
		t.Errorf("UsageSource = %s, want %s", capturedLog.UsageSource, domain.UsageSourceUpstream)
	}
	if !capturedLog.FinalCost.Equal(decimal.NewFromFloat(0.005)) {
		t.Errorf("FinalCost = %s, want 0.005", capturedLog.FinalCost)
	}
	if capturedLog.Currency != "CNY" {
		t.Errorf("Currency = %s, want CNY", capturedLog.Currency)
	}
	if capturedLog.QuotaDeducted != 500 {
		t.Errorf("QuotaDeducted = %d, want 500", capturedLog.QuotaDeducted)
	}
	if !capturedLog.WalletCharged.Equal(decimal.NewFromFloat(0.005)) {
		t.Errorf("WalletCharged = %s, want 0.005", capturedLog.WalletCharged)
	}
}

func TestLogger_Record_ChargeLinePerLineSourceVersion(t *testing.T) {
	var capturedLines []domain.ChargeLine
	repo := &mockUsageRepo{
		createUsageLogFn: func(ctx context.Context, log *domain.UsageLog) error { return nil },
		createChargeLinesFn: func(ctx context.Context, lines []domain.ChargeLine) error {
			capturedLines = lines
			return nil
		},
		createProviderEvidenceFn: func(ctx context.Context, evidence *domain.ProviderEvidence) error { return nil },
	}

	l := NewLogger(repo)
	params := LogUsageParams{
		UserID:          uuid.New(),
		APIKeyID:        uuid.New(),
		PublicModelCode: "gpt-4o",
		ChargeLines: []ChargeLineInput{
			{
				Dimension: "input", UnitName: "token", Quantity: 100,
				UnitPrice: decimal.NewFromFloat(0.01), LineCost: decimal.NewFromFloat(1.0),
				PriceSource: "model_pricing", PriceVersion: 7,
			},
			{
				Dimension: "output", UnitName: "token", Quantity: 50,
				UnitPrice: decimal.NewFromFloat(0.02), LineCost: decimal.NewFromFloat(1.0),
			},
		},
		ChargeLineSource: "platform",
		ChargeLineVer:    1,
		Status:           domain.UsageLogStatusCompleted,
	}

	_, err := l.Record(context.Background(), params)
	if err != nil {
		t.Fatalf("Record unexpected error: %v", err)
	}
	if len(capturedLines) != 2 {
		t.Fatalf("captured lines count = %d, want 2", len(capturedLines))
	}
	if capturedLines[0].PriceSource != "model_pricing" {
		t.Errorf("lines[0].PriceSource = %q, want per-line model_pricing", capturedLines[0].PriceSource)
	}
	if capturedLines[0].PriceVersion != 7 {
		t.Errorf("lines[0].PriceVersion = %d, want per-line 7", capturedLines[0].PriceVersion)
	}
	if capturedLines[1].PriceSource != "platform" {
		t.Errorf("lines[1].PriceSource = %q, want params-level fallback platform", capturedLines[1].PriceSource)
	}
	if capturedLines[1].PriceVersion != 1 {
		t.Errorf("lines[1].PriceVersion = %d, want params-level fallback 1", capturedLines[1].PriceVersion)
	}
}
