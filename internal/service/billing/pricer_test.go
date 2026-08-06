package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestPricer_Calculate_SingleDimension(t *testing.T) {
	modelID := uuid.New()
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				makePricingEntry("input", "0.01", "0.005"),
			}, nil
		},
	}

	p := NewPricer(repo)
	usage := &usageparser.NormalizedUsage{InputTokens: 1000}

	result, err := p.Calculate(context.Background(), modelID, nil, usage)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	expected := decimal.NewFromFloat(10.0) // 1000 * 0.01
	if !result.ListCost.Equal(expected) {
		t.Errorf("ListCost = %s, want %s", result.ListCost, expected)
	}
	expectedUp := decimal.NewFromFloat(5.0) // 1000 * 0.005
	if !result.UpstreamCost.Equal(expectedUp) {
		t.Errorf("UpstreamCost = %s, want %s", result.UpstreamCost, expectedUp)
	}
	if len(result.ChargeLines) != 1 {
		t.Fatalf("ChargeLines count = %d, want 1", len(result.ChargeLines))
	}
	if result.ChargeLines[0].Dimension != "input" {
		t.Errorf("Dimension = %s, want input", result.ChargeLines[0].Dimension)
	}
	if result.ChargeLines[0].Quantity != 1000 {
		t.Errorf("Quantity = %d, want 1000", result.ChargeLines[0].Quantity)
	}
}

func TestPricer_Calculate_MultiDimension(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				makePricingEntry("input", "0.01", "0.005"),
				makePricingEntry("output", "0.03", "0.015"),
			}, nil
		},
	}

	p := NewPricer(repo)
	usage := &usageparser.NormalizedUsage{InputTokens: 1000, OutputTokens: 500}

	result, err := p.Calculate(context.Background(), uuid.New(), nil, usage)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	// 1000*0.01 + 500*0.03 = 10 + 15 = 25
	expected := decimal.NewFromFloat(25.0)
	if !result.ListCost.Equal(expected) {
		t.Errorf("ListCost = %s, want %s", result.ListCost, expected)
	}
	// 1000*0.005 + 500*0.015 = 5 + 7.5 = 12.5
	expectedUp := decimal.NewFromFloat(12.5)
	if !result.UpstreamCost.Equal(expectedUp) {
		t.Errorf("UpstreamCost = %s, want %s", result.UpstreamCost, expectedUp)
	}
	if len(result.ChargeLines) != 2 {
		t.Fatalf("ChargeLines count = %d, want 2", len(result.ChargeLines))
	}
}

func TestPricer_Calculate_AllDimensions(t *testing.T) {
	allDims := []string{"input", "output", "cache_read", "cache_write", "reasoning", "image", "audio", "tts", "video"}
	pricing := make([]domain.ModelPricing, len(allDims))
	for i, dim := range allDims {
		pricing[i] = makePricingEntry(dim, "0.01", "0.005")
	}

	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return pricing, nil
		},
	}

	p := NewPricer(repo)
	usage := &usageparser.NormalizedUsage{
		InputTokens: 100, OutputTokens: 200, CacheReadTokens: 50, CacheWriteTokens: 25,
		ReasoningTokens: 75, ImageCount: 2, AudioSeconds: 60, TTSCharacters: 1000, VideoUnits: 1,
	}

	result, err := p.Calculate(context.Background(), uuid.New(), nil, usage)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	// Total quantity = 100+200+50+25+75+2+60+1000+1 = 1513
	// ListCost = 1513 * 0.01 = 15.13
	expected := decimal.NewFromFloat(15.13)
	if !result.ListCost.Equal(expected) {
		t.Errorf("ListCost = %s, want %s", result.ListCost, expected)
	}
	if len(result.ChargeLines) != 9 {
		t.Errorf("ChargeLines count = %d, want 9", len(result.ChargeLines))
	}
}

func TestPricer_Calculate_SkipZeroQuantity(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				makePricingEntry("input", "0.01", "0.005"),
				makePricingEntry("output", "0.03", "0.015"),
			}, nil
		},
	}

	p := NewPricer(repo)
	usage := &usageparser.NormalizedUsage{InputTokens: 1000, OutputTokens: 0}

	result, err := p.Calculate(context.Background(), uuid.New(), nil, usage)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	if len(result.ChargeLines) != 1 {
		t.Fatalf("ChargeLines count = %d, want 1 (only input)", len(result.ChargeLines))
	}
	if result.ChargeLines[0].Dimension != "input" {
		t.Errorf("Dimension = %s, want input", result.ChargeLines[0].Dimension)
	}
}

func TestPricer_Calculate_SkipNoMatch(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				makePricingEntry("output", "0.03", "0.015"),
			}, nil
		},
	}

	p := NewPricer(repo)
	usage := &usageparser.NormalizedUsage{InputTokens: 1000}

	result, err := p.Calculate(context.Background(), uuid.New(), nil, usage)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	if !result.ListCost.IsZero() {
		t.Errorf("ListCost = %s, want 0 (no matching pricing)", result.ListCost)
	}
	if len(result.ChargeLines) != 0 {
		t.Errorf("ChargeLines count = %d, want 0", len(result.ChargeLines))
	}
}

