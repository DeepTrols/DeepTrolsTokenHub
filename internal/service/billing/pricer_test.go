package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func newTestPricer(rows []domain.ModelPricing) *Pricer {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return rows, nil
		},
	}
	return NewPricer(repo)
}

func TestPricer_Calculate_SingleDimension_TokenScale(t *testing.T) {
	// Sell row (no price_type): unit price is per 1M tokens (元/百万).
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("input", "10", "5"),
	})

	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	// 1,000,000 tokens * 10 / 1,000,000 = 10
	if !result.ListCost.Equal(decimal.NewFromFloat(10)) {
		t.Errorf("ListCost = %s, want 10", result.ListCost)
	}
	if !result.UpstreamCost.Equal(decimal.NewFromFloat(5)) {
		t.Errorf("UpstreamCost = %s, want 5", result.UpstreamCost)
	}
	if len(result.ChargeLines) != 1 {
		t.Fatalf("ChargeLines count = %d, want 1", len(result.ChargeLines))
	}
	if result.ChargeLines[0].Quantity != 1_000_000 {
		t.Errorf("Quantity = %d, want 1000000", result.ChargeLines[0].Quantity)
	}
	if result.ChargeLines[0].LineCost.Equal(decimal.NewFromFloat(10)) == false {
		t.Errorf("LineCost = %s, want 10", result.ChargeLines[0].LineCost)
	}
}

func TestPricer_Calculate_TieredByContext(t *testing.T) {
	modelID := uuid.New()
	p := newTestPricer([]domain.ModelPricing{
		{
			ID: uuid.New(), ModelID: modelID, PricingDimension: "input", UnitName: "1M tokens",
			UnitPrice: "2", PriceType: domain.PriceTypeSell, IsActive: true,
			Conditions: map[string]any{"max_total_tokens": float64(1000)},
		},
		{
			ID: uuid.New(), ModelID: modelID, PricingDimension: "input", UnitName: "1M tokens",
			UnitPrice: "10", PriceType: domain.PriceTypeSell, IsActive: true,
			Conditions: map[string]any{"min_total_tokens": float64(1001)},
		},
	})
	ctx := context.Background()

	// 1K-token request fits the short tier: 2 * 1000 / 1M.
	small, err := p.CalculateAt(ctx, modelID, nil, &usageparser.NormalizedUsage{InputTokens: 1000, TotalTokens: 1000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate small: %v", err)
	}
	if !small.ListCost.Equal(decimal.NewFromFloat(0.002)) {
		t.Errorf("small ListCost = %s, want 0.002", small.ListCost)
	}

	// 50K-token request matches the long tier: 10 * 1000 / 1M.
	large, err := p.CalculateAt(ctx, modelID, nil, &usageparser.NormalizedUsage{InputTokens: 1000, TotalTokens: 50000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate large: %v", err)
	}
	if !large.ListCost.Equal(decimal.NewFromFloat(0.01)) {
		t.Errorf("large ListCost = %s, want 0.01", large.ListCost)
	}
}

func TestPricingConditionsMatch(t *testing.T) {
	base := &domain.ModelPricing{PricingDimension: "input", IsActive: true}
	usage := &usageparser.NormalizedUsage{TotalTokens: 5000}
	if !pricingConditionsMatch(base, usage) {
		t.Fatal("row without conditions must always match")
	}
	withMax := &domain.ModelPricing{Conditions: map[string]any{"max_total_tokens": float64(10000)}}
	if !pricingConditionsMatch(withMax, usage) {
		t.Fatal("usage within max must match")
	}
	if pricingConditionsMatch(&domain.ModelPricing{Conditions: map[string]any{"max_total_tokens": float64(1000)}}, usage) {
		t.Fatal("usage over max must not match")
	}
	if !pricingConditionsMatch(&domain.ModelPricing{Conditions: map[string]any{"min_total_tokens": float64(1000)}}, usage) {
		t.Fatal("usage over min must match")
	}
	if pricingConditionsMatch(&domain.ModelPricing{Conditions: map[string]any{"min_total_tokens": float64(6000)}}, usage) {
		t.Fatal("usage under min must not match")
	}
}

func TestPricer_Calculate_MultiDimension(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("input", "10", "5"),
		makePricingEntry("output", "30", "15"),
	})

	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1_000_000, OutputTokens: 500_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	// 1M*10/1M + 500K*30/1M = 10 + 15 = 25
	if !result.ListCost.Equal(decimal.NewFromFloat(25)) {
		t.Errorf("ListCost = %s, want 25", result.ListCost)
	}
	if !result.UpstreamCost.Equal(decimal.NewFromFloat(12.5)) {
		t.Errorf("UpstreamCost = %s, want 12.5", result.UpstreamCost)
	}
	if len(result.ChargeLines) != 2 {
		t.Fatalf("ChargeLines count = %d, want 2", len(result.ChargeLines))
	}
}

