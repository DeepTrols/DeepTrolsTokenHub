package gateway

import (
	"context"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/channel"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/google/uuid"
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
	RoutePolicyID *uuid.UUID
}

type Router struct {
	models   model.Repository
	channels channel.Repository
}

func NewRouter(models model.Repository, channels channel.Repository) *Router {
	return &Router{models: models, channels: channels}
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
		if err != nil || tenantModel == nil || !tenantModel.IsListed {
			return nil, ErrTenantNotAllowed
		}
	}

	channels, err := r.channels.ListByModel(ctx, mdl.ID, identity.TenantID)
	if err != nil || len(channels) == 0 {
		return nil, ErrNoChannelAvailable
	}

	// Look up active route policy for this model/tenant/user-level combination.
	var routePolicyID *uuid.UUID
	policy, _ := r.channels.FindRoutePolicy(ctx, identity.TenantID, mdl.ID, identity.UserLevel)

	if policy != nil && policy.IsActive {
		routePolicyID = &policy.ID

		if len(policy.CandidateChannelIDs) > 0 {
			filtered := filterChannelsByCandidateIDs(channels, policy.CandidateChannelIDs)
			if len(filtered) == 0 {
				switch policy.FallbackPolicy {
				case domain.FallbackDisabled:
					return nil, ErrNoChannelAvailable
				case domain.FallbackTenantDefault:
					// Use all channels from the original list.
				case domain.FallbackSharedAllowed:
					channels = filterChannelsByPoolType(channels, domain.PoolTypeShared)
				case domain.FallbackNextPolicy:
					// Use all channels as fallback (next-policy recursion not yet implemented).
				}
			} else {
				channels = filtered
			}
		}
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
		for _, inst := range instances[1:] {
			if inst.CurrentLoad < instance.CurrentLoad {
				instance = inst
			}
		}
		channelCopy := ch // avoid loop-variable aliasing
		results = append(results, RouteResult{
			Channel:       &channelCopy,
			Instance:      &instance,
			UpstreamModel: instance.ProviderRoute,
			RoutePolicyID: routePolicyID,
		})
	}
	if len(results) == 0 {
		return nil, ErrNoChannelAvailable
	}
	return results, nil
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

// filterChannelsByCandidateIDs returns only channels whose ID appears in the candidate set.
func filterChannelsByCandidateIDs(channels []domain.Channel, candidateIDs []uuid.UUID) []domain.Channel {
	idSet := make(map[uuid.UUID]bool, len(candidateIDs))
	for _, id := range candidateIDs {
		idSet[id] = true
	}
	var filtered []domain.Channel
	for _, ch := range channels {
		if idSet[ch.ID] {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}

// filterChannelsByPoolType returns only channels matching the given pool type.
func filterChannelsByPoolType(channels []domain.Channel, poolType domain.PoolType) []domain.Channel {
	var filtered []domain.Channel
	for _, ch := range channels {
		if ch.PoolType == poolType {
			filtered = append(filtered, ch)
		}
	}
	return filtered
}
