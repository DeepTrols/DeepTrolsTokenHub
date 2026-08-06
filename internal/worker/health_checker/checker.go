package health_checker

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// doer abstracts HTTP requests for testability.
// http.DefaultClient satisfies this interface.
type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Checker periodically probes channel instances and updates health scores.
type Checker struct {
	pool       *pgxpool.Pool
	httpClient doer
}

// New creates a new health Checker. The HTTP client carries a 10s timeout so
// a hung upstream /health endpoint cannot block the whole check cycle.
func New(pool *pgxpool.Pool) *Checker {
	return &Checker{
		pool: pool,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Run executes one health check cycle.
func (c *Checker) Run(ctx context.Context) (int, error) {
	ids, err := c.fetchActiveChannels(ctx)
	if err != nil {
		return 0, fmt.Errorf("health_checker: fetch: %w", err)
	}

	checked := 0
	for _, chID := range ids {
		if err := c.checkChannel(ctx, chID); err != nil {
			log.Printf("health_checker: channel %s: %v", chID, err)
			continue
		}
		checked++
	}
	return checked, nil
}

func (c *Checker) fetchActiveChannels(ctx context.Context) ([]string, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT id FROM channels WHERE status = 'active'`)
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

// checkChannel updates health for an active channel using a progressive
// score: each probe cycle moves the score by ±30 (0..100), so a single
// blip degrades gradually and recovery is gradual too. Status mapping:
// ≥70 healthy, 30–69 degraded (still routable), <30 unhealthy.
func (c *Checker) checkChannel(ctx context.Context, chID string) error {
	// Load the current score for gradual adjustment.
	var currentScore int
	if err := c.pool.QueryRow(ctx, `SELECT health_score FROM channels WHERE id = $1`, chID).Scan(&currentScore); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("channel %s not found", chID)
		}
		return fmt.Errorf("read channel health: %w", err)
	}

	// Fetch channel instances to get BaseURLs for probing.
	rows, err := c.pool.Query(ctx,
		`SELECT base_url FROM channel_instances WHERE channel_id = $1 AND status = 'active'`,
		chID)
	if err != nil {
		return err
	}
	defer rows.Close()

	anyHealthy := false
	for rows.Next() {
		var baseURL string
		if err := rows.Scan(&baseURL); err != nil {
			return err
		}
		if err := c.probeHealth(ctx, baseURL); err == nil {
			anyHealthy = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	score := currentScore
	if anyHealthy {
		score += healthStep
		if score > 100 {
			score = 100
		}
	} else {
		score -= healthStep
		if score < 0 {
			score = 0
		}
	}
	status := healthStatusForScore(score)

	tag, err := c.pool.Exec(ctx,
		`UPDATE channels SET health_score = $1, health_status = $2, updated_at = $3 WHERE id = $4`,
		score, status, time.Now().UTC(), chID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("channel %s not found", chID)
	}
	return nil
}

const healthStep = 30

// healthStatusForScore maps a progressive score to a health status.
func healthStatusForScore(score int) string {
	switch {
	case score >= 70:
		return "healthy"
	case score >= 30:
		return "degraded"
	default:
		return "unhealthy"
	}
}

// probeHealth sends a GET request to baseURL/health and reports reachability.
// Any response below 500 (2xx/3xx/4xx) counts as healthy: OpenAI-compatible
// providers often have no /health endpoint and answer 401 (auth required) or
// 404 (not found), which still prove the upstream is reachable. Only a 5xx
// response or a transport-level failure (timeout, DNS, connection refused)
// marks the channel unhealthy.
func (c *Checker) probeHealth(ctx context.Context, baseURL string) error {
	url := baseURL + "/health"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("probeHealth: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("probeHealth: %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("probeHealth: %s returned %d", url, resp.StatusCode)
	}
	return nil
}
