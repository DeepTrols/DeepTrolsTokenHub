package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type APIKey struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	TenantID        *uuid.UUID
	KeyPrefix       string
	KeyHash         string
	EncryptedKey    string
	MaskedKey       string
	Name            string
	Status          APIKeyStatus
	AllowedModels   []string
	SourceWhitelist []string
	CumulativeLimit decimal.Decimal
	WeeklyLimit     decimal.Decimal
	MonthlyLimit    decimal.Decimal
	OverLimitAction OverLimitAction
	LastUsedAt      *time.Time
	Last7dActive    bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
	RevokedAt       *time.Time
}

type APIKeyStatus string

const (
	APIKeyStatusActive    APIKeyStatus = "active"
	APIKeyStatusDisabled  APIKeyStatus = "disabled"
	APIKeyStatusRevoked   APIKeyStatus = "revoked"
	APIKeyStatusOverLimit APIKeyStatus = "over_limit"
)

type OverLimitAction string

const (
	OverLimitBlock OverLimitAction = "block"
	OverLimitWarn  OverLimitAction = "warn"
)

func (k APIKey) IsUsable() bool {
	return k.Status == APIKeyStatusActive || k.Status == APIKeyStatusOverLimit
}

func NewAPIKey(userID uuid.UUID, tenantID *uuid.UUID, keyPrefix, keyHash, maskedKey, name string) APIKey {
	now := time.Now().UTC()
	return APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		TenantID:        tenantID,
		KeyPrefix:       keyPrefix,
		KeyHash:         keyHash,
		MaskedKey:       maskedKey,
		Name:            name,
		Status:          APIKeyStatusActive,
		OverLimitAction: OverLimitBlock,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// APIKeySpend tracks spend against API Key limits.
type APIKeySpend struct {
	ID          uuid.UUID
	APIKeyID    uuid.UUID
	PeriodType  string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	TotalCost   decimal.Decimal
	UpdatedAt   time.Time
}
