package billing

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PriceResult is the outcome of pricing a request: sell cost charged to the
// user, upstream cost paid to the provider, per-dimension charge lines and a
// full evidence snapshot.
type PriceResult struct {
	ListCost       decimal.Decimal
	UpstreamCost   decimal.Decimal
	ChargeLines    []ChargeLineInput
	PriceSnapshot  map[string]any
	MissingPricing []string // dimensions with usage but no resolvable sell price
	Period         string
}

type ChargeLineInput struct {
	Dimension       string
	UnitName        string
	Quantity        int64
	UnitPrice       decimal.Decimal
	LineCost        decimal.Decimal
	DiscountApplied decimal.Decimal
	PriceSource     string
	PriceVersion    int
}

// tokenDimensions are priced per 1K tokens (unit_name "1K tokens"); all other
// dimensions (image/audio/tts/video) are priced per single unit.
var tokenDimensions = map[string]bool{
	"input":       true,
	"output":      true,
	"cache_read":  true,
	"cache_write": true,
	"reasoning":   true,
}

// shanghaiLocation is the billing timezone. DeepSeek peak windows are defined
// in Asia/Shanghai local time: 09:00-12:00 and 14:00-18:00.
var shanghaiLocation = time.FixedZone("Asia/Shanghai", 8*3600)

type Pricer struct {
	pricing model.PricingRepository
}

func NewPricer(pricing model.PricingRepository) *Pricer {
	return &Pricer{pricing: pricing}
}

// Calculate prices usage for the current time (see CalculateAt).
func (p *Pricer) Calculate(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID, usage *usageparser.NormalizedUsage) (*PriceResult, error) {
	return p.CalculateAt(ctx, modelID, tenantID, usage, time.Now().UTC())
}

// CalculateAt prices usage at a specific instant so tests are deterministic.
// Sell price resolution (first match wins):
//  1. explicit sell row (price_type='sell') for the dimension,
//     preferring the current period and tenant-specific rows;
//  2. cost row (price_type='cost') at its real price (no markup);
//  3. otherwise the dimension is reported in MissingPricing (fail-closed).
func (p *Pricer) CalculateAt(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID, usage *usageparser.NormalizedUsage, now time.Time) (*PriceResult, error) {
	if usage == nil {
		return nil, fmt.Errorf("pricer calculate: usage must not be nil")
	}

	pricings, err := p.pricing.FindByModel(ctx, modelID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("pricer calculate: %w", err)
	}

	period := pricingPeriod(now)

	result := &PriceResult{
		ListCost:      decimal.Zero,
		UpstreamCost:  decimal.Zero,
		ChargeLines:   make([]ChargeLineInput, 0),
		PriceSnapshot: make(map[string]any),
		Period:        period,
	}

	snapshotRows := make([]any, 0)
	snapshotCurrency := "CNY"
	capturedAt := now.Format(time.RFC3339)

	dimensions := []struct {
		name string
		qty  int64
	}{
		{"input", usage.InputTokens},
		{"output", usage.OutputTokens},
		{"cache_read", usage.CacheReadTokens},
		{"cache_write", usage.CacheWriteTokens},
		{"reasoning", usage.ReasoningTokens},
		{"image", usage.ImageCount},
		{"audio", usage.AudioSeconds},
		{"tts", usage.TTSCharacters},
		{"video", usage.VideoUnits},
	}

	for _, dim := range dimensions {
		if dim.qty <= 0 {
			continue
		}

		sellRow, costRow := selectPricingRows(pricings, dim.name, period)

		sellPrice := decimal.Zero
		unitName := ""
		priceVersion := int64(0)
		priceID := ""
		source := ""
		rowType := ""
		var rowTenant *uuid.UUID

		if sellRow != nil {
			v, ok := parsePricingDecimal(sellRow.UnitPrice, dim.name, "unit price")
			if ok {
				sellPrice = v
				unitName = sellRow.UnitName
				priceVersion = sellRow.PriceVersion
				priceID = sellRow.ID.String()
				source = "explicit_sell"
				rowType = normalizePriceType(sellRow.PriceType)
				rowTenant = sellRow.TenantID
			}
		}
		if sellRow == nil || unitName == "" {
			// Fall back to the real cost price (no markup).
			if costRow != nil {
				if v, ok := parsePricingDecimal(costRow.UnitPrice, dim.name, "cost"); ok {
					sellPrice = v
					unitName = costRow.UnitName
					priceVersion = costRow.PriceVersion
					priceID = costRow.ID.String()
					source = "cost_derived"
					rowType = normalizePriceType(costRow.PriceType)
					rowTenant = costRow.TenantID
				}
			}
		}

		if unitName == "" {
			// No sell price resolvable: never charge zero silently.
			result.MissingPricing = append(result.MissingPricing, dim.name)
			continue
		}

		costPrice := decimal.Zero
		if costRow != nil {
			costPrice, _ = parsePricingDecimal(costRow.UnitPrice, dim.name, "cost")
		} else if sellRow != nil && sellRow.UpstreamCost != "" {
			costPrice, _ = parsePricingDecimal(sellRow.UpstreamCost, dim.name, "upstream cost")
		}

		scale := decimal.NewFromInt(1)
		if tokenDimensions[dim.name] {
			scale = decimal.NewFromInt(1000)
		}
		qty := decimal.NewFromInt(dim.qty)
		lineCost := sellPrice.Mul(qty).Div(scale)
		upstreamLineCost := costPrice.Mul(qty).Div(scale)

		result.ListCost = result.ListCost.Add(lineCost)
		result.UpstreamCost = result.UpstreamCost.Add(upstreamLineCost)
		result.ChargeLines = append(result.ChargeLines, ChargeLineInput{
			Dimension:    dim.name,
			UnitName:     unitName,
			Quantity:     dim.qty,
			UnitPrice:    sellPrice,
			LineCost:     lineCost,
			PriceSource:  "model_pricing",
			PriceVersion: int(priceVersion),
		})

		if sellRow != nil && sellRow.Currency != "" {
			snapshotCurrency = sellRow.Currency
		} else if costRow != nil && costRow.Currency != "" {
			snapshotCurrency = costRow.Currency
		}

		var tenant any
		if rowTenant != nil {
			tenant = rowTenant.String()
		}
		snapshotRows = append(snapshotRows, map[string]any{
			"pricing_id":    priceID,
			"dimension":     dim.name,
			"unit_name":     unitName,
			"unit_price":    sellPrice.String(),
			"upstream_cost": costPrice.String(),
			"price_version": priceVersion,
			"price_type":    rowType,
			"period":        period,
			"source":        source,
			"tenant_id":     tenant,
		})
	}

	result.PriceSnapshot = map[string]any{
		"source":      "model_pricing",
		"currency":    snapshotCurrency,
		"captured_at": capturedAt,
		"period":      period,
		"rows":        snapshotRows,
	}

	return result, nil
}

