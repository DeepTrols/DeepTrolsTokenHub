package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// dbPool abstracts a subset of *pgxpool.Pool for testing.
type dbPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Compile-time interface conformance check.
var _ dbPool = (*pgxpool.Pool)(nil)

// Reconciler runs periodic financial reconciliation between usage_logs and provider_evidence.
type Reconciler struct {
	pool dbPool
}

// New creates a new Reconciler.
func New(pool *pgxpool.Pool) *Reconciler {
	return &Reconciler{pool: pool}
}

// Run executes one reconciliation cycle combining L0 (usage vs charge_lines),
// L1 (usage vs provider_evidence), L3 (internal usage vs external billing
// records) and L4 (gateway undercharge flags) checks over the last hour.
func (r *Reconciler) Run(ctx context.Context) error {
	runID := uuid.New()
	periodEnd := time.Now().UTC()
	periodStart := periodEnd.Add(-1 * time.Hour)

	if err := r.createRun(ctx, runID, periodStart, periodEnd); err != nil {
		return fmt.Errorf("reconciler: create run: %w", err)
	}

	usageCount, chargeCount, err := r.countLogs(ctx, periodStart, periodEnd)
	if err != nil {
		r.markRunFailed(ctx, runID)
		return fmt.Errorf("reconciler: count logs: %w", err)
	}

	evidenceCount, err := r.countEvidence(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: countEvidence: %v", err)
		evidenceCount = -1 // distinguish "query failed" from "zero"
	}

	// L0: usage_logs with no charge_lines.
	orphaned, err := r.findOrphaned(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findOrphaned: %v", err)
	}

	// L1: evidence gaps, token mismatches, and error mislabels.
	missingEvidence, err := r.findMissingEvidence(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findMissingEvidence: %v", err)
	}
	mismatches, err := r.findUsageMismatch(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findUsageMismatch: %v", err)
	}
	mislabels, err := r.findErrorMislabel(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findErrorMislabel: %v", err)
	}

	// L2: internal cross-check of L0 (charge_lines) vs L1 (provider_evidence)
	// coverage for completed usage logs — a usage row missing both is one
	// root cause, and the quadrant counts expose coverage drift.
	l2, err := r.runL2(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: runL2: %v", err)
	}

	// L3: external billing records (billing_records, synced from OneAPI /
	// NewAPI / Aliyun) vs internal usage evidence.
	billingCount, err := r.countBillingRecords(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: countBillingRecords: %v", err)
		billingCount = -1
	}
	billingWithoutUsage, err := r.findBillingWithoutUsage(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findBillingWithoutUsage: %v", err)
	}
	usageWithoutBilling, err := r.findUsageWithoutBilling(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findUsageWithoutBilling: %v", err)
	}
	amountMismatches, err := r.findBillingAmountMismatch(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findBillingAmountMismatch: %v", err)
	}

	// Money integrity (TH-P05-02): usage_logs the gateway flagged as
	// undercharged (settle could not cover the final cost, the reserved hold
	// was committed instead). These become review diffs — reconciliation
	// NEVER mutates wallet state; an operator decides the remedy.
	undercharged, err := r.findUndercharged(ctx, periodStart, periodEnd)
	if err != nil {
		log.Printf("reconciler: findUndercharged: %v", err)
	}

	totalDiffs := len(orphaned) + len(missingEvidence) + len(mismatches) + len(mislabels) +
		len(billingWithoutUsage) + len(usageWithoutBilling) + len(amountMismatches) + len(undercharged)

	report := map[string]interface{}{
		"level":        "L1",
		"period_start": periodStart.Format(time.RFC3339),
		"period_end":   periodEnd.Format(time.RFC3339),
		"L0": map[string]interface{}{
			"usage_logs":     usageCount,
			"charge_lines":   chargeCount,
			"orphaned_count": len(orphaned),
		},
		"L1": map[string]interface{}{
			"evidence_count":   evidenceCount,
			"missing_evidence": len(missingEvidence),
			"usage_mismatch":   len(mismatches),
			"error_mislabel":   len(mislabels),
		},
		"L2": map[string]interface{}{
			"usage_logs":    l2.UsageLogs,
			"with_charge":   l2.WithCharge,
			"with_evidence": l2.WithEvidence,
			"both_missing":  l2.BothMissing,
			"balanced":      l2.WithCharge == l2.UsageLogs && l2.WithEvidence == l2.UsageLogs,
		},
		"L3": map[string]interface{}{
			"billing_records":       billingCount,
			"billing_without_usage": len(billingWithoutUsage),
			"usage_without_billing": len(usageWithoutBilling),
			"amount_mismatch":       len(amountMismatches),
		},
		"L4": map[string]interface{}{
			"undercharged": len(undercharged),
		},
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		log.Printf("reconciler: marshal report: %v", err)
		reportJSON = json.RawMessage(`{"error":"marshal_failed"}`)
	}

	if err := r.completeRun(ctx, runID, usageCount, totalDiffs, reportJSON); err != nil {
		return fmt.Errorf("reconciler: complete run: %w", err)
	}

	// Persist all discrepancies. A failed write means a discrepancy went
	// unrecorded — surface it rather than reporting false consistency.
	diffErrs := 0
	writeDiff := func(usageLogID, diffType, severity, detail string) {
		if err := r.createDiff(ctx, runID, usageLogID, diffType, severity, detail); err != nil {
			log.Printf("reconciler: createDiff for %s: %v", usageLogID, err)
			diffErrs++
		}
	}
	for _, o := range orphaned {
		writeDiff(o, "missing_charge_lines", "warning",
			fmt.Sprintf("usage_log %s has no charge_lines", o))
	}
	for _, id := range missingEvidence {
		writeDiff(id, "missing_evidence", "warning",
			fmt.Sprintf("usage_log %s has no provider_evidence", id))
	}
	for _, m := range mismatches {
		detail := fmt.Sprintf(`{"log_total_tokens": %d, "evidence_total_tokens": %d}`,
			m.LogTokens, m.EvidenceTokens)
		writeDiff(m.UsageLogID, "usage_mismatch", "warning", detail)
	}
	for _, m := range mislabels {
		detail := fmt.Sprintf(`{"evidence_status_code": %d, "usage_log_status": "completed"}`,
			m.StatusCode)
		writeDiff(m.UsageLogID, "error_mislabel", "critical", detail)
	}
	writeBillingDiff := func(diffType, severity, detail string) {
		if err := r.createBillingDiff(ctx, runID, diffType, severity, detail); err != nil {
			log.Printf("reconciler: createBillingDiff for %s: %v", diffType, err)
			diffErrs++
		}
	}
	for _, b := range billingWithoutUsage {
		writeBillingDiff("billing_without_usage", "warning",
			fmt.Sprintf(`{"external_request_id": %q, "net_amount": %q, "connector_id": %q}`,
				b.ExternalRequestID, b.NetAmount, b.ConnectorID))
	}
	for _, u := range usageWithoutBilling {
		writeDiff(u.UsageLogID, "usage_without_billing", "warning",
			fmt.Sprintf(`{"provider_request_id": %q}`, u.ProviderRequestID))
	}
	for _, m := range amountMismatches {
		detail := fmt.Sprintf(`{"external_request_id": %q, "billing_net_amount": %q, "usage_upstream_cost": %q}`,
			m.ExternalRequestID, m.BillingNetAmount, m.UpstreamCost)
		writeDiff(m.UsageLogID, "billing_amount_mismatch", "warning", detail)
	}
	for _, u := range undercharged {
		// Critical: money was charged below the provable list cost. The diff
		// is review input only — no automatic wallet correction (TH-P05-02).
		detail := fmt.Sprintf(`{"list_cost":%q,"wallet_charged":%q}`, u.ListCost, u.WalletCharged)
		writeDiff(u.UsageLogID, "undercharge_review", "critical", detail)
	}

	if diffErrs > 0 {
		return fmt.Errorf("reconciler: %d of %d diffs failed to persist", diffErrs, totalDiffs)
	}

	log.Printf("reconciler: run %s: %d diffs (L0 orphaned=%d, L1 missing=%d mismatch=%d mislabel=%d, L3 no_usage=%d no_billing=%d amount=%d, L4 undercharged=%d)",
		runID, totalDiffs, len(orphaned), len(missingEvidence), len(mismatches), len(mislabels),
		len(billingWithoutUsage), len(usageWithoutBilling), len(amountMismatches), len(undercharged))
	return nil
}