func TestPricer_Calculate_InvalidUnitPrice(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				{ID: uuid.New(), ModelID: mID, PricingDimension: "input", UnitName: "token", UnitPrice: "not-a-number", UpstreamCost: "0.005", IsActive: true},
			}, nil
		},
	}

	p := NewPricer(repo)
	result, err := p.Calculate(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1000})
	if err != nil {
		t.Fatalf("Calculate should not error on invalid price (log+skip): %v", err)
	}
	if !result.ListCost.IsZero() {
		t.Errorf("ListCost = %s, want 0 (invalid price skipped)", result.ListCost)
	}
}

func TestPricer_Calculate_InvalidUpstreamCost(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				{ID: uuid.New(), ModelID: mID, PricingDimension: "input", UnitName: "token", UnitPrice: "0.01", UpstreamCost: "bad", IsActive: true},
			}, nil
		},
	}

	p := NewPricer(repo)
	result, err := p.Calculate(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1000})
	if err != nil {
		t.Fatalf("Calculate should not error on invalid upstream cost: %v", err)
	}
	if !result.ListCost.IsZero() {
		t.Errorf("ListCost = %s, want 0 (invalid upstream cost skipped)", result.ListCost)
	}
}

func TestPricer_Calculate_FindByModelError(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return nil, errors.New("database error")
		},
	}

	p := NewPricer(repo)
	_, err := p.Calculate(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 100})
	if err == nil {
		t.Fatal("expected error from repo")
	}
	// Known: error is NOT wrapped (inconsistent with Charger/Logger pattern)
}

func TestPricer_Calculate_EmptyPricing(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{}, nil
		},
	}

	p := NewPricer(repo)
	result, err := p.Calculate(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1000})
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if !result.ListCost.IsZero() {
		t.Errorf("ListCost = %s, want 0", result.ListCost)
	}
	if len(result.ChargeLines) != 0 {
		t.Errorf("ChargeLines count = %d, want 0", len(result.ChargeLines))
	}
}

func TestPricer_Calculate_UpstreamCost(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				makePricingEntry("input", "0.10", "0.04"),
				makePricingEntry("output", "0.30", "0.12"),
			}, nil
		},
	}

	p := NewPricer(repo)
	usage := &usageparser.NormalizedUsage{InputTokens: 100, OutputTokens: 200}
	result, err := p.Calculate(context.Background(), uuid.New(), nil, usage)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	expectedList := decimal.NewFromFloat(70.0) // 100*0.10 + 200*0.30
	expectedUp := decimal.NewFromFloat(28.0)   // 100*0.04 + 200*0.12
	if !result.ListCost.Equal(expectedList) {
		t.Errorf("ListCost = %s, want %s", result.ListCost, expectedList)
	}
	if !result.UpstreamCost.Equal(expectedUp) {
		t.Errorf("UpstreamCost = %s, want %s", result.UpstreamCost, expectedUp)
	}
}

func TestPricer_Calculate_FirstMatchWins(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{
				makePricingEntry("input", "0.05", "0.02"),
				makePricingEntry("input", "0.99", "0.50"),
			}, nil
		},
	}

	p := NewPricer(repo)
	result, err := p.Calculate(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 100})
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}

	expected := decimal.NewFromFloat(5.0) // 100 * 0.05
	if !result.ListCost.Equal(expected) {
		t.Errorf("ListCost = %s, want %s (first match should be used)", result.ListCost, expected)
	}
}

// TestPricer_Calculate_TenantID verifies tenantID is correctly forwarded to FindByModel.
func TestPricer_Calculate_TenantID(t *testing.T) {
	var receivedTenantID *uuid.UUID
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			receivedTenantID = tenantID
			return []domain.ModelPricing{makePricingEntry("input", "0.01", "0.005")}, nil
		},
	}

	p := NewPricer(repo)
	tenantID := uuid.New()
	_, err := p.Calculate(context.Background(), uuid.New(), &tenantID, &usageparser.NormalizedUsage{InputTokens: 100})
	if err != nil {
		t.Fatalf("Calculate error: %v", err)
	}

	if receivedTenantID == nil || *receivedTenantID != tenantID {
		t.Errorf("tenantID not forwarded correctly: received %v, want %v", receivedTenantID, tenantID)
	}
}

func TestPricer_Calculate_NilUsage(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{makePricingEntry("input", "0.01", "0.005")}, nil
		},
	}

	p := NewPricer(repo)
	_, err := p.Calculate(context.Background(), uuid.New(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil usage")
	}
	t.Logf("Nil usage correctly returns error: %v", err)
}
