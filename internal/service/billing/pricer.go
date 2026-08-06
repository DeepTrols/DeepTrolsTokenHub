package billing

import (
	"context"

	"fmt"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"log"
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
				break
			}
		}

		if unitName == "" {
			continue
		}

		lineCost := unitPrice.Mul(decimal.NewFromInt(qty))
		upstreamLineCost := upstreamPrice.Mul(decimal.NewFromInt(qty))

		result.ListCost = result.ListCost.Add(lineCost)
		result.UpstreamCost = result.UpstreamCost.Add(upstreamLineCost)

		result.ChargeLines = append(result.ChargeLines, ChargeLineInput{
			Dimension: dim,
			UnitName:  unitName,
			Quantity:  qty,
			UnitPrice: unitPrice,
			LineCost:  lineCost,
		})
	}

	return result, nil
}
