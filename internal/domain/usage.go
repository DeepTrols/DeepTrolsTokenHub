package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type UsageLog struct {
	ID                uuid.UUID
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
	UsageSource       UsageSource
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
	Status            UsageLogStatus
	ErrorCode         string
	ErrorMessage      string
	RequestSummary    string
	ResponseSummary   string
	CreatedAt         time.Time
}

type UsageSource string

const (
	UsageSourceUpstream   UsageSource = "upstream"
	UsageSourceFinalChunk UsageSource = "final_chunk"
	UsageSourceEstimated  UsageSource = "estimated"
	UsageSourceCached     UsageSource = "cached"
)

type UsageLogStatus string

const (
	UsageLogStatusCompleted UsageLogStatus = "completed"
	UsageLogStatusFailed    UsageLogStatus = "failed"
	UsageLogStatusPartial   UsageLogStatus = "partial"
	UsageLogStatusRefunded  UsageLogStatus = "refunded"
)

type ChargeLine struct {
	ID              uuid.UUID
	UsageLogID      uuid.UUID
	Dimension       string
	UnitName        string
	Quantity        int64
	UnitPrice       decimal.Decimal
	LineCost        decimal.Decimal
	DiscountApplied decimal.Decimal
	PriceSource     string
	PriceVersion    int
	CreatedAt       time.Time
}

type ProviderEvidence struct {
	ID                uuid.UUID
	UsageLogID        *uuid.UUID
	Provider          string
	ProviderRequestID string
	RequestBody       map[string]any
	ResponseBody      map[string]any
	StatusCode        int
	DurationMs        int
	UsageRaw          map[string]any
	ProviderCost      decimal.Decimal
	ProviderCurrency  string
	ErrorMessage      string
	CreatedAt         time.Time
}
