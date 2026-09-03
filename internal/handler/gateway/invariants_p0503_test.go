package gateway

// TH-P05-03 (Billing Invariant And Concurrency Tests) — gateway level.
//
// The full non-streaming chat handler is driven against a REAL Postgres
// wallet repository (through the real Charger) so the money invariants are
// proven on the production path, not on mocks:
//
//   - Case A / AC-01: two concurrent requests, balance funds exactly one
//     hold -> at most one provider call, no over-consume;
//   - W6: zero balance -> no reserve, hence zero provider calls;
//   - AC-03: upstream failure after a successful reserve -> hold released,
//     failure evidence recorded;
//   - W5: a replayed request id never debits twice;
//   - W10: settle above the reserve commits exactly the hold and preserves
//     undercharge evidence (TH-P05-02 pinned semantics);
//   - Case F: parallel successful requests end ledger-consistent.
//
// Pricing is 1.0 (input) / 2.0 (output) per 1M tokens, so every cost for
// integer token counts is exact at DECIMAL(18,6). The standard fixture body
// (usage 20/15) settles at exactly 0.000050.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/deeptrols/api/internal/service/billing"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// p0503StandardFinalCost is the exact settle cost of validResponseBody under
// the fixture pricing: 20*1.0/1M + 15*2.0/1M = 0.000050.
func p0503StandardFinalCost() decimal.Decimal {
	return decimal.RequireFromString("0.000050")
}

// p0503HugeUsageResponseBody reports 20 prompt + 200000 completion tokens:
// a final cost of 0.400020, far above any fixture hold.
func p0503HugeUsageResponseBody() map[string]any {
	body := validResponseBody()
	body["usage"] = map[string]any{
		"prompt_tokens":     float64(20),
		"completion_tokens": float64(200000),
		"total_tokens":      float64(200020),
	}
	return body
}

// p0503ObservedWallets wraps the REAL Postgres wallet repository without
// altering any money semantics: it only counts Reserve attempts and can gate
// reserves after the first, letting tests force deterministic interleavings.
// Every mutation still executes against the real database.
type p0503ObservedWallets struct {
	inner wallet.Repository
	mu    sync.Mutex
	// reserveAttempts counts every Reserve call.
	reserveAttempts int
	// gateSecondReserve, when non-nil, blocks every Reserve call after the
	// first until the channel is closed.
	gateSecondReserve chan struct{}
}

func (o *p0503ObservedWallets) Reserve(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	o.mu.Lock()
	o.reserveAttempts++
	attempt := o.reserveAttempts
	gate := o.gateSecondReserve
	o.mu.Unlock()
	if attempt > 1 && gate != nil {
		<-gate
	}
	return o.inner.Reserve(ctx, walletID, amount, idempotencyKey)
}

func (o *p0503ObservedWallets) attempts() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.reserveAttempts
}

func (o *p0503ObservedWallets) FindByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
	return o.inner.FindByUser(ctx, userID, tenantID)
}
func (o *p0503ObservedWallets) FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	return o.inner.FindByID(ctx, id)
}
func (o *p0503ObservedWallets) Create(ctx context.Context, w *domain.Wallet) error {
	return o.inner.Create(ctx, w)
}
func (o *p0503ObservedWallets) Commit(ctx context.Context, txID uuid.UUID) error {
	return o.inner.Commit(ctx, txID)
}
func (o *p0503ObservedWallets) Settle(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error {
	return o.inner.Settle(ctx, txID, finalAmount)
}
func (o *p0503ObservedWallets) Release(ctx context.Context, txID uuid.UUID) error {
	return o.inner.Release(ctx, txID)
}
func (o *p0503ObservedWallets) TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	return o.inner.TopUp(ctx, walletID, amount, idempotencyKey)
}
func (o *p0503ObservedWallets) Spend(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	return o.inner.Spend(ctx, walletID, amount, idempotencyKey)
}
func (o *p0503ObservedWallets) Transfer(ctx context.Context, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	return o.inner.Transfer(ctx, fromWalletID, toWalletID, amount, idempotencyKey)
}
func (o *p0503ObservedWallets) ListTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]domain.WalletTransaction, error) {
	return o.inner.ListTransactions(ctx, walletID, limit, offset)
}

