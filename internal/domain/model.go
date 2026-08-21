package domain

import (
	"time"

	"github.com/google/uuid"
)

type Model struct {
	ID              uuid.UUID
	Code            string
	Provider        string
	Category        ModelCategory
	DisplayName     string
	Description     string
	ContextWindow   int
	MaxOutputTokens int
	Capabilities    map[string]any
	Status          ModelStatus
	ReleaseStage    ReleaseStage
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ModelCategory string

const (
	ModelCategoryChat      ModelCategory = "chat"
	ModelCategoryEmbedding ModelCategory = "embedding"
	ModelCategoryImage     ModelCategory = "image"
	ModelCategoryAudio     ModelCategory = "audio"
	ModelCategoryVideo     ModelCategory = "video"
)

type ModelStatus string

const (
	ModelStatusActive     ModelStatus = "active"
	ModelStatusBeta       ModelStatus = "beta"
	ModelStatusDeprecated ModelStatus = "deprecated"
	ModelStatusInactive   ModelStatus = "inactive"
)

type ReleaseStage string

const (
	ReleaseStageGA          ReleaseStage = "GA"
	ReleaseStageBeta        ReleaseStage = "beta"
	ReleaseStageUnsupported ReleaseStage = "unsupported"
)

func (m Model) IsCallable() bool {
	return m.Status == ModelStatusActive || m.Status == ModelStatusBeta
}

// Price type / period dimensions for pricing rows.
const (
	PriceTypeCost = "cost"
	PriceTypeSell = "sell"

	PricingPeriodPeak    = "peak"
	PricingPeriodOffPeak = "off_peak"
)

// ModelPricing defines a price for a specific dimension.
type ModelPricing struct {
	ID               uuid.UUID
	ModelID          uuid.UUID
	TenantID         *uuid.UUID
	RequestType      string
	PricingDimension string
	UnitName         string
	UnitPrice        string
	Currency         string
	UpstreamCost     string
	PriceVersion     int64
	PriceType        string
	Period           string
	Conditions       map[string]any
	IsActive         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TenantModel is the OEM model selection configuration.
type TenantModel struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	ModelID      uuid.UUID
	IsListed     bool
	AllowPayg    bool
	QuotaEnabled bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
