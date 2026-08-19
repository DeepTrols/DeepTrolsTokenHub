package app

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// healthHandler returns a simple health-check response.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeHealth(w, http.StatusOK, "ok")
}

// healthzHandler is the liveness probe: the process is up, so it always
// answers 200. Dependency health belongs to readyzHandler.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	writeHealth(w, http.StatusOK, "ok")
}

// readyzHandler is the readiness probe: it pings the database and, when
// configured, Redis, and answers 503 until every required dependency is
// reachable. Redis is optional in single-instance dev mode (nil client);
// when configured but unreachable the probe fails closed.
func readyzHandler(a *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]string{}
		ready := true
		if a.Pool != nil {
			if err := a.Pool.Ping(ctx); err != nil {
				checks["database"] = "error"
				ready = false
			} else {
				checks["database"] = "ok"
			}
		}
		if a.Redis != nil {
			if err := a.Redis.Ping(ctx).Err(); err != nil {
				checks["redis"] = "error"
				ready = false
			} else {
				checks["redis"] = "ok"
			}
		}

		status := "ready"
		code := http.StatusOK
		if !ready {
			status = "not_ready"
			code = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]any{"status": status, "checks": checks})
	}
}

func writeHealth(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}