var _ wallet.Repository = (*p0503ObservedWallets)(nil)

// p0503Fixture bundles a real-DB gateway app with its seeded wallet.
type p0503Fixture struct {
	application *app.App
	pool        *pgxpool.Pool
	repo        *wallet.PostgresRepository
	observed    *p0503ObservedWallets
	usageRepo   *mockUsageRepo
	executor    *mockExecutor
	userID      uuid.UUID
	walletID    uuid.UUID
	hold        decimal.Decimal // the hold the gateway reserves for the standard body
	fund        decimal.Decimal // the amount funded via ledgered TopUp
}

// p0503PricingEntries prices input at 1.0 and output at 2.0 per 1M tokens.
// Integer token counts then yield costs exact at DECIMAL(18,6).
func p0503PricingEntries() []domain.ModelPricing {
	return []domain.ModelPricing{
		{
			ID:               uuid.New(),
			ModelID:          uuid.Nil,
			PricingDimension: "input",
			UnitName:         "1M tokens",
			UnitPrice:        "1.0",
			UpstreamCost:     "0.5",
			Currency:         "CNY",
			IsActive:         true,
		},
		{
			ID:               uuid.New(),
			ModelID:          uuid.Nil,
			PricingDimension: "output",
			UnitName:         "1M tokens",
			UnitPrice:        "2.0",
			UpstreamCost:     "1.0",
			Currency:         "CNY",
			IsActive:         true,
		},
	}
}

// newP0503Fixture builds the real-DB gateway app. fundAmount maps the
// discovered hold H to the wallet funding (e.g. H for the undercharge case,
// 2H for success cases, 0 for the zero-balance case).
func newP0503Fixture(t *testing.T, fundAmount func(hold decimal.Decimal) decimal.Decimal) *p0503Fixture {
	t.Helper()
	pool := testutil.SetupPool(t)
	ctx := context.Background()
	testutil.TruncateTables(t, pool,
		"wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	repo := wallet.NewPostgresRepository(pool)
	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, userID.String()+"@p0503.test", "hash"); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	modelID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive,
				HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID,
				BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o",
				Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return p0503PricingEntries(), nil
		},
	}
	usageRepo := &mockUsageRepo{}
	executor := &mockExecutor{}

	observed := &p0503ObservedWallets{inner: repo}
	application := &app.App{
		Config: &config.Config{
			LiteLLM: config.LiteLLMConfig{BaseURL: "http://localhost:9999", MasterKey: "test-master-key"},
		},
		Executor:   executor,
		Router:     gw.NewRouter(modelRepo, channelRepo),
		Pricer:     billing.NewPricer(pricingRepo),
		Charger:    billing.NewCharger(observed),
		Logger:     billing.NewLogger(usageRepo),
		Wallets:    observed,
		HttpClient: &http.Client{},
	}

	// Discover the hold the gateway will reserve for the standard body.
	probe := newNonStreamChatRequest(userID, uuid.New(), validRequestBody())
	hold, _, ok := computeMaxChargeHold(httptest.NewRecorder(), application, probe, modelID, nil, validRequestBody())
	if !ok {
		t.Fatal("hold computation failed for the standard fixture body")
	}
	if !hold.IsPositive() {
		t.Fatalf("hold = %s, want positive", hold)
	}

	w := domain.NewWallet(userID, nil, "CNY")
	if err := repo.Create(ctx, &w); err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	fund := fundAmount(hold)
	if fund.IsPositive() {
		if _, err := repo.TopUp(ctx, w.ID, fund, "p0503-fund-"+w.ID.String()); err != nil {
			t.Fatalf("fund wallet: %v", err)
		}
	}

	return &p0503Fixture{
		application: application,
		pool:        pool,
		repo:        repo,
		observed:    observed,
		usageRepo:   usageRepo,
		executor:    executor,
		userID:      userID,
		walletID:    w.ID,
		hold:        hold,
		fund:        fund,
	}
}

