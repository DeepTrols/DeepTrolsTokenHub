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

type PriceResult struct {
	ListCost      decimal.Decimal
	UpstreamCost  decimal.Decimal
	ChargeLines   []ChargeLineInput
	PriceSnapshot map[string]any
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

type Pricer struct {
	pricing model.PricingRepository
}

func NewPricer(pricing model.PricingRepository) *Pricer {
	return &Pricer{pricing: pricing}
}

func (p *Pricer) Calculate(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID, usage *usageparser.NormalizedUsage) (*PriceResult, error) {
	if usage == nil {
		return nil, fmt.Errorf("pricer calculate: usage must not be nil")
	}

	pricings, err := p.pricing.FindByModel(ctx, modelID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("pricer calculate: %w", err)
	}

	result := &PriceResult{
		ListCost:      decimal.Zero,
		UpstreamCost:  decimal.Zero,
		ChargeLines:   make([]ChargeLineInput, 0),
		PriceSnapshot: make(map[string]any),
	}

	snapshotRows := make([]any, 0)
	snapshotCurrency := "CNY"
	capturedAt := time.Now().UTC().Format(time.RFC3339)

	dimensions := map[string]int64{
		"input":       usage.InputTokens,
		"output":      usage.OutputTokens,
		"cache_read":  usage.CacheReadTokens,
		"cache_write": usage.CacheWriteTokens,
		"reasoning":   usage.ReasoningTokens,
		"image":       usage.ImageCount,
		"audio":       usage.AudioSeconds,
		"tts":         usage.TTSCharacters,
		"video":       usage.VideoUnits,
	}

	for dim, qty := range dimensions {
		if qty <= 0 {
			continue
		}

		// Find matching pricing entry
		var unitPrice, upstreamPrice decimal.Decimal
		var unitName string
		var matched *domain.ModelPricing
		for _, pr := range pricings {
			if pr.PricingDimension == dim {
				var err error
				unitPrice, err = decimal.NewFromString(pr.UnitPrice)
				if err != nil {
					log.Printf("billing: invalid unit price %q for dimension %s: %v", pr.UnitPrice, dim, err)
					continue
				}
				upstreamPrice, err = decimal.NewFromString(pr.UpstreamCost)
				if err != nil {
					log.Printf("billing: invalid upstream cost %q for dimension %s: %v", pr.UpstreamCost, dim, err)
					continue
				}
				unitName = pr.UnitName
				matched = &pr
				break
			}
		}

		if unitName == "" {
			continue
		}

		if matched.Currency != "" {
			snapshotCurrency = matched.Currency
		}

		lineCost := unitPrice.Mul(decimal.NewFromInt(qty))
		upstreamLineCost := upstreamPrice.Mul(decimal.NewFromInt(qty))

		result.ListCost = result.ListCost.Add(lineCost)
		result.UpstreamCost = result.UpstreamCost.Add(upstreamLineCost)

		result.ChargeLines = append(result.ChargeLines, ChargeLineInput{
			Dimension:    dim,
			UnitName:     unitName,
			Quantity:     qty,
			UnitPrice:    unitPrice,
			LineCost:     lineCost,
			PriceSource:  "model_pricing",
			PriceVersion: int(matched.PriceVersion),
		})

		var tenantID any
		if matched.TenantID != nil {
			tenantID = matched.TenantID.String()
		}
		snapshotRows = append(snapshotRows, map[string]any{
			"pricing_id":    matched.ID.String(),
			"dimension":     dim,
			"unit_name":     unitName,
			"unit_price":    matched.UnitPrice,
			"upstream_cost": matched.UpstreamCost,
			"price_version": matched.PriceVersion,
			"tenant_id":     tenantID,
		})
	}

	result.PriceSnapshot = map[string]any{
		"source":      "model_pricing",
		"currency":    snapshotCurrency,
		"captured_at": capturedAt,
		"rows":        snapshotRows,
	}

	return result, nil
}
