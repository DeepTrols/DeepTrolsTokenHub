package gateway

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"sort"
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

// FilterByGroup restricts candidates to channels whose group matches the
// caller's group. A caller group of "" imposes no restriction, and a channel
// with no group is always eligible (backward compatible).
func FilterByGroup(candidates []RouteResult, callerGroup string) []RouteResult {
	if callerGroup == "" {
		return candidates
	}
	out := make([]RouteResult, 0, len(candidates))
	for _, c := range candidates {
		if c.Channel == nil || c.Channel.GroupName == "" || c.Channel.GroupName == callerGroup {
			out = append(out, c)
		}
	}
	return out
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
	affinity AffinityStore
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

// EnableAffinity wires the channel-affinity store (Redis or in-memory).
func (r *Router) EnableAffinity(store AffinityStore) {
	r.affinity = store
}

// RecordAffinity remembers the channel chosen for a user + model so the next
// request prefers the same upstream to improve cache-hit rates.
func (r *Router) RecordAffinity(ctx context.Context, userID, model, channelID string) {
	if r.affinity == nil || userID == "" || model == "" || channelID == "" {
		return
	}
	_ = r.affinity.Set(ctx, affinityKey(userID, model), channelID, affinityTTL)
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

	// Phase 2 routing: fallback_order groups → strategy ordering → sticky
	// session → request-hash rotation (rendezvous approximation).
	routingKey := identity.RequestID
	if routingKey == "" {
		routingKey = identity.UserID.String() + publicModelCode
	}
	candidates = orderChannels(candidates, routingKey)
	if r.affinity != nil && identity != nil && identity.UserID != uuid.Nil {
		candidates = applyAffinity(ctx, r.affinity, identity.UserID.String(), publicModelCode, candidates)
	}
	if len(candidates) > max {
		candidates = candidates[:max]
	}

	results := make([]RouteResult, 0, len(candidates))
	now := time.Now().UTC()
	for _, ch := range candidates {
		instances, err := r.channels.ListInstances(ctx, ch.ID)
		if err != nil || len(instances) == 0 {
			continue
		}
		// Skip instances in cooldown or already at their concurrency limit;
		// among the rest pick the one with the lowest real-time load.
		var instance *domain.ChannelInstance
		var bestLoad int64 = 1<<63 - 1
		for i := range instances {
			inst := &instances[i]
			if inst.CooldownUntil != nil && inst.CooldownUntil.After(now) {
				continue
			}
			load := effectiveLoad(ctx, r.loads, *inst)
			if inst.ConcurrencyLimit > 0 && load >= int64(inst.ConcurrencyLimit) {
				continue
			}
			if load < bestLoad {
				instance = inst
				bestLoad = load
			}
		}
		if instance == nil {
			continue
		}
		channelCopy := ch // avoid loop-variable aliasing
		results = append(results, RouteResult{
			Channel:       &channelCopy,
			Instance:      instance,
			UpstreamModel: instance.ProviderRoute,
		})
	}
	if len(results) == 0 {
		return nil, ErrNoChannelAvailable
	}
	// Remember the preferred channel so the next request from the same
	// user + model stays on the same upstream (best-effort, TTL-bounded).
	if r.affinity != nil && identity != nil && identity.UserID != uuid.Nil {
		r.RecordAffinity(ctx, identity.UserID.String(), publicModelCode, results[0].Channel.ID.String())
	}
	return results, nil
}

// orderChannels applies Phase 2 channel ordering: stable grouping by
// fallback_order (ascending), then per-group strategy ordering with sticky
// session preference and request-hash rotation. The input slice is not
// mutated.
func orderChannels(channels []domain.Channel, routingKey string) []domain.Channel {
	if len(channels) <= 1 {
		return channels
	}
	groups := map[int][]domain.Channel{}
	var orders []int
	for _, ch := range channels {
		if _, ok := groups[ch.FallbackOrder]; !ok {
			orders = append(orders, ch.FallbackOrder)
		}
		groups[ch.FallbackOrder] = append(groups[ch.FallbackOrder], ch)
	}
	sort.Ints(orders)
	ordered := make([]domain.Channel, 0, len(channels))
	for _, order := range orders {
		ordered = append(ordered, orderGroup(groups[order], routingKey)...)
	}
	return ordered
}

// orderGroup orders one fallback tier: sticky channels first (deterministic
// by routing key), then non-sticky channels by strategy (quality → health,
// otherwise weight/max_concurrency score), then a request-hash rotation so
// different requests spread across siblings.
func orderGroup(group []domain.Channel, routingKey string) []domain.Channel {
	if len(group) <= 1 {
		return group
	}
	var sticky, rest []domain.Channel
	for _, ch := range group {
		if ch.StickySession {
			sticky = append(sticky, ch)
		} else {
			rest = append(rest, ch)
		}
	}
	out := make([]domain.Channel, 0, len(group))
	if len(sticky) > 0 {
		idx := hashIndex(routingKey, len(sticky))
		out = append(out, sticky[idx])
		sticky = append(sticky[:idx], sticky[idx+1:]...)
		out = append(out, sticky...)
	}
	if len(rest) > 1 {
		if containsQualityStrategy(rest) {
			sort.SliceStable(rest, func(i, j int) bool {
				if rest[i].HealthScore != rest[j].HealthScore {
					return rest[i].HealthScore > rest[j].HealthScore
				}
				return routeScore(rest[i]) > routeScore(rest[j])
			})
		} else {
			sortCandidatesByScore(rest)
		}
		if routingKey != "" && scoresEqual(rest) {
			rot := hashIndex(routingKey, len(rest))
			if rot > 0 {
				rest = append(rest[rot:], rest[:rot]...)
			}
		}
	}
	out = append(out, rest...)
	return out
}

// scoresEqual reports whether all channels in the group have identical
// routing scores, i.e. they are interchangeable siblings. Only then is
// request-hash rotation applied, so weighted orderings stay deterministic.
func scoresEqual(channels []domain.Channel) bool {
	if len(channels) < 2 {
		return true
	}
	base := routeScore(channels[0])
	for _, ch := range channels[1:] {
		if routeScore(ch) != base {
			return false
		}
	}
	return true
}

func containsQualityStrategy(channels []domain.Channel) bool {
	for _, ch := range channels {
		if ch.Strategy == domain.RouteStrategyQuality {
			return true
		}
	}
	return false
}

func routeScore(ch domain.Channel) float64 {
	return float64(ch.Weight) / float64(ch.MaxConcurrency+1)
}

// hashIndex returns a deterministic index in [0, n) for a routing key.
func hashIndex(key string, n int) int {
	if n <= 0 || key == "" {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(n))
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
