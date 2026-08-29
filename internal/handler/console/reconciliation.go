package console

import (
	"errors"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/jackc/pgx/v5"
)

type reconciliationRunResponse struct {
	ID             string  `json:"id"`
	RunType        string  `json:"run_type"`
	Status         string  `json:"status"`
	StartedAt      string  `json:"started_at"`
	CompletedAt    *string `json:"completed_at"`
	TotalUsageLogs int     `json:"total_usage_logs"`
	MatchedCount   int     `json:"matched_count"`
	DiffCount      int     `json:"diff_count"`
	PeriodStart    string  `json:"period_start"`
	PeriodEnd      string  `json:"period_end"`
}

// HandleListReconciliationRuns returns the most recent reconciliation runs.
func HandleListReconciliationRuns(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rows, err := a.Pool.Query(ctx,
			`SELECT id, level, period_start, period_end, total_requests, diff_count,
			        status, created_at, completed_at
			 FROM reconciliation_runs
			 ORDER BY created_at DESC
			 LIMIT 50`,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list reconciliation runs"})
			return
		}
		defer rows.Close()

		runs := make([]reconciliationRunResponse, 0)
		for rows.Next() {
			var r reconciliationRunResponse
			var periodStart, periodEnd time.Time
			var startedAt time.Time
			var completedAt *time.Time
			if err := rows.Scan(&r.ID, &r.RunType, &periodStart, &periodEnd,
				&r.TotalUsageLogs, &r.DiffCount, &r.Status, &startedAt, &completedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read reconciliation run"})
				return
			}
			r.StartedAt = startedAt.Format(time.RFC3339)
			if completedAt != nil {
				formatted := completedAt.Format(time.RFC3339)
				r.CompletedAt = &formatted
			}

			r.PeriodStart = periodStart.Format(time.RFC3339)
			r.PeriodEnd = periodEnd.Format(time.RFC3339)

			// Matched count = total requests minus diffs
			r.MatchedCount = r.TotalUsageLogs - r.DiffCount

			runs = append(runs, r)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate reconciliation runs"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  runs,
			"total": len(runs),
		})
	}
}

// HandleReconciliationSummary returns L0/L1 internal reconciliation counts:
// usage_logs vs charge_lines totals and any orphan/missing gaps.
func HandleReconciliationSummary(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		count := func(q string) int64 {
			var n int64
			_ = a.Pool.QueryRow(r.Context(), q).Scan(&n)
			return n
		}
		totalUsage := count(`SELECT COUNT(*) FROM usage_logs`)
		totalCharge := count(`SELECT COUNT(*) FROM charge_lines`)
		usageMissingCharge := count(`
			SELECT COUNT(*) FROM usage_logs u
			LEFT JOIN charge_lines c ON c.usage_log_id = u.id
			WHERE c.id IS NULL`)

		// L2 internal cross-check from the latest completed run's report.
		l2 := map[string]any{
			"usage_logs":    int64(0),
			"with_charge":   int64(0),
			"with_evidence": int64(0),
			"both_missing":  int64(0),
			"balanced":      false,
			"available":     false,
		}
		var (
			l2Usage, l2Charge, l2Evidence, l2Both int64
			l2Balanced                            bool
		)
		err := a.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(report->'L2'->>'usage_logs','0')::bigint,
			        COALESCE(report->'L2'->>'with_charge','0')::bigint,
			        COALESCE(report->'L2'->>'with_evidence','0')::bigint,
			        COALESCE(report->'L2'->>'both_missing','0')::bigint,
			        COALESCE(report->'L2'->>'balanced','false')::boolean
			 FROM reconciliation_runs
			 WHERE status = 'completed'
			 ORDER BY created_at DESC
			 LIMIT 1`,
		).Scan(&l2Usage, &l2Charge, &l2Evidence, &l2Both, &l2Balanced)
		if err == nil {
			l2 = map[string]any{
				"usage_logs":    l2Usage,
				"with_charge":   l2Charge,
				"with_evidence": l2Evidence,
				"both_missing":  l2Both,
				"balanced":      l2Balanced,
				"available":     true,
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			// A transient DB error must not fail the whole summary; L2 stays
			// unavailable so the UI shows "no data" instead of a wrong number.
			l2["error"] = true
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"usage_logs":           totalUsage,
			"charge_lines":         totalCharge,
			"usage_missing_charge": usageMissingCharge,
			"balanced":             usageMissingCharge == 0,
			"l2":                   l2,
		})
	}
}