// pricingPeriod returns the DeepSeek pricing window for a time in Asia/Shanghai:
// peak 09:00-12:00 and 14:00-18:00, otherwise off_peak.
func pricingPeriod(now time.Time) string {
	t := now.In(shanghaiLocation)
	switch h := t.Hour(); {
	case h >= 9 && h < 12, h >= 14 && h < 18:
		return domain.PricingPeriodPeak
	default:
		return domain.PricingPeriodOffPeak
	}
}

// selectPricingRows returns the best sell and cost rows for a dimension.
// Preference: current period first, then tenant-specific rows, then first row.
func selectPricingRows(rows []domain.ModelPricing, dim, period string) (*domain.ModelPricing, *domain.ModelPricing) {
	var sell, cost *domain.ModelPricing
	for i := range rows {
		r := &rows[i]
		if r.PricingDimension != dim || !r.IsActive {
			continue
		}
		switch normalizePriceType(r.PriceType) {
		case domain.PriceTypeSell:
			if sell == nil || pricingRowScore(r, period) < pricingRowScore(sell, period) {
				sell = r
			}
		case domain.PriceTypeCost:
			if cost == nil || pricingRowScore(r, period) < pricingRowScore(cost, period) {
				cost = r
			}
		}
	}
	return sell, cost
}

// pricingRowScore ranks candidate rows: period mismatch adds 10, platform
// (tenant_id IS NULL) rows add 1 so tenant-specific rows win ties.
func pricingRowScore(r *domain.ModelPricing, period string) int {
	score := 0
	if normalizePeriod(r.Period) != period {
		score += 10
	}
	if r.TenantID == nil {
		score++
	}
	return score
}

func normalizePriceType(t string) string {
	if t == "" {
		return domain.PriceTypeSell
	}
	return t
}

func normalizePeriod(p string) string {
	if p == "" {
		return domain.PricingPeriodOffPeak
	}
	return p
}

func parsePricingDecimal(s, dim, kind string) (decimal.Decimal, bool) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		log.Printf("billing: invalid %s %q for dimension %s: %v", kind, s, dim, err)
		return decimal.Zero, false
	}
	return d, true
}
