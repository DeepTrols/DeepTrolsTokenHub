package billing

import (
	"context"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/usage"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Compile-time interface conformance checks.
var _ wallet.Repository = (*mockWalletRepo)(nil)
var _ usage.Repository = (*mockUsageRepo)(nil)
var _ model.PricingRepository = (*mockPricingRepo)(nil)

// ---------------------------------------------------------------------------
// mockWalletRepo — mocks wallet.Repository for Charger tests
// ---------------------------------------------------------------------------

type mockWalletRepo struct {
	reserveFn func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error)
	commitFn  func(ctx context.Context, txID uuid.UUID) error
	settleFn  func(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error
	releaseFn func(ctx context.Context, txID uuid.UUID) error
}

func (m *mockWalletRepo) FindByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
	return nil, nil
}
func (m *mockWalletRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	return nil, nil
}
func (m *mockWalletRepo) Create(ctx context.Context, wallet *domain.Wallet) error {
	return nil
}
func (m *mockWalletRepo) Reserve(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	if m.reserveFn != nil {
		return m.reserveFn(ctx, walletID, amount, idempotencyKey)
	}
	return nil, nil
}
func (m *mockWalletRepo) Commit(ctx context.Context, txID uuid.UUID) error {
	if m.commitFn != nil {
		return m.commitFn(ctx, txID)
	}
	return nil
}
func (m *mockWalletRepo) Settle(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error {
	if m.settleFn != nil {
		return m.settleFn(ctx, txID, finalAmount)
	}
	return nil
}
func (m *mockWalletRepo) Release(ctx context.Context, txID uuid.UUID) error {
	if m.releaseFn != nil {
		return m.releaseFn(ctx, txID)
	}
	return nil
}
func (m *mockWalletRepo) ListTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]domain.WalletTransaction, error) {
	return nil, nil
}

func (m *mockWalletRepo) TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	return nil, nil
}
func (m *mockWalletRepo) Transfer(ctx context.Context, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// mockUsageRepo — mocks usage.Repository for Logger tests
// ---------------------------------------------------------------------------

type mockUsageRepo struct {
	createUsageLogFn         func(ctx context.Context, log *domain.UsageLog) error
	createChargeLinesFn      func(ctx context.Context, lines []domain.ChargeLine) error
	createProviderEvidenceFn func(ctx context.Context, evidence *domain.ProviderEvidence) error
}

func (m *mockUsageRepo) CreateUsageLog(ctx context.Context, log *domain.UsageLog) error {
	if m.createUsageLogFn != nil {
		return m.createUsageLogFn(ctx, log)
	}
	return nil
}
func (m *mockUsageRepo) CreateChargeLines(ctx context.Context, lines []domain.ChargeLine) error {
	if m.createChargeLinesFn != nil {
		return m.createChargeLinesFn(ctx, lines)
	}
	return nil
}
func (m *mockUsageRepo) CreateProviderEvidence(ctx context.Context, evidence *domain.ProviderEvidence) error {
	if m.createProviderEvidenceFn != nil {
		return m.createProviderEvidenceFn(ctx, evidence)
	}
	return nil
}
func (m *mockUsageRepo) FindByRequestID(ctx context.Context, requestID string) (*domain.UsageLog, error) {
	return nil, nil
}
func (m *mockUsageRepo) ListByUser(ctx context.Context, userID uuid.UUID, filter usage.UsageFilter) ([]domain.UsageLog, int, error) {
	return nil, 0, nil
}
func (m *mockUsageRepo) ListByAPIKey(ctx context.Context, apiKeyID uuid.UUID, filter usage.UsageFilter) ([]domain.UsageLog, int, error) {
	return nil, 0, nil
}

// ---------------------------------------------------------------------------
// mockPricingRepo — mocks model.PricingRepository for Pricer tests
// ---------------------------------------------------------------------------

type mockPricingRepo struct {
	findByModelFn func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error)
}

func (m *mockPricingRepo) FindByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
	if m.findByModelFn != nil {
		return m.findByModelFn(ctx, modelID, tenantID)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// makePricingEntry returns a domain.ModelPricing with the given dimension and prices.
func makePricingEntry(dimension, unitPrice, upstreamCost string) domain.ModelPricing {
	return domain.ModelPricing{
		ID:               uuid.New(),
		ModelID:          uuid.New(),
		RequestType:      "chat",
		PricingDimension: dimension,
		UnitName:         "token",
		UnitPrice:        unitPrice,
		Currency:         "CNY",
		UpstreamCost:     upstreamCost,
		IsActive:         true,
	}
}
