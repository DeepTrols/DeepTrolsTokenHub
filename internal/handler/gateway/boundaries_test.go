package gateway

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// mockAPIKeyRepoForBoundaries implements apikey.Repository for boundary tests.
type mockAPIKeyRepoForBoundaries struct {
	key               *domain.APIKey
	spend             map[string]*domain.APIKeySpend
	updateSpendCalled int
}

func (m *mockAPIKeyRepoForBoundaries) FindByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	return m.key, nil
}
func (m *mockAPIKeyRepoForBoundaries) FindByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	return m.key, nil
}
func (m *mockAPIKeyRepoForBoundaries) ListByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) ([]domain.APIKey, error) {
	return nil, nil
}
func (m *mockAPIKeyRepoForBoundaries) Create(ctx context.Context, key *domain.APIKey) error {
	return nil
}
func (m *mockAPIKeyRepoForBoundaries) Update(ctx context.Context, key *domain.APIKey) error {
	return nil
}
func (m *mockAPIKeyRepoForBoundaries) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockAPIKeyRepoForBoundaries) GetSpend(ctx context.Context, keyID uuid.UUID, periodType string) (*domain.APIKeySpend, error) {
	if m.spend == nil {
		return &domain.APIKeySpend{APIKeyID: keyID, PeriodType: periodType, TotalCost: decimal.Zero}, nil
	}
	if s, ok := m.spend[periodType]; ok {
		return s, nil
	}
	return &domain.APIKeySpend{APIKeyID: keyID, PeriodType: periodType, TotalCost: decimal.Zero}, nil
}
func (m *mockAPIKeyRepoForBoundaries) UpdateSpend(ctx context.Context, spend *domain.APIKeySpend) error {
	m.updateSpendCalled++
	return nil
}

func TestEnforceAPIKeyBoundaries_ModelAllowlist(t *testing.T) {
	key := &domain.APIKey{ID: uuid.New(), AllowedModels: []string{"gpt-4o"}, OverLimitAction: domain.OverLimitBlock}
	appl := &app.App{APIKeys: &mockAPIKeyRepoForBoundaries{key: key}}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxAPIKeyID, key.ID.String()))

	if err := enforceAPIKeyBoundaries(req, appl, "gpt-4o"); err != nil {
		t.Fatalf("allowed model should pass: %v", err)
	}
	err := enforceAPIKeyBoundaries(req, appl, "gpt-4o-mini")
	if err == nil {
		t.Fatal("disallowed model should be rejected")
	}
	be, ok := err.(*boundaryError)
	if !ok || be.status != 403 || be.errType != "model_not_allowed" {
		t.Fatalf("expected model_not_allowed 403, got %v", err)
	}
}

func TestEnforceAPIKeyBoundaries_IPWhitelist(t *testing.T) {
	key := &domain.APIKey{ID: uuid.New(), SourceWhitelist: []string{"203.0.113.10"}, OverLimitAction: domain.OverLimitBlock}
	appl := &app.App{APIKeys: &mockAPIKeyRepoForBoundaries{key: key}}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxAPIKeyID, key.ID.String()))

	if err := enforceAPIKeyBoundaries(req, appl, "gpt-4o"); err != nil {
		t.Fatalf("whitelisted IP should pass: %v", err)
	}

	req2 := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req2.RemoteAddr = "198.51.100.7:12345"
	req2 = req2.WithContext(context.WithValue(req2.Context(), middleware.CtxAPIKeyID, key.ID.String()))
	err := enforceAPIKeyBoundaries(req2, appl, "gpt-4o")
	if err == nil {
		t.Fatal("non-whitelisted IP should be rejected")
	}
	be, ok := err.(*boundaryError)
	if !ok || be.status != 403 || be.errType != "ip_not_allowed" {
		t.Fatalf("expected ip_not_allowed 403, got %v", err)
	}
}

func TestEnforceAPIKeyBoundaries_SpendLimit(t *testing.T) {
	spent := decimal.NewFromInt(120)
	key := &domain.APIKey{ID: uuid.New(), CumulativeLimit: decimal.NewFromInt(100), OverLimitAction: domain.OverLimitBlock}
	repo := &mockAPIKeyRepoForBoundaries{
		key: key,
		spend: map[string]*domain.APIKeySpend{
			"cumulative": {APIKeyID: key.ID, PeriodType: "cumulative", TotalCost: spent},
		},
	}
	appl := &app.App{APIKeys: repo}
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxAPIKeyID, key.ID.String()))

	err := enforceAPIKeyBoundaries(req, appl, "gpt-4o")
	if err == nil {
		t.Fatal("over-limit key with block action should be rejected")
	}
	be, ok := err.(*boundaryError)
	if !ok || be.errType != "limit_exceeded" {
		t.Fatalf("expected limit_exceeded, got %v", err)
	}

	// warn action: request allowed
	warnKey := &domain.APIKey{ID: uuid.New(), CumulativeLimit: decimal.NewFromInt(100), OverLimitAction: domain.OverLimitWarn}
	repo.key = warnKey
	if err := enforceAPIKeyBoundaries(req, appl, "gpt-4o"); err != nil {
		t.Fatalf("warn action should allow request: %v", err)
	}
}

func TestRecordAPIKeySpend_UpdatesThreePeriods(t *testing.T) {
	repo := &mockAPIKeyRepoForBoundaries{key: &domain.APIKey{ID: uuid.New()}}
	appl := &app.App{APIKeys: repo}
	recordAPIKeySpend(context.Background(), appl, repo.key.ID, decimal.NewFromInt(5))
	if repo.updateSpendCalled != 3 {
		t.Fatalf("expected 3 spend updates (cumulative/weekly/monthly), got %d", repo.updateSpendCalled)
	}

	// zero cost and nil repo are no-ops
	recordAPIKeySpend(context.Background(), appl, repo.key.ID, decimal.Zero)
	if repo.updateSpendCalled != 3 {
		t.Fatalf("zero cost should not record spend, got %d", repo.updateSpendCalled)
	}
	recordAPIKeySpend(context.Background(), &app.App{APIKeys: nil}, uuid.New(), decimal.NewFromInt(5)) // must not panic
}