// ---------------------------------------------------------------------------
// L3: internal usage evidence vs external billing records.
// ---------------------------------------------------------------------------

func (r *Reconciler) countBillingRecords(ctx context.Context, start, end time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM billing_records WHERE usage_start_at >= $1 AND usage_start_at < $2`,
		start, end).Scan(&count)
	return count, err
}

type billingWithoutUsage struct {
	ConnectorID       string
	ExternalRequestID string
	NetAmount         string
}

// findBillingWithoutUsage finds billing_records whose external_request_id does
// not match any usage_log in the period (provider paid for a call we never
// recorded, or the billing connector picked up a foreign request).
func (r *Reconciler) findBillingWithoutUsage(ctx context.Context, start, end time.Time) ([]billingWithoutUsage, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT br.connector_id::text, br.external_request_id, br.net_amount
		 FROM billing_records br
		 WHERE br.usage_start_at >= $1 AND br.usage_start_at < $2
		   AND NOT EXISTS (
		     SELECT 1 FROM usage_logs ul
		     WHERE (ul.provider_request_id = br.external_request_id OR ul.request_id = br.external_request_id)
		       AND ul.created_at >= $1 AND ul.created_at < $2
		   )
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []billingWithoutUsage
	for rows.Next() {
		var it billingWithoutUsage
		if err := rows.Scan(&it.ConnectorID, &it.ExternalRequestID, &it.NetAmount); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

type usageWithoutBilling struct {
	UsageLogID        string
	ProviderRequestID string
}

// findUsageWithoutBilling finds charged usage_logs with a provider_request_id
// that no billing_record references (we billed the customer but the provider
// bill has no matching line).
func (r *Reconciler) findUsageWithoutBilling(ctx context.Context, start, end time.Time) ([]usageWithoutBilling, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text, COALESCE(ul.provider_request_id, '')
		 FROM usage_logs ul
		 WHERE ul.created_at >= $1 AND ul.created_at < $2
		   AND COALESCE(ul.upstream_cost, 0) > 0
		   AND COALESCE(ul.provider_request_id, '') <> ''
		   AND NOT EXISTS (
		     SELECT 1 FROM billing_records br
		     WHERE br.external_request_id = ul.provider_request_id
		   )
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []usageWithoutBilling
	for rows.Next() {
		var it usageWithoutBilling
		if err := rows.Scan(&it.UsageLogID, &it.ProviderRequestID); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

type billingAmountMismatch struct {
	UsageLogID        string
	ExternalRequestID string
	BillingNetAmount  string
	UpstreamCost      string
}

// findBillingAmountMismatch finds matched billing_records whose net amount
// differs from the internal upstream_cost by more than one cent (CNY).
func (r *Reconciler) findBillingAmountMismatch(ctx context.Context, start, end time.Time) ([]billingAmountMismatch, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text, br.external_request_id, br.net_amount, ul.upstream_cost::text
		 FROM billing_records br
		 JOIN usage_logs ul
		   ON (ul.provider_request_id = br.external_request_id OR ul.request_id = br.external_request_id)
		  AND ul.created_at >= $1 AND ul.created_at < $2
		 WHERE br.usage_start_at >= $1 AND br.usage_start_at < $2
		   AND ABS(COALESCE(NULLIF(br.net_amount, '')::decimal, 0) - COALESCE(ul.upstream_cost, 0)) > 0.01
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []billingAmountMismatch
	for rows.Next() {
		var it billingAmountMismatch
		if err := rows.Scan(&it.UsageLogID, &it.ExternalRequestID, &it.BillingNetAmount, &it.UpstreamCost); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// underchargedLog describes a usage_log the gateway marked with
// error_code='undercharged': the settle could not cover the final cost, so
// the reserved hold was committed and the shortfall left for review.
type underchargedLog struct {
	UsageLogID    string
	ListCost      string
	WalletCharged string
}

// findUndercharged finds usage_logs flagged undercharged within the period.
func (r *Reconciler) findUndercharged(ctx context.Context, start, end time.Time) ([]underchargedLog, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text, ul.list_cost::text, ul.wallet_charged::text
		 FROM usage_logs ul
		 WHERE ul.created_at >= $1 AND ul.created_at < $2
		   AND ul.error_code = 'undercharged'
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []underchargedLog
	for rows.Next() {
		var it underchargedLog
		if err := rows.Scan(&it.UsageLogID, &it.ListCost, &it.WalletCharged); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// createBillingDiff records a diff without a usage_log reference (e.g. an
// upstream billing line with no matching internal call).
func (r *Reconciler) createBillingDiff(ctx context.Context, runID uuid.UUID, diffType, severity, detail string) error {
	detailJSON, err := diffDetailJSON(detail)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO reconciliation_diffs (id, run_id, usage_log_id, diff_type, severity, diff_detail, resolution_status)
		 VALUES ($1, $2, NULL, $3, $4, $5, 'open')`,
		uuid.New(), runID, diffType, severity, detailJSON)
	return err
}

