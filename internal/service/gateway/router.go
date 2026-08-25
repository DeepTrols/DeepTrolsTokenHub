package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/channel"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrModelNotFound      = fmt.Errorf("model not found in catalog")
	ErrModelNotActive     = fmt.Errorf("model is not active")
	ErrTenantNotAllowed   = fmt.Errorf("tenant does not have access to this model")
	ErrNoChannelAvailable = fmt.Errorf("no healthy channel available")
)

type RouteResult struct {
	Channel       *domain.Channel
	Instance      *domain.ChannelInstance
	UpstreamModel string
}

// LoadSource supplies real-time in-flight counts per channel instance. The
// database current_load column is never maintained at runtime, so routing
// falls back to it only when no live source is wired or Redis errors out.
type LoadSource interface {
	Load(ctx context.Context, instanceID uuid.UUID) (int64, error)
}

type Router struct {
	models   model.Repository
	channels channel.Repository
	loads    LoadSource
}

// loadFallbackLog throttle prevents a Redis outage from flooding the logs on
// every routed request: at most one warning per instance per minute.
var (
	loadFallbackLogMu   sync.Mutex
	loadFallbackLogLast = map[string]time.Time{}
)

func NewRouter(models model.Repository, channels channel.Repository) *Router {
	return &Router{models: models, channels: channels}
}

// SetLoadSource wires a real-time load source (e.g. the Redis LoadTracker).
// Nil or disabled sources leave routing on the database current_load column.
func (r *Router) SetLoadSource(loads LoadSource) {
	r.loads = loads
}

func (r *Router) Route(ctx context.Context, identity *domain.RequestIdentity, publicModelCode string) (*RouteResult, error) {
	candidates, err := r.RouteCandidates(ctx, identity, publicModelCode, 1)
	if err != nil {
		return nil, err
	}
	return &candidates[0], nil
}

// RouteCandidates returns up to `max` routing candidates ordered by preference
// (weighted score desc), each with its best instance resolved. The gateway uses
// the full list for failover retries: if the first candidate fails at execution
// time, the next one is tried instead of failing the whole request.
func (r *Router) RouteCandidates(ctx context.Context, identity *domain.RequestIdentity, publicModelCode string, max int) ([]RouteResult, error) {
	if max <= 0 {
		max = 3
	}

	mdl, err := r.models.FindByCode(ctx, publicModelCode)
	if err != nil {
		return nil, ErrModelNotFound
	}
	if !mdl.IsCallable() {
		return nil, ErrModelNotActive
	}

	if identity.TenantID != nil {
		tenantModel, err := r.models.GetTenantModel(ctx, *identity.TenantID, publicModelCode)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				// A tenant_models lookup failure must never widen access; fail closed.
				return nil, ErrTenantNotAllowed
			}
			// pgx.ErrNoRows: no allowlist row — the tenant inherits the shared catalog.
		} else if tenantModel == nil {
			// Defensive: the repo contract is (model, nil) or (nil, pgx.ErrNoRows),
			// never (nil, nil). Fail closed rather than silently widening access.
			return nil, ErrTenantNotAllowed
		}
		// Only an explicit is_listed=false is a hard denial.
		if tenantModel != nil && !tenantModel.IsListed {
			return nil, ErrTenantNotAllowed
		}
	}

	channels, err := r.channels.ListByModel(ctx, mdl.ID, identity.TenantID)
	if err != nil || len(channels) == 0 {
		return nil, ErrNoChannelAvailable
	}

	var candidates []domain.Channel
	for _, ch := range channels {
		if ch.IsRoutable() {
			candidates = append(candidates, ch)
		}
	}
	if len(candidates) == 0 {
		return nil, ErrNoChannelAvailable
	}

	sortCandidatesByScore(candidates)
	if len(candidates) > max {
		candidates = candidates[:max]
	}

	results := make([]RouteResult, 0, len(candidates))
	for _, ch := range candidates {
		instances, err := r.channels.ListInstances(ctx, ch.ID)
		if err != nil || len(instances) == 0 {
			continue
		}
		instance := instances[0]
		bestLoad := effectiveLoad(ctx, r.loads, instance)
		for _, inst := range instances[1:] {
			if l := effectiveLoad(ctx, r.loads, inst); l < bestLoad {
				instance = inst
				bestLoad = l
			}
		}
		channelCopy := ch // avoid loop-variable aliasing
		results = append(results, RouteResult{
			Channel:       &channelCopy,
			Instance:      &instance,
			UpstreamModel: instance.ProviderRoute,
		})
	}
	if len(results) == 0 {
		return nil, ErrNoChannelAvailable
	}
	return results, nil
}

// effectiveLoad returns the real-time in-flight count when a load source is
// available, falling back to the database current_load on error or when the
// source is disabled.
func effectiveLoad(ctx context.Context, loads LoadSource, inst domain.ChannelInstance) int64 {
	if loads == nil {
		return int64(inst.CurrentLoad)
	}
	if l, err := loads.Load(ctx, inst.ID); err == nil {
		return l
	} else {
		logLoadFallback(inst.ID, err)
	}
	return int64(inst.CurrentLoad)
}

// logLoadFallback records a load-source failure at most once per minute per
// instance. Routing itself fails open to the DB current_load column, but the
// degraded mode must be observable (not silently masquerade as success).
func logLoadFallback(instanceID uuid.UUID, err error) {
	loadFallbackLogMu.Lock()
	defer loadFallbackLogMu.Unlock()
	key := instanceID.String()
	if last, ok := loadFallbackLogLast[key]; ok && time.Since(last) < time.Minute {
		return
	}
	loadFallbackLogLast[key] = time.Now()
	log.Printf("gateway: load source unavailable for instance %s, falling back to DB current_load: %v", key, err)
}

// sortCandidatesByScore orders channels by routing preference descending:
// higher weight/max-concurrency score = higher preference.
func sortCandidatesByScore(channels []domain.Channel) {
	score := func(ch domain.Channel) float64 {
		return float64(ch.Weight) / float64(ch.MaxConcurrency+1)
	}
	// insertion sort (candidate lists are tiny)
	for i := 1; i < len(channels); i++ {
		for j := i; j > 0 && score(channels[j]) > score(channels[j-1]); j-- {
			channels[j], channels[j-1] = channels[j-1], channels[j]
		}
	}
}

func selectWeightedLeastLoad(channels []domain.Channel) domain.Channel {
	best := channels[0]
	bestScore := float64(best.Weight) / float64(best.MaxConcurrency+1)

	for _, ch := range channels[1:] {
		score := float64(ch.Weight) / float64(ch.MaxConcurrency+1)
		if score > bestScore {
			best = ch
			bestScore = score
		}
	}
	return best
}
