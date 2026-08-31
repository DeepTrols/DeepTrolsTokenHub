package console

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/service/rankings"
)

// rankingsCacheTTL mirrors new-api's 5-minute snapshot cache: leaderboards are
// read-heavy analytics and do not need second-level freshness.
const rankingsCacheTTL = 5 * time.Minute

type rankingsCacheItem struct {
	expiresAt time.Time
	data      *rankings.Snapshot
}

var (
	rankingsCacheMu sync.Mutex
	rankingsCache   = map[string]rankingsCacheItem{}
)

// HandleRankings implements GET /api/public/rankings (port of new-api's
// GetRankings): model/vendor leaderboards, movers/droppers and token history
// aggregated from usage_logs for the requested period.
func HandleRankings(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := rankingsConfig(r.URL.Query().Get("period"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !settingBool(a, r, "models_public_visible") {
			writeJSON(w, http.StatusOK, rankings.BuildSnapshot(cfg, nil, nil, nil))
			return
		}

		now := time.Now().UTC()
		rankingsCacheMu.Lock()
		if item, ok := rankingsCache[cfg.ID]; ok && now.Before(item.expiresAt) {
			rankingsCacheMu.Unlock()
			writeJSON(w, http.StatusOK, item.data)
			return
		}
		rankingsCacheMu.Unlock()

		totals, err := queryRankingTotals(a, r, cfg.Start, cfg.End)
		if err != nil {
			log.Printf("console: rankings totals: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to aggregate usage"})
			return
		}
		previous, err := queryRankingTotals(a, r, cfg.PrevStart, cfg.PrevEnd)
		if err != nil {
			log.Printf("console: rankings previous totals: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to aggregate usage"})
			return
		}
		buckets, err := queryRankingBuckets(a, r, cfg)
		if err != nil {
			log.Printf("console: rankings buckets: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to aggregate usage"})
			return
		}

		snap := rankings.BuildSnapshot(cfg, totals, previous, buckets)
		rankingsCacheMu.Lock()
		rankingsCache[cfg.ID] = rankingsCacheItem{expiresAt: now.Add(rankingsCacheTTL), data: snap}
		rankingsCacheMu.Unlock()
		writeJSON(w, http.StatusOK, snap)
	}
}

// rankingsConfig resolves the period window, previous-period window and bucket
// granularity.
func rankingsConfig(period string) (rankings.Config, error) {
	now := time.Now().UTC()
	day := 24 * time.Hour
	switch period {
	case "", "week":
		return rankings.Config{
			ID: "week", Start: now.Add(-7 * day), End: now,
			PrevStart: now.Add(-14 * day), PrevEnd: now.Add(-7 * day),
			BucketTrunc: "day", LabelLayout: "01-02",
		}, nil
	case "today":
		return rankings.Config{
			ID: "today", Start: now.Add(-day), End: now,
			PrevStart: now.Add(-2 * day), PrevEnd: now.Add(-day),
			BucketTrunc: "hour", LabelLayout: "15:04",
		}, nil
	case "month":
		return rankings.Config{
			ID: "month", Start: now.Add(-30 * day), End: now,
			PrevStart: now.Add(-60 * day), PrevEnd: now.Add(-30 * day),
			BucketTrunc: "day", LabelLayout: "01-02",
		}, nil
	case "year":
		return rankings.Config{
			ID: "year", Start: now.Add(-365 * day), End: now,
			PrevStart: now.Add(-730 * day), PrevEnd: now.Add(-365 * day),
			BucketTrunc: "week", LabelLayout: "01-02",
		}, nil
	default:
		return rankings.Config{}, fmt.Errorf("invalid ranking period: %s", period)
	}
}

func queryRankingTotals(a *app.App, r *http.Request, start, end time.Time) ([]rankings.ModelTotal, error) {
	rows, err := a.Pool.Query(r.Context(),
		`SELECT ul.public_model_code, COALESCE(m.provider, ''),
		        COALESCE(SUM((ul.usage_normalized->>'total_tokens')::bigint), 0) AS tokens
		 FROM usage_logs ul
		 LEFT JOIN models m ON m.code = ul.public_model_code
		 WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.status <> 'failed'
		 GROUP BY ul.public_model_code, m.provider`,
		start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []rankings.ModelTotal{}
	for rows.Next() {
		var mt rankings.ModelTotal
		if err := rows.Scan(&mt.Model, &mt.Provider, &mt.TotalTokens); err != nil {
			return nil, err
		}
		out = append(out, mt)
	}
	return out, rows.Err()
}

func queryRankingBuckets(a *app.App, r *http.Request, cfg rankings.Config) ([]rankings.BucketPoint, error) {
	rows, err := a.Pool.Query(r.Context(),
		`SELECT date_trunc($3, ul.created_at) AS bucket,
		        ul.public_model_code, COALESCE(m.provider, ''),
		        COALESCE(SUM((ul.usage_normalized->>'total_tokens')::bigint), 0) AS tokens
		 FROM usage_logs ul
		 LEFT JOIN models m ON m.code = ul.public_model_code
		 WHERE ul.created_at >= $1 AND ul.created_at < $2 AND ul.status <> 'failed'
		 GROUP BY 1, 2, 3`,
		cfg.Start, cfg.End, cfg.BucketTrunc)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []rankings.BucketPoint{}
	for rows.Next() {
		var bp rankings.BucketPoint
		if err := rows.Scan(&bp.Bucket, &bp.Model, &bp.Provider, &bp.Tokens); err != nil {
			return nil, err
		}
		out = append(out, bp)
	}
	return out, rows.Err()
}
