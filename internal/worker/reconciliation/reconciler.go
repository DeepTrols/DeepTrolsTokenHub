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

// Run executes one reconciliation cycle combining L0 (usage vs charge_lines)
// and L1 (usage vs provider_evidence) checks over the last hour.
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

	totalDiffs := len(orphaned) + len(missingEvidence) + len(mismatches) + len(mislabels)

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

	if diffErrs > 0 {
		return fmt.Errorf("reconciler: %d of %d diffs failed to persist", diffErrs, totalDiffs)
	}

	log.Printf("reconciler: L1 run %s: %d diffs (%d orphaned, %d missing evidence, %d usage mismatch, %d error mislabel)",
		runID, totalDiffs, len(orphaned), len(missingEvidence), len(mismatches), len(mislabels))
	return nil
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
	_, err = r.pool.Exec(ctx,
		`INSERT INTO reconciliation_diffs (id, run_id, usage_log_id, diff_type, severity, diff_detail, resolution_status)
		 VALUES ($1, $2, $3, $4, $5, $6, 'open')`,
		uuid.New(), runID, uid, diffType, severity, detail)
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