func TestPricer_Calculate_NonTokenDimension(t *testing.T) {
	// image is priced per unit (no /1000 scaling).
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("image", "0.50", "0.30"),
	})

	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{ImageCount: 2}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(1.0)) {
		t.Errorf("ListCost = %s, want 1.0 (2 images * 0.50)", result.ListCost)
	}
}

func TestPricer_Calculate_CostRow_NoMarkup(t *testing.T) {
	costRow := makePricingEntry("input", "1.5", "1.5")
	costRow.PriceType = domain.PriceTypeCost
	costRow.Period = domain.PricingPeriodOffPeak
	costRow.UnitName = "1M tokens"

	p := newTestPricer([]domain.ModelPricing{costRow})
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	// sell = cost = 1.5 per 1M tokens (no markup)
	if !result.ListCost.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("ListCost = %s, want 1.5", result.ListCost)
	}
	if !result.UpstreamCost.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("UpstreamCost = %s, want 1.5", result.UpstreamCost)
	}
	if result.ChargeLines[0].UnitPrice.Equal(decimal.NewFromFloat(1.5)) == false {
		t.Errorf("UnitPrice = %s, want 1.5", result.ChargeLines[0].UnitPrice)
	}
}

func TestPricer_Calculate_ExplicitSellWins(t *testing.T) {
	sellRow := makePricingEntry("input", "50", "20")
	sellRow.PriceType = domain.PriceTypeSell
	costRow := makePricingEntry("input", "1.5", "1.5")
	costRow.PriceType = domain.PriceTypeCost

	p := newTestPricer([]domain.ModelPricing{costRow, sellRow})
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(50)) {
		t.Errorf("ListCost = %s, want 50 (explicit sell wins)", result.ListCost)
	}
	if !result.UpstreamCost.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("UpstreamCost = %s, want 1.5 (cost row is authoritative)", result.UpstreamCost)
	}
}

// TestPricer_Calculate_UsesEditedSellPrice locks the invariant that billing
// follows the price shown in the model management UI: after an admin edits a
// sell price (new value, bumped version), the very next charge uses the new
// price and the evidence snapshot records it.
func TestPricer_Calculate_UsesEditedSellPrice(t *testing.T) {
	editedSell := makePricingEntry("input", "80", "1.5")
	editedSell.PriceType = domain.PriceTypeSell
	editedSell.PriceVersion = 7
	costRow := makePricingEntry("input", "1.5", "1.5")
	costRow.PriceType = domain.PriceTypeCost

	p := newTestPricer([]domain.ModelPricing{editedSell, costRow})
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(80)) {
		t.Errorf("ListCost = %s, want 80 (edited sell price, not old cost)", result.ListCost)
	}
	if len(result.ChargeLines) != 1 || !result.ChargeLines[0].UnitPrice.Equal(decimal.NewFromFloat(80)) {
		t.Errorf("ChargeLines UnitPrice = %+v, want 80", result.ChargeLines)
	}
	rows, ok := result.PriceSnapshot["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("snapshot rows = %T (%d), want []any with 1 row", rows, len(rows))
	}
	row := rows[0].(map[string]any)
	if row["unit_price"] != "80" || row["price_version"] != int64(7) {
		t.Errorf("snapshot unit_price/version = %v/%v, want 80/7", row["unit_price"], row["price_version"])
	}
}