// walletState reads balance/frozen straight from the database.
func (f *p0503Fixture) walletState(t *testing.T) (balance, frozen decimal.Decimal) {
	t.Helper()
	var balanceStr, frozenStr string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT balance::text, frozen::text FROM wallets WHERE id = $1`, f.walletID).
		Scan(&balanceStr, &frozenStr); err != nil {
		t.Fatalf("read wallet: %v", err)
	}
	return parseP0503Decimal(balanceStr), parseP0503Decimal(frozenStr)
}

// ledgerNet recomputes the balance from wallet_transactions (same formula as
// the wallet repository suite / TH-P05-02).
func (f *p0503Fixture) ledgerNet(t *testing.T) decimal.Decimal {
	t.Helper()
	var netStr string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(CASE WHEN tx_type IN ('reserve','release') THEN 0
		                          WHEN tx_type = 'charge' THEN -amount
		                          ELSE amount END), 0)::text
		 FROM wallet_transactions WHERE wallet_id = $1`, f.walletID).Scan(&netStr); err != nil {
		t.Fatalf("ledger net: %v", err)
	}
	return parseP0503Decimal(netStr)
}

// countTxRows counts the wallet's ledger rows, optionally filtered by type.
func (f *p0503Fixture) countTxRows(t *testing.T, txType string) int {
	t.Helper()
	query := `SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id = $1`
	args := []any{f.walletID}
	if txType != "" {
		query += ` AND tx_type = $2`
		args = append(args, txType)
	}
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count tx rows: %v", err)
	}
	return n
}