// diffDetailJSON normalizes a diff detail into a JSONB-safe value. Details
// that are already JSON objects are passed through; plain-text details are
// encoded as JSON strings so they never violate the JSONB column.
func diffDetailJSON(detail string) (json.RawMessage, error) {
	if json.Valid([]byte(detail)) {
		return json.RawMessage(detail), nil
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return nil, fmt.Errorf("diffDetailJSON: %w", err)
	}
	return encoded, nil
}

func (r *Reconciler) createRun(ctx context.Context, id uuid.UUID, start, end time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO reconciliation_runs (id, level, period_start, period_end, status)
		 VALUES ($1, 'L1', $2, $3, 'running')`, id, start, end)
	return err
}

func (r *Reconciler) countLogs(ctx context.Context, start, end time.Time) (int, int, error) {
	var usageCount, chargeCount int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM usage_logs WHERE created_at >= $1 AND created_at < $2`,
		start, end).Scan(&usageCount)
	if err != nil {
		return 0, 0, err
	}
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM charge_lines cl
		 JOIN usage_logs ul ON cl.usage_log_id = ul.id
		 WHERE ul.created_at >= $1 AND ul.created_at < $2`,
		start, end).Scan(&chargeCount)
	return usageCount, chargeCount, err
}

// l2Quadrants is the L2 internal cross-check snapshot: completed usage logs
// classified by whether they carry charge_lines (L0) and provider_evidence
// (L1). both_missing rows indicate a single root cause behind both levels.
type l2Quadrants struct {
	UsageLogs    int
	WithCharge   int
	WithEvidence int
	BothMissing  int
}

// runL2 cross-checks L0/L1 coverage for completed usage logs in the period.
func (r *Reconciler) runL2(ctx context.Context, start, end time.Time) (l2Quadrants, error) {
	var q l2Quadrants
	err := r.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*),
		   COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM charge_lines cl WHERE cl.usage_log_id = ul.id)),
		   COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM provider_evidence pe WHERE pe.usage_log_id = ul.id)),
		   COUNT(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM charge_lines cl WHERE cl.usage_log_id = ul.id)
		                     AND NOT EXISTS (SELECT 1 FROM provider_evidence pe WHERE pe.usage_log_id = ul.id))
		 FROM usage_logs ul
		 WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.status = 'completed'`,
		start, end).Scan(&q.UsageLogs, &q.WithCharge, &q.WithEvidence, &q.BothMissing)
	return q, err
}

