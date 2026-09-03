package gateway

// TH-P05-02 (B5 Settle Fallback Visibility Correction).
//
// These tests pin the settle fallback contract:
//   - AC-01: final cost > reserve + no available balance → commit the
//     reserved hold, mark undercharged=true, ledger stays consistent.
//   - AC-02: a replayed request (settle hits a transaction that is no
//     longer in reserve state) must NOT trigger an additional wallet
//     debit and must NOT raise the undercharge flag.
//   - AC-04: a normal settle (final ≤ reserve) completes with no flag.
//   - Failure injection: a generic settle DB failure commits the hold and
//     leaves an explicit evidence code — never a silent success.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/wallet"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ---------------------------------------------------------------------------
// Unit: settle fallback classifier.
// ---------------------------------------------------------------------------

func TestClassifySettleFailure(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		wantCommit   bool
		wantUnder    bool
		wantEvidence string
	}{
		{
			name:         "nil error is not a fallback",
			err:          nil,
			wantCommit:   false,
			wantUnder:    false,
			wantEvidence: "",
		},
		{
			name:         "insufficient available balance",
			err:          fmt.Errorf("charger settle: %w", wallet.ErrInsufficientBalance),
			wantCommit:   true,
			wantUnder:    true,
			wantEvidence: "undercharged",
		},
		{
			name:         "bare insufficient sentinel",
			err:          wallet.ErrInsufficientBalance,
			wantCommit:   true,
			wantUnder:    true,
			wantEvidence: "undercharged",
		},
		{
			name:         "transaction no longer reserved (idempotent replay)",
			err:          fmt.Errorf("charger settle: %w: type=charge", wallet.ErrTxNotReserved),
			wantCommit:   false,
			wantUnder:    false,
			wantEvidence: "",
		},
		{
			name:         "generic infrastructure error",
			err:          errors.New("connection reset by peer"),
			wantCommit:   true,
			wantUnder:    false,
			wantEvidence: "settle_error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifySettleFailure(tc.err)
			if got.commitReserved != tc.wantCommit {
				t.Errorf("commitReserved = %v, want %v", got.commitReserved, tc.wantCommit)
			}
			if got.undercharged != tc.wantUnder {
				t.Errorf("undercharged = %v, want %v", got.undercharged, tc.wantUnder)
			}
			if got.evidenceCode != tc.wantEvidence {
				t.Errorf("evidenceCode = %q, want %q", got.evidenceCode, tc.wantEvidence)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unit: undercharge fallback counter (observability requirement).
// ---------------------------------------------------------------------------

func TestUnderchargeFallbackCounter(t *testing.T) {
	before := UnderchargeFallbackCounts()["unit-test-endpoint|unit-model"]
	countUnderchargeFallback("unit-test-endpoint", "unit-model")
	countUnderchargeFallback("unit-test-endpoint", "unit-model")
	countUnderchargeFallback("unit-test-endpoint", "other-model")

	snap := UnderchargeFallbackCounts()
	if got := snap["unit-test-endpoint|unit-model"]; got != before+2 {
		t.Errorf("counter[unit-test-endpoint|unit-model] = %d, want %d", got, before+2)
	}
	if got := snap["unit-test-endpoint|other-model"]; got < 1 {
		t.Errorf("counter[unit-test-endpoint|other-model] = %d, want >= 1", got)
	}
}

// ---------------------------------------------------------------------------
// Handler-level fixtures shared by the settle fallback scenarios.
// ---------------------------------------------------------------------------

// settleFallbackFixture wires a non-streaming chat handler where the settle
// behavior is fully controlled by settleErr. Usage is the small standard
// response (final cost ≪ any hold), so undercharge vs no-undercharge is
// decided purely by the injected error class.
func settleFallbackFixture(t *testing.T, settleErr error) (*mockWalletRepo, *mockUsageRepo, *httptest.ResponseRecorder) {
	t.Helper()
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode:    http.StatusOK,
				Body:          respBody,
				Usage:         usage,
				UsageSource:   usageparser.SourceUpstream,
				ProviderReqID: "chatcmpl-settle-fallback",
				DurationMs:    120,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		settleFn: func(ctx context.Context, tID uuid.UUID, amount decimal.Decimal) error {
			return settleErr
		},
		commitFn:  func(ctx context.Context, tID uuid.UUID) error { return nil },
		releaseFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}
	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	return walletRepo, usageRepo, w
}

// ---------------------------------------------------------------------------
// AC-02: replayed request id → settle rejects a finalized transaction and
// the gateway must not debit the wallet a second time.
// ---------------------------------------------------------------------------

func TestHandleNonStreamingChat_SettleReplay_NoAdditionalDebit(t *testing.T) {
	walletRepo, usageRepo, _ := settleFallbackFixture(t,
		fmt.Errorf("charger settle: %w: type=charge", wallet.ErrTxNotReserved))

	if walletRepo.settleCalled == 0 {
		t.Fatal("settle should have been attempted")
	}
	if walletRepo.commitCalled != 0 {
		t.Errorf("commit called %d times on a replayed (already finalized) settle; want 0 — replay must not move money", walletRepo.commitCalled)
	}
	if walletRepo.releaseCalled != 0 {
		t.Errorf("release called %d times on replay; want 0", walletRepo.releaseCalled)
	}

	log := waitForUsageLog(t, usageRepo)
	if log.ErrorCode != "" {
		t.Errorf("ErrorCode = %q, want empty (replay is not an undercharge)", log.ErrorCode)
	}
	if !log.WalletCharged.IsZero() {
		t.Errorf("WalletCharged = %s, want 0 (no additional debit on replay)", log.WalletCharged)
	}
}

// ---------------------------------------------------------------------------
// Failure injection: settle fails with an unknown/infrastructure error.
// The reserved hold must still be committed (value was consumed upstream)
// and the evidence chain must carry an explicit settle_error marker.
// ---------------------------------------------------------------------------

func TestHandleNonStreamingChat_SettleDBFailure_CommitsHoldWithEvidence(t *testing.T) {
	walletRepo, usageRepo, _ := settleFallbackFixture(t,
		errors.New("pq: unexpected EOF on connection"))

	if walletRepo.commitCalled != 1 {
		t.Errorf("commit called %d times, want 1 (hold must be collected after a settle DB failure)", walletRepo.commitCalled)
	}
	if walletRepo.releaseCalled != 0 {
		t.Errorf("release called %d times after upstream success; want 0", walletRepo.releaseCalled)
	}

	log := waitForUsageLog(t, usageRepo)
	if log.ErrorCode != "settle_error" {
		t.Errorf("ErrorCode = %q, want %q", log.ErrorCode, "settle_error")
	}
	// final cost ≪ hold in this fixture: the committed hold covers the cost,
	// so the charged amount must equal the reserved hold, not the final cost.
	if !log.WalletCharged.Equal(walletRepo.lastReserveAmt) {
		t.Errorf("WalletCharged = %s, want reserved hold %s", log.WalletCharged, walletRepo.lastReserveAmt)
	}
	if log.WalletCharged.LessThan(log.FinalCost) {
		t.Errorf("WalletCharged %s must not fall below FinalCost %s when no shortfall exists", log.WalletCharged, log.FinalCost)
	}
}

// ---------------------------------------------------------------------------
// AC-04: normal settle succeeds → no fallback, no evidence flag.
// ---------------------------------------------------------------------------

func TestHandleNonStreamingChat_NormalSettle_NoEvidenceFlag(t *testing.T) {
	walletRepo, usageRepo, _ := settleFallbackFixture(t, nil)

	if walletRepo.settleCalled != 1 {
		t.Errorf("settle called %d times, want 1", walletRepo.settleCalled)
	}
	if walletRepo.commitCalled != 0 {
		t.Errorf("commit fallback called %d times on a successful settle; want 0", walletRepo.commitCalled)
	}

	log := waitForUsageLog(t, usageRepo)
	if log.ErrorCode != "" {
		t.Errorf("ErrorCode = %q, want empty on a clean settle", log.ErrorCode)
	}
	if !log.WalletCharged.Equal(log.FinalCost) {
		t.Errorf("WalletCharged = %s, want FinalCost %s", log.WalletCharged, log.FinalCost)
	}
}

// ---------------------------------------------------------------------------
// AC-01 (sentinel precision): ErrInsufficientBalance from settle must mark
// undercharged evidence even when wrapped by the charger.
// ---------------------------------------------------------------------------

func TestHandleNonStreamingChat_InsufficientBalance_MarksUndercharged(t *testing.T) {
	walletRepo, usageRepo, _ := settleFallbackFixture(t,
		fmt.Errorf("charger settle: %w: available=0.10 extra_required=2.00", wallet.ErrInsufficientBalance))

	if walletRepo.commitCalled != 1 {
		t.Errorf("commit called %d times, want 1 (undercharge fallback commits the hold)", walletRepo.commitCalled)
	}
	if walletRepo.releaseCalled != 0 {
		t.Errorf("release called %d times after upstream success; want 0", walletRepo.releaseCalled)
	}

	log := waitForUsageLog(t, usageRepo)
	if log.ErrorCode != "undercharged" {
		t.Errorf("ErrorCode = %q, want %q", log.ErrorCode, "undercharged")
	}
	if !strings.Contains(log.ErrorMessage, "shortfall") {
		t.Errorf("ErrorMessage = %q, want it to describe the shortfall", log.ErrorMessage)
	}
	if log.WalletCharged.IsZero() {
		t.Error("WalletCharged must equal the committed reserved hold, got 0")
	}
}