// assertInvariants pins W1-W3 and W11 on the current wallet state.
func (f *p0503Fixture) assertInvariants(t *testing.T) {
	t.Helper()
	balance, frozen := f.walletState(t)
	if balance.IsNegative() {
		t.Errorf("W1 violated: negative balance %s", balance)
	}
	if frozen.IsNegative() {
		t.Errorf("W2 violated: negative frozen %s", frozen)
	}
	if balance.Sub(frozen).IsNegative() {
		t.Errorf("W3 violated: negative available balance (balance=%s frozen=%s)", balance, frozen)
	}
	if net := f.ledgerNet(t); !net.Equal(balance) {
		t.Errorf("W11 violated: ledger net %s != balance %s", net, balance)
	}
	var openStr string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT COALESCE(SUM(amount), 0)::text FROM wallet_transactions
		 WHERE wallet_id = $1 AND tx_type = 'reserve'`, f.walletID).Scan(&openStr); err != nil {
		t.Fatalf("open reserve sum: %v", err)
	}
	if open := parseP0503Decimal(openStr); !open.Equal(frozen) {
		t.Errorf("W11 violated: open reserve sum %s != frozen %s", open, frozen)
	}
}

func parseP0503Decimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// ---------------------------------------------------------------------------
// Case A / AC-01: two concurrent requests against a wallet funded for
// exactly one hold (H <= B < 2H). Determinism is achieved by role, not by
// goroutine identity:
//
//   - Winner = the request whose Reserve executes first (attempt #1,
//     ungated). It proceeds to the provider checkpoint, signals READY,
//     and is held there until the test explicitly RELEASEs it.
//   - Loser = the request whose Reserve executes second (attempt #2,
//     gated at the observed-wallet wrapper). The test releases the gate
//     only after the winner's hold is frozen in the DB; the loser's
//     reserve must then be safely rejected (402 or 500) with no provider
//     call.
//
// Because the winner cannot finish before RELEASE, the first handler
// goroutine to return is necessarily the loser — the test waits on that
// role event and never on a fixed goroutine. Timeouts in this test are
// deadlock safety guards only, never ordering mechanisms. Exactly one
// request may reach the provider; the wallet is charged exactly once.
// ---------------------------------------------------------------------------

func TestP0503_Gateway_CaseA_ConcurrentRequests_OneProviderCall(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		// B = H + round(H/2): one hold fits, two never can.
		return hold.Add(hold.DivRound(decimal.NewFromInt(2), 6))
	})

	secondGate := make(chan struct{})
	f.observed.gateSecondReserve = secondGate
	winnerAtProvider := make(chan struct{}) // READY: winner held at the provider checkpoint, hold frozen.
	proceed := make(chan struct{})          // RELEASE: winner may complete the provider call and settle.
	firstFinished := make(chan struct{})    // closed by the first handler to return — by construction the loser.
	var enteredProvider, handlerReturned sync.Once
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		enteredProvider.Do(func() { close(winnerAtProvider) })
		select {
		case <-proceed:
		case <-time.After(5 * time.Second):
			// Deadlock safety guard only — never an ordering mechanism.
			t.Error("executor gate timed out — test orchestration deadlock")
		}
		respBody := validResponseBody()
		usage, _ := usageparser.ParseOpenAIUsage(respBody)
		return &gw.ExecuteResponse{
			StatusCode: http.StatusOK, Body: respBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream, ProviderReqID: "p0503-caseA", DurationMs: 5,
		}, nil
	}

	apiKeyID := uuid.New()
	req1 := newNonStreamChatRequest(f.userID, apiKeyID, validRequestBody())
	req2 := newNonStreamChatRequest(f.userID, apiKeyID, validRequestBody())
	w1, w2 := httptest.NewRecorder(), httptest.NewRecorder()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		HandleNonStreamingChat(w1, req1, f.application, "gpt-4o", validRequestBody())
		handlerReturned.Do(func() { close(firstFinished) })
	}()
	go func() {
		defer wg.Done()
		HandleNonStreamingChat(w2, req2, f.application, "gpt-4o", validRequestBody())
		handlerReturned.Do(func() { close(firstFinished) })
	}()

	// Deterministic, role-based orchestration (no goroutine identity, no
	// wall-clock ordering):
	// 1. READY — the winner has reserved and is held at the provider
	//    checkpoint; its hold is frozen in the DB.
	p0503WaitOrchestration(t, winnerAtProvider, "winner never reached the provider checkpoint")
	// 2. Release the loser's gated reserve against the frozen hold; it must
	//    be rejected by the wallet/DB.
	close(secondGate)
	// 3. The first handler to finish is necessarily the loser (the winner is
	//    still held at the provider checkpoint). Waiting on the role event —
	//    not on a fixed goroutine — is what makes both race outcomes legal.
	p0503WaitOrchestration(t, firstFinished, "losing request never completed against the frozen hold")
	// 4. RELEASE — the winner completes the provider call and settles; both
	//    handlers are now done.
	close(proceed)
	wg.Wait()

	var okCount, rejectedCount int
	for _, code := range []int{w1.Code, w2.Code} {
		switch {
		case code == http.StatusOK:
			okCount++
		case code == http.StatusPaymentRequired || code == http.StatusInternalServerError:
			// 402: the pre-reserve availability check saw the frozen hold;
			// 500: the DB-level reserve rejected it. Both are legal "no
			// provider call" outcomes; neither may be a success.
			rejectedCount++
		default:
			t.Errorf("unexpected status %d (body: %s / %s)", code, w1.Body.String(), w2.Body.String())
		}
	}
	if okCount != 1 || rejectedCount != 1 {
		t.Fatalf("AC-01 violated: statuses = %d and %d, want exactly one 200 and one 402/500",
			w1.Code, w2.Code)
	}
	if calls := f.executor.executeCalled; calls != 1 {
		t.Errorf("AC-01 violated: provider called %d times, want exactly 1", calls)
	}

	finalCost := p0503StandardFinalCost()
	balance, frozen := f.walletState(t)
	if want := f.fund.Sub(finalCost); !balance.Equal(want) {
		t.Errorf("balance = %s, want %s (charged exactly once)", balance, want)
	}
	if !frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", frozen)
	}
	if rows := f.countTxRows(t, ""); rows != 2 { // topup + reserve finalized in place to charge
		t.Errorf("ledger rows = %d, want 2 (topup + one reserve/charge)", rows)
	}
	if charges := f.countTxRows(t, "charge"); charges != 1 {
		t.Errorf("charge rows = %d, want exactly 1", charges)
	}
	f.assertInvariants(t)
}

// p0503OrchestrationGuard bounds every orchestration wait in the Case A
// deterministic construction. It is a deadlock safety guard only: it fails
// the test instead of hanging the package, and is never used to order
// business events.
const p0503OrchestrationGuard = 30 * time.Second

func p0503WaitOrchestration(t *testing.T, event <-chan struct{}, deadlockMsg string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(p0503OrchestrationGuard):
		t.Fatalf("%s (test orchestration deadlock)", deadlockMsg)
	}
}

// ---------------------------------------------------------------------------
// W6: a zero-balance wallet must be rejected BEFORE any reserve and BEFORE
// any provider call (provider only after a successful reserve).
// ---------------------------------------------------------------------------

func TestP0503_Gateway_W6_ZeroBalance_NoProviderCall(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal { return decimal.Zero })
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		respBody := validResponseBody()
		usage, _ := usageparser.ParseOpenAIUsage(respBody)
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream}, nil
	}

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 for a zero-balance wallet; body=%s", w.Code, w.Body.String())
	}
	if calls := f.executor.executeCalled; calls != 0 {
		t.Errorf("W6 violated: provider called %d times although no reserve could succeed", calls)
	}
	balance, frozen := f.walletState(t)
	if !balance.IsZero() || !frozen.IsZero() {
		t.Errorf("wallet mutated without a reserve: %s/%s, want 0/0", balance, frozen)
	}
	if rows := f.countTxRows(t, ""); rows != 0 {
		t.Errorf("ledger rows = %d, want 0 (no mutation ever happened)", rows)
	}
}

// ---------------------------------------------------------------------------
// AC-03: an upstream failure after a successful reserve must release the
// hold and leave failure evidence — never a silent charge, never a lost
// request.
// ---------------------------------------------------------------------------

func TestP0503_Gateway_AC03_UpstreamFailureAfterReserve_ReleasesHold(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		return hold.Mul(decimal.NewFromInt(2))
	})
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		return nil, errors.New("p0503: upstream exploded")
	}

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 after upstream failure; body=%s", w.Code, w.Body.String())
	}

	// releaseWalletHold is synchronous on a live request context: by the
	// time the handler returns, the hold must already be released.
	balance, frozen := f.walletState(t)
	if !balance.Equal(f.fund) {
		t.Errorf("AC-03 violated: balance = %s, want %s (hold released, nothing charged)", balance, f.fund)
	}
	if !frozen.IsZero() {
		t.Errorf("AC-03 violated: frozen = %s, want 0 after release", frozen)
	}
	if releases := f.countTxRows(t, "release"); releases != 1 {
		t.Errorf("release rows = %d, want exactly 1 (the hold)", releases)
	}
	if charges := f.countTxRows(t, "charge"); charges != 0 {
		t.Errorf("AC-03 violated: %d charge rows after an upstream failure, want 0", charges)
	}
	f.assertInvariants(t)

	// Failure evidence: a failed usage_log with zero charge.
	usageLog := waitForUsageLog(t, f.usageRepo)
	if usageLog.Status != domain.UsageLogStatusFailed {
		t.Errorf("usage log status = %q, want %q", usageLog.Status, domain.UsageLogStatusFailed)
	}
	if !usageLog.WalletCharged.IsZero() {
		t.Errorf("WalletCharged = %s, want 0 on upstream failure", usageLog.WalletCharged)
	}
	if usageLog.ErrorCode == "" {
		t.Error("ErrorCode is empty; failure evidence must carry an explicit code")
	}
}

// ---------------------------------------------------------------------------
// W5: replaying the same X-Request-ID must never debit twice. The second
// pass reuses the idempotent reserve (already finalized) and the settle
// classifier treats it as a replay: no commit, no undercharge flag.
// ---------------------------------------------------------------------------

func TestP0503_Gateway_W5_ReplayedRequestID_NoSecondDebit(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		return hold.Mul(decimal.NewFromInt(2))
	})
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		respBody := validResponseBody()
		usage, _ := usageparser.ParseOpenAIUsage(respBody)
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream, ProviderReqID: "p0503-w5", DurationMs: 5}, nil
	}

	requestID := "p0503-replay-" + uuid.New().String()
	apiKeyID := uuid.New()

	// First pass: charges the standard final cost.
	req1 := newNonStreamChatRequest(f.userID, apiKeyID, validRequestBody())
	req1.Header.Set("X-Request-ID", requestID)
	w1 := httptest.NewRecorder()
	HandleNonStreamingChat(w1, req1, f.application, "gpt-4o", validRequestBody())
	if w1.Code != http.StatusOK {
		t.Fatalf("first pass status = %d, want 200; body=%s", w1.Code, w1.Body.String())
	}
	firstLog := waitForUsageLog(t, f.usageRepo)
	if !firstLog.WalletCharged.Equal(p0503StandardFinalCost()) {
		t.Fatalf("first pass WalletCharged = %s, want %s", firstLog.WalletCharged, p0503StandardFinalCost())
	}
	balanceAfterFirst, _ := f.walletState(t)
	wantBalance := f.fund.Sub(p0503StandardFinalCost())
	if !balanceAfterFirst.Equal(wantBalance) {
		t.Fatalf("balance after first pass = %s, want %s", balanceAfterFirst, wantBalance)
	}

	// Reset the captured log so the second pass's evidence is observable.
	f.usageRepo.mu.Lock()
	f.usageRepo.lastUsageLog = nil
	f.usageRepo.mu.Unlock()

	// Second pass: same request id.
	req2 := newNonStreamChatRequest(f.userID, apiKeyID, validRequestBody())
	req2.Header.Set("X-Request-ID", requestID)
	w2 := httptest.NewRecorder()
	HandleNonStreamingChat(w2, req2, f.application, "gpt-4o", validRequestBody())
	if w2.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200; body=%s", w2.Code, w2.Body.String())
	}

	secondLog := waitForUsageLog(t, f.usageRepo)
	if !secondLog.WalletCharged.IsZero() {
		t.Errorf("W5 violated: replay charged %s, want 0 (no double debit)", secondLog.WalletCharged)
	}
	if secondLog.ErrorCode != "" {
		t.Errorf("W5 violated: replay evidence code = %q, want empty (replay is not an anomaly)",
			secondLog.ErrorCode)
	}

	balance, frozen := f.walletState(t)
	if !balance.Equal(wantBalance) {
		t.Errorf("W5 violated: balance after replay = %s, want %s (unchanged)", balance, wantBalance)
	}
	if !frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", frozen)
	}
	if rows := f.countTxRows(t, ""); rows != 2 {
		t.Errorf("W5 violated: ledger rows = %d after replay, want 2 (replay appends nothing)", rows)
	}
	if charges := f.countTxRows(t, "charge"); charges != 1 {
		t.Errorf("charge rows = %d, want exactly 1", charges)
	}
	f.assertInvariants(t)
}

// ---------------------------------------------------------------------------
// W10: when the final cost exceeds reserve + available balance, the gateway
// must commit exactly the reserved hold (never more) and preserve the
// undercharge evidence in both the usage log and the fallback counter
// (TH-P05-02 pinned semantics).
// ---------------------------------------------------------------------------

func TestP0503_Gateway_W10_Undercharge_EvidencePreserved(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		return hold // balance == hold: any cost above the hold is unaffordable
	})
	hugeBody := p0503HugeUsageResponseBody()
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		usage, _ := usageparser.ParseOpenAIUsage(hugeBody)
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: hugeBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream, ProviderReqID: "p0503-w10", DurationMs: 5}, nil
	}

	before := UnderchargeFallbackCounts()["chat|gpt-4o"]

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (client received the completion); body=%s", w.Code, w.Body.String())
	}

	usageLog := waitForUsageLog(t, f.usageRepo)
	if usageLog.ErrorCode != "undercharged" {
		t.Errorf("W10 violated: ErrorCode = %q, want %q", usageLog.ErrorCode, "undercharged")
	}
	if !strings.Contains(usageLog.ErrorMessage, "shortfall") {
		t.Errorf("ErrorMessage = %q, want it to describe the shortfall", usageLog.ErrorMessage)
	}
	// Charged exactly the reserved hold — never the unaffordable final cost.
	if !usageLog.WalletCharged.Equal(f.hold) {
		t.Errorf("W10 violated: WalletCharged = %s, want the reserved hold %s", usageLog.WalletCharged, f.hold)
	}
	wantFinal := decimal.RequireFromString("0.400020")
	if !usageLog.FinalCost.Equal(wantFinal) {
		t.Errorf("FinalCost = %s, want %s", usageLog.FinalCost, wantFinal)
	}

	balance, frozen := f.walletState(t)
	if !balance.IsZero() {
		t.Errorf("balance = %s, want 0 (the whole hold collected, nothing more)", balance)
	}
	if !frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", frozen)
	}
	if charges := f.countTxRows(t, "charge"); charges != 1 {
		t.Errorf("charge rows = %d, want exactly 1", charges)
	}
	f.assertInvariants(t)

	if after := UnderchargeFallbackCounts()["chat|gpt-4o"]; after != before+1 {
		t.Errorf("W10 observability: undercharge counter[chat|gpt-4o] = %d, want %d (exactly one event)",
			after, before+1)
	}
}

// ---------------------------------------------------------------------------
// Case F: parallel independent successful requests. Whatever the
// interleaving, the wallet must end ledger-consistent with every request
// charged exactly its final cost.
// ---------------------------------------------------------------------------

func TestP0503_Gateway_CaseF_ParallelSuccessfulRequests_ConsistentLedger(t *testing.T) {
	const requests = 4
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		return hold.Mul(decimal.NewFromInt(requests))
	})
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		respBody := validResponseBody()
		usage, _ := usageparser.ParseOpenAIUsage(respBody)
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream, ProviderReqID: "p0503-caseF", DurationMs: 5}, nil
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make([]int, requests)
	apiKeyID := uuid.New()
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req := newNonStreamChatRequest(f.userID, apiKeyID, validRequestBody())
			w := httptest.NewRecorder()
			HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())
			codes[i] = w.Code
		}(i)
	}
	close(start)
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200", i, code)
		}
	}
	if calls := f.executor.executeCalled; calls != requests {
		t.Errorf("provider calls = %d, want %d", calls, requests)
	}

	finalCost := p0503StandardFinalCost()
	wantBalance := f.fund.Sub(finalCost.Mul(decimal.NewFromInt(requests)))
	balance, frozen := f.walletState(t)
	if !balance.Equal(wantBalance) {
		t.Errorf("balance = %s, want %s (each request charged exactly once)", balance, wantBalance)
	}
	if !frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", frozen)
	}
	if charges := f.countTxRows(t, "charge"); charges != requests {
		t.Errorf("charge rows = %d, want %d", charges, requests)
	}
	if rows := f.countTxRows(t, ""); rows != requests+1 { // topup + one row per request
		t.Errorf("ledger rows = %d, want %d", rows, requests+1)
	}
	f.assertInvariants(t)
}
