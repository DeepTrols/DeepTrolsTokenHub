package app

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/domain"
)

// PublicStatsHandler returns lightweight, unauthenticated platform stats for
// public pages (e.g. the login page's "在线模型" counter). Only aggregate,
// non-sensitive counts are exposed. Model count reflects routable models only
// (those backed by at least one active channel), matching every catalog list.
func PublicStatsHandler(a *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var models int64
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*)
			 FROM models m
			 WHERE m.status = $1
			   AND EXISTS (
			     SELECT 1 FROM channels c
			     WHERE c.model_id = m.id AND c.status = 'active'
			   )`,
			string(domain.ModelStatusActive),
		).Scan(&models); err != nil {
			log.Printf("public stats: count models: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to load stats"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]int64{"models": models})
	}
}