func TestPricer_Calculate_PeriodSelection(t *testing.T) {
	peakRow := makePricingEntry("input", "3", "3")
	peakRow.PriceType = domain.PriceTypeCost
	peakRow.Period = domain.PricingPeriodPeak
	offPeakRow := makePricingEntry("input", "1.5", "1.5")
	offPeakRow.PriceType = domain.PriceTypeCost
	offPeakRow.Period = domain.PricingPeriodOffPeak

	p := newTestPricer([]domain.ModelPricing{offPeakRow, peakRow})
	usage := &usageparser.NormalizedUsage{InputTokens: 1_000_000}

	// 10:30 Asia/Shanghai = peak
	peakTime := time.Date(2026, 8, 21, 10, 30, 0, 0, shanghaiLocation)
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, usage, peakTime)
	if err != nil {
		t.Fatalf("peak Calculate: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(3)) {
		t.Errorf("peak ListCost = %s, want 3", result.ListCost)
	}
	if result.Period != domain.PricingPeriodPeak {
		t.Errorf("Period = %s, want peak", result.Period)
	}

	// 20:00 Asia/Shanghai = off_peak
	offPeakTime := time.Date(2026, 8, 21, 20, 0, 0, 0, shanghaiLocation)
	result, err = p.CalculateAt(context.Background(), uuid.New(), nil, usage, offPeakTime)
	if err != nil {
		t.Fatalf("off-peak Calculate: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("off-peak ListCost = %s, want 1.5", result.ListCost)
	}
	if result.Period != domain.PricingPeriodOffPeak {
		t.Errorf("Period = %s, want off_peak", result.Period)
	}
}

func TestPricer_Calculate_CacheReadDimension(t *testing.T) {
	row := makePricingEntry("cache_read", "0.1", "0.1")
	row.PriceType = domain.PriceTypeCost
	row.Period = domain.PricingPeriodPeak
	row.UnitName = "1M tokens"

	p := newTestPricer([]domain.ModelPricing{row})
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{CacheReadTokens: 5_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	// sell = cost = 0.1 per 1M; 5M tokens -> 0.5
	if !result.ListCost.Equal(decimal.NewFromFloat(0.5)) {
		t.Errorf("ListCost = %s, want 0.5", result.ListCost)
	}
	if len(result.MissingPricing) != 0 {
		t.Errorf("MissingPricing = %v, want empty", result.MissingPricing)
	}
}

func TestPricer_Calculate_MissingPricing(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("output", "0.03", "0.015"),
	})

	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if len(result.MissingPricing) != 1 || result.MissingPricing[0] != "input" {
		t.Errorf("MissingPricing = %v, want [input]", result.MissingPricing)
	}
	if !result.ListCost.IsZero() {
		t.Errorf("ListCost = %s, want 0 (no sell price for input)", result.ListCost)
	}
	if len(result.ChargeLines) != 0 {
		t.Errorf("ChargeLines count = %d, want 0", len(result.ChargeLines))
	}
}

func TestPricer_Calculate_EmptyPricing(t *testing.T) {
	p := newTestPricer(nil)
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if len(result.MissingPricing) != 1 || result.MissingPricing[0] != "input" {
		t.Errorf("MissingPricing = %v, want [input]", result.MissingPricing)
	}
	if !result.ListCost.IsZero() {
		t.Errorf("ListCost = %s, want 0", result.ListCost)
	}
}

func TestPricer_Calculate_FindByModelError(t *testing.T) {
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return nil, errors.New("database error")
		},
	}
	p := NewPricer(repo)
	_, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 100}, time.Now())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestPricer_Calculate_NilUsage(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{makePricingEntry("input", "0.01", "0.005")})
	_, err := p.CalculateAt(context.Background(), uuid.New(), nil, nil, time.Now())
	if err == nil {
		t.Fatal("expected error for nil usage")
	}
}

func TestPricer_Calculate_TenantIDForwarded(t *testing.T) {
	var receivedTenantID *uuid.UUID
	repo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			receivedTenantID = tenantID
			return []domain.ModelPricing{makePricingEntry("input", "0.01", "0.005")}, nil
		},
	}
	p := NewPricer(repo)
	tenantID := uuid.New()
	if _, err := p.CalculateAt(context.Background(), uuid.New(), &tenantID, &usageparser.NormalizedUsage{InputTokens: 100}, time.Now()); err != nil {
		t.Fatalf("Calculate error: %v", err)
	}
	if receivedTenantID == nil || *receivedTenantID != tenantID {
		t.Errorf("tenantID not forwarded: received %v, want %v", receivedTenantID, tenantID)
	}
}

