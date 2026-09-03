package gateway

// TH-P05-02 (B5 Settle Fallback Visibility Correction).
//
// Every gateway settle site funnels through settleOrFallback so that a
// rejected settle is never disguised and never silently drops evidence:
//
//   - ErrInsufficientBalance (final cost > reserve, no available balance):
//     commit the reserved hold, charge = hold, evidence "undercharged".
//     The shortfall stays visible in usage_logs for reconciliation review —
//     reconciliation never mutates wallets itself.
//   - ErrTxNotReserved (idempotent replay of an already-finalized request):
//     the first pass already moved the money. No commit, no release, no
//     undercharge flag — touching the wallet again would double-debit.
//   - any other error (infrastructure failure): commit the reserved hold
//     (value was already consumed upstream, releasing would lose money) and
//     record "settle_error", or "undercharged" when the committed hold is
//     provably below the final cost.

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Evidence codes written to usage_logs.error_code by the settle fallback.
const (
	settleEvidenceUndercharged = "undercharged"
	settleEvidenceSettleError  = "settle_error"
)

// settleFailureClass is the classified reaction to a rejected settle.
type settleFailureClass struct {
	// commitReserved: fall back to committing the reserved hold so the
	// frozen money is still collected (never released after the upstream
	// call consumed real value).
	commitReserved bool
	// undercharged: the wallet provably cannot cover the final cost; the
	// evidence chain must carry the undercharge flag.
	undercharged bool
	// evidenceCode is written to usage_logs.error_code ("" = no code).
	evidenceCode string
}

// classifySettleFailure maps a Charger.Settle error to a fallback class.
// A nil error is not a fallback and yields the zero class.
func classifySettleFailure(err error) settleFailureClass {
	switch {
	case err == nil:
		return settleFailureClass{}
	case errors.Is(err, wallet.ErrInsufficientBalance):
		return settleFailureClass{commitReserved: true, undercharged: true, evidenceCode: settleEvidenceUndercharged}
	case errors.Is(err, wallet.ErrTxNotReserved):
		// Replay of an already-finalized transaction: the original pass is
		// authoritative and no money moved on the rejected call.
		return settleFailureClass{}
	default:
		return settleFailureClass{commitReserved: true, undercharged: false, evidenceCode: settleEvidenceSettleError}
	}
}

// label names the class for log lines.
func (c settleFailureClass) label() string {
	switch {
	case c.undercharged:
		return "insufficient_balance"
	case c.commitReserved:
		return "settle_error"
	default:
		return "replay"
	}
}

// settleOrFallback settles the reserved transaction against the final cost
// and, when the settle is rejected, applies the classified fallback. It
// returns the amount actually charged to the wallet and the evidence code
// ("" when the settle left no anomaly to record). The reservedHold is the
// amount frozen by the original Reserve and is what the commit fallback
// charges.
func settleOrFallback(
	ctx context.Context,
	application *app.App,
	endpoint, model string,
	r *http.Request,
	txID uuid.UUID,
	finalCost, reservedHold decimal.Decimal,
) (decimal.Decimal, string) {
	settleErr := application.Charger.Settle(ctx, txID, finalCost)
	if settleErr == nil {
		return finalCost, ""
	}

	class := classifySettleFailure(settleErr)
	log.Printf("gateway: settle_fallback endpoint=%s model=%s request_id=%s class=%s tx=%s err=%v",
		endpoint, model, requestIDForSettleLog(r), class.label(), txID, settleErr)

	if !class.commitReserved {
		// Idempotent replay: money already moved on the first pass. No
		// commit, no release, no undercharge flag (AC: no double debit).
		return decimal.Zero, ""
	}

	if commitErr := application.Charger.Commit(ctx, txID); commitErr != nil {
		log.Printf("gateway: settle_fallback commit error endpoint=%s tx=%s: %v", endpoint, txID, commitErr)
		if errors.Is(commitErr, wallet.ErrTxNotReserved) {
			// A concurrent pass already finalized the transaction: this
			// call moved no money, so report no charge.
			return decimal.Zero, ""
		}
	}

	if class.undercharged || finalCost.GreaterThan(reservedHold) {
		// Charged hold < provable list cost: keep the shortfall visible.
		countUnderchargeFallback(endpoint, model)
		metrics.IncUnderchargeFallback(endpoint) // TH-P05-04
		return reservedHold, settleEvidenceUndercharged
	}
	return reservedHold, class.evidenceCode
}

// requestIDForSettleLog extracts the middleware request id for settle
// fallback log lines (request id only — no user/wallet identities).
func requestIDForSettleLog(r *http.Request) string {
	if r == nil {
		return ""
	}
	if rid, ok := r.Context().Value(middleware.CtxRequestID).(string); ok && rid != "" {
		return rid
	}
	return ""
}

// ---------------------------------------------------------------------------
// Undercharge fallback counters (observability requirement of TH-P05-02).
// A process-local registry keyed by "endpoint|model"; exported snapshot for
// future metrics wiring (metrics infrastructure lands with TH-P05-04).
// ---------------------------------------------------------------------------

var (
	settleFallbackMu     sync.Mutex
	settleFallbackCounts = make(map[string]*atomic.Int64)
)

// countUnderchargeFallback records one undercharge fallback event.
func countUnderchargeFallback(endpoint, model string) {
	key := endpoint + "|" + model
	settleFallbackMu.Lock()
	counter, ok := settleFallbackCounts[key]
	if !ok {
		counter = &atomic.Int64{}
		settleFallbackCounts[key] = counter
	}
	settleFallbackMu.Unlock()
	counter.Add(1)
}

// UnderchargeFallbackCounts returns a snapshot of the undercharge fallback
// counters keyed by "endpoint|model". Safe for concurrent use.
func UnderchargeFallbackCounts() map[string]int64 {
	settleFallbackMu.Lock()
	defer settleFallbackMu.Unlock()
	snapshot := make(map[string]int64, len(settleFallbackCounts))
	for key, counter := range settleFallbackCounts {
		snapshot[key] = counter.Load()
	}
	return snapshot
}