// countEvidence returns the number of provider_evidence records linked to
// usage_logs within the period. It is the L1 counterpart to countLogs.
func (r *Reconciler) countEvidence(ctx context.Context, start, end time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM provider_evidence pe
		 JOIN usage_logs ul ON pe.usage_log_id = ul.id
		 WHERE ul.created_at >= $1 AND ul.created_at < $2`,
		start, end).Scan(&count)
	return count, err
}

// errorMislabel describes a completed usage_log whose provider_evidence
// recorded an HTTP error — i.e. a failed upstream call was billed as success.
type errorMislabel struct {
	UsageLogID string
	StatusCode int
}

// findErrorMislabel finds completed usage_logs whose evidence has status >= 400.
func (r *Reconciler) findErrorMislabel(ctx context.Context, start, end time.Time) ([]errorMislabel, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text, pe.status_code FROM usage_logs ul
		 JOIN provider_evidence pe ON pe.usage_log_id = ul.id
		 WHERE ul.created_at >= $1 AND ul.created_at < $2
		   AND ul.status = 'completed' AND pe.status_code >= 400
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []errorMislabel
	for rows.Next() {
		var it errorMislabel
		if err := rows.Scan(&it.UsageLogID, &it.StatusCode); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// usageMismatch describes a completed usage_log whose recorded total_tokens
// disagrees with the token count in its provider_evidence.usage_raw.
type usageMismatch struct {
	UsageLogID     string
	LogTokens      int
	EvidenceTokens int
}

// findUsageMismatch finds rows where both sides recorded non-zero total_tokens
// but the values differ, indicating a counting error in the billing pipeline.
func (r *Reconciler) findUsageMismatch(ctx context.Context, start, end time.Time) ([]usageMismatch, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text,
		        COALESCE((ul.usage_raw->>'total_tokens')::int, 0),
		        COALESCE((pe.usage_raw->>'total_tokens')::int, 0)
		 FROM usage_logs ul
		 JOIN provider_evidence pe ON pe.usage_log_id = ul.id
		 WHERE ul.created_at >= $1 AND ul.created_at < $2
		   AND ul.status = 'completed'
		   AND COALESCE((ul.usage_raw->>'total_tokens')::int, 0) !=
		       COALESCE((pe.usage_raw->>'total_tokens')::int, 0)
		   AND COALESCE((ul.usage_raw->>'total_tokens')::int, 0) > 0
		   AND COALESCE((pe.usage_raw->>'total_tokens')::int, 0) > 0
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []usageMismatch
	for rows.Next() {
		var it usageMismatch
		if err := rows.Scan(&it.UsageLogID, &it.LogTokens, &it.EvidenceTokens); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// findMissingEvidence finds completed usage_logs with no provider_evidence record.
func (r *Reconciler) findMissingEvidence(ctx context.Context, start, end time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text FROM usage_logs ul
		 LEFT JOIN provider_evidence pe ON pe.usage_log_id = ul.id
		 WHERE ul.created_at >= $1 AND ul.created_at < $2
		   AND ul.status = 'completed' AND pe.id IS NULL
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Reconciler) findOrphaned(ctx context.Context, start, end time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ul.id::text FROM usage_logs ul
		 LEFT JOIN charge_lines cl ON cl.usage_log_id = ul.id
		 WHERE ul.created_at >= $1 AND ul.created_at < $2
		   AND cl.id IS NULL AND ul.status = 'completed'
		 LIMIT 100`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Reconciler) createDiff(ctx context.Context, runID uuid.UUID, usageLogID, diffType, severity, detail string) error {
	uid, err := uuid.Parse(usageLogID)
	if err != nil {
		return fmt.Errorf("createDiff invalid usage_log_id %q: %w", usageLogID, err)
	}
	detailJSON, err := diffDetailJSON(detail)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO reconciliation_diffs (id, run_id, usage_log_id, diff_type, severity, diff_detail, resolution_status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'open')`,
		uuid.New(), runID, uid, diffType, severity, detailJSON)
	return err
}

func (r *Reconciler) completeRun(ctx context.Context, id uuid.UUID, total, diffCount int, report json.RawMessage) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reconciliation_runs
		 SET total_requests = $2, diff_count = $3, status = 'completed', report = $4, completed_at = $5
		 WHERE id = $1`,
		id, total, diffCount, report, time.Now().UTC())
	return err
}

func (r *Reconciler) markRunFailed(ctx context.Context, id uuid.UUID) {
	_, err := r.pool.Exec(ctx,
		`UPDATE reconciliation_runs SET status = 'failed', completed_at = $2 WHERE id = $1`,
		id, time.Now().UTC())
	if err != nil {
		log.Printf("reconciler: markRunFailed: %v", err)
	}
}