func TestPricer_Calculate_TenantRowPreferred(t *testing.T) {
	platform := makePricingEntry("input", "10", "5")
	platform.PriceType = domain.PriceTypeSell
	tenant := makePricingEntry("input", "20", "5")
	tenant.PriceType = domain.PriceTypeSell
	tid := uuid.New()
	tenant.TenantID = &tid

	p := newTestPricer([]domain.ModelPricing{platform, tenant})
	result, err := p.CalculateAt(context.Background(), uuid.New(), &tid, &usageparser.NormalizedUsage{InputTokens: 1_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(20)) {
		t.Errorf("ListCost = %s, want 20 (tenant row preferred)", result.ListCost)
	}
}

func TestPricer_Calculate_InvalidSellPriceFallsBackToCost(t *testing.T) {
	badSell := makePricingEntry("input", "not-a-number", "0.005")
	badSell.PriceType = domain.PriceTypeSell
	costRow := makePricingEntry("input", "1.5", "1.5")
	costRow.PriceType = domain.PriceTypeCost

	p := newTestPricer([]domain.ModelPricing{badSell, costRow})
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1_000_000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(1.5)) {
		t.Errorf("ListCost = %s, want 1.5 (fallback to real cost)", result.ListCost)
	}
}

func TestPricer_PriceSnapshot_Populated(t *testing.T) {
	modelID := uuid.New()
	sellRow := makePricingEntry("input", "0.000015", "0.000010")
	sellRow.ModelID = modelID
	sellRow.PriceVersion = 3
	sellRow.PriceType = domain.PriceTypeSell

	p := newTestPricer([]domain.ModelPricing{sellRow})
	now := time.Date(2026, 8, 21, 20, 0, 0, 0, shanghaiLocation)
	result, err := p.CalculateAt(context.Background(), modelID, nil, &usageparser.NormalizedUsage{InputTokens: 1000}, now)
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if result.PriceSnapshot == nil {
		t.Fatal("PriceSnapshot should not be nil")
	}
	if result.PriceSnapshot["source"] != "model_pricing" {
		t.Errorf("snapshot source = %v, want model_pricing", result.PriceSnapshot["source"])
	}
	if result.PriceSnapshot["period"] != domain.PricingPeriodOffPeak {
		t.Errorf("snapshot period = %v, want off_peak", result.PriceSnapshot["period"])
	}
	rows, ok := result.PriceSnapshot["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("snapshot rows = %T (%d), want []any with 1 row", rows, len(rows))
	}
	row := rows[0].(map[string]any)
	if row["price_type"] != domain.PriceTypeSell {
		t.Errorf("row price_type = %v, want sell", row["price_type"])
	}
	if row["source"] != "explicit_sell" {
		t.Errorf("row source = %v, want explicit_sell", row["source"])
	}
	if row["period"] != domain.PricingPeriodOffPeak {
		t.Errorf("row period = %v, want off_peak", row["period"])
	}
	if row["price_version"] != int64(3) {
		t.Errorf("row price_version = %v, want 3", row["price_version"])
	}
}

func TestPricer_ChargeLines_CarryPriceSourceAndVersion(t *testing.T) {
	modelID := uuid.New()
	row := makePricingEntry("input", "0.01", "0.005")
	row.ModelID = modelID
	row.PriceVersion = 9

	p := newTestPricer([]domain.ModelPricing{row})
	result, err := p.CalculateAt(context.Background(), modelID, nil, &usageparser.NormalizedUsage{InputTokens: 100}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if len(result.ChargeLines) != 1 {
		t.Fatalf("ChargeLines count = %d, want 1", len(result.ChargeLines))
	}
	if result.ChargeLines[0].PriceSource != "model_pricing" {
		t.Errorf("PriceSource = %q, want model_pricing", result.ChargeLines[0].PriceSource)
	}
	if result.ChargeLines[0].PriceVersion != 9 {
		t.Errorf("PriceVersion = %d, want 9", result.ChargeLines[0].PriceVersion)
	}
}

func TestPricer_PriceSnapshot_EmptyRowsWhenNoPricing(t *testing.T) {
	p := newTestPricer(nil)
	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{InputTokens: 1000}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	if result.PriceSnapshot == nil {
		t.Fatal("PriceSnapshot should not be nil")
	}
	if result.PriceSnapshot["source"] != "model_pricing" {
		t.Errorf("snapshot source = %v, want model_pricing", result.PriceSnapshot["source"])
	}
	rows, ok := result.PriceSnapshot["rows"].([]any)
	if !ok || len(rows) != 0 {
		t.Errorf("snapshot rows = %T (%d), want empty []any", rows, len(rows))
	}
}

func TestPricer_PricingPeriodBoundaries(t *testing.T) {
	cases := []struct {
		hour int
		want string
	}{
		{8, domain.PricingPeriodOffPeak},
		{9, domain.PricingPeriodPeak},
		{11, domain.PricingPeriodPeak},
		{12, domain.PricingPeriodOffPeak}, // lunch break
		{13, domain.PricingPeriodOffPeak},
		{14, domain.PricingPeriodPeak},
		{17, domain.PricingPeriodPeak},
		{18, domain.PricingPeriodOffPeak},
		{23, domain.PricingPeriodOffPeak},
	}
	for _, c := range cases {
		now := time.Date(2026, 8, 21, c.hour, 0, 0, 0, shanghaiLocation)
		if got := pricingPeriod(now); got != c.want {
			t.Errorf("pricingPeriod(%d:00) = %s, want %s", c.hour, got, c.want)
		}
	}
}

// ============================================================================
// RED: reasoning tokens are already included in completion_tokens for the
// domestic providers (DeepSeek etc.). Without an explicit reasoning price row
// the dimension must NOT be reported as missing pricing (which would fail the
// whole call closed) and must NOT be billed a second time.
// ============================================================================

func TestPricer_ReasoningWithoutPrice_NotMissing(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("input", "10", "5"),
		makePricingEntry("output", "30", "15"),
	})

	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{
		InputTokens:     1_000_000,
		OutputTokens:    500_000,
		ReasoningTokens: 200_000, // already included in OutputTokens
	}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	for _, missing := range result.MissingPricing {
		if missing == "reasoning" {
			t.Fatal("reasoning must not be missing pricing when no reasoning row exists (it is included in output)")
		}
	}
	// Cost = input 10 + output 15, NOT an extra reasoning line.
	if !result.ListCost.Equal(decimal.NewFromFloat(25)) {
		t.Errorf("ListCost = %s, want 25 (no separate reasoning charge)", result.ListCost)
	}
}

