package console

import (
	"net/http"
	"runtime"
	"time"

	"github.com/deeptrols/api/internal/app"
)

var processStart = time.Now()

// HandleSystemInfo returns aggregate platform counts and runtime info
// (new-api system-info / dashboard overview, adapted to DeepTrols).
func HandleSystemInfo(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		queries := map[string]string{
			"users":     "SELECT COUNT(*) FROM users",
			"models":    "SELECT COUNT(*) FROM models",
			"channels":  "SELECT COUNT(*) FROM channels",
			"instances": "SELECT COUNT(*) FROM channel_instances",
			"wallets":   "SELECT COUNT(*) FROM wallets",
			"usage":     "SELECT COUNT(*) FROM usage_logs",
			"orders":    "SELECT COUNT(*) FROM payment_orders",
		}
		counts := map[string]int64{}
		for name, q := range queries {
			var n int64
			_ = a.Pool.QueryRow(r.Context(), q).Scan(&n)
			counts[name] = n
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"version":     "0.1.0",
			"go_version":  runtime.Version(),
			"uptime_secs": int64(time.Since(processStart).Seconds()),
			"counts":      counts,
		})
	}
}
