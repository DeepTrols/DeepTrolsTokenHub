package console

import (
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
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