func TestPricer_ReasoningWithExplicitPrice_Charged(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("input", "10", "5"),
		makePricingEntry("output", "30", "15"),
		makePricingEntry("reasoning", "20", "10"),
	})

	result, err := p.CalculateAt(context.Background(), uuid.New(), nil, &usageparser.NormalizedUsage{
		InputTokens:     1_000_000,
		OutputTokens:    500_000,
		ReasoningTokens: 200_000,
	}, time.Now())
	if err != nil {
		t.Fatalf("Calculate unexpected error: %v", err)
	}
	// 10 + 15 + 200K*20/1M = 25 + 4 = 29
	if !result.ListCost.Equal(decimal.NewFromFloat(29)) {
		t.Errorf("ListCost = %s, want 29", result.ListCost)
	}
}

func TestPricer_CalculateWithRatio_ScalesSellPrice(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{
		makePricingEntry("input", "10", "5"),
	})

	// VIP group ratio 0.8: sell 10 -> 8, upstream cost stays 5.
	result, err := p.CalculateWithRatio(context.Background(), uuid.New(), nil,
		&usageparser.NormalizedUsage{InputTokens: 1_000_000}, decimal.NewFromFloat(0.8))
	if err != nil {
		t.Fatalf("CalculateWithRatio: %v", err)
	}
	if !result.ListCost.Equal(decimal.NewFromFloat(8)) {
		t.Errorf("ListCost = %s, want 8", result.ListCost)
	}
	if !result.UpstreamCost.Equal(decimal.NewFromFloat(5)) {
		t.Errorf("UpstreamCost = %s, want 5 (cost must not be scaled)", result.UpstreamCost)
	}
	if len(result.ChargeLines) != 1 || !result.ChargeLines[0].UnitPrice.Equal(decimal.NewFromFloat(8)) {
		t.Errorf("ChargeLines = %+v, want unit price 8", result.ChargeLines)
	}
	if got := result.PriceSnapshot["price_ratio"]; got != "0.8" {
		t.Errorf("snapshot price_ratio = %v, want 0.8", got)
	}
	rows, ok := result.PriceSnapshot["rows"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("snapshot rows = %v", result.PriceSnapshot["rows"])
	}
	if row, ok := rows[0].(map[string]any); !ok || row["price_ratio"] != "0.8" {
		t.Errorf("row price_ratio = %v, want 0.8", rows[0])
	}
}

func TestPricer_CalculateWithRatio_RejectsNonPositive(t *testing.T) {
	p := newTestPricer([]domain.ModelPricing{makePricingEntry("input", "10", "5")})
	for _, ratio := range []decimal.Decimal{decimal.Zero, decimal.NewFromFloat(-1)} {
		if _, err := p.CalculateWithRatio(context.Background(), uuid.New(), nil,
			&usageparser.NormalizedUsage{InputTokens: 100}, ratio); err == nil {
			t.Fatalf("ratio %s: expected error, got nil", ratio)
		}
	}
}
