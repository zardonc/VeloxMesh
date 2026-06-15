package routing

import (
	"context"
	"sync/atomic"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type RoutingDecision struct {
	ProviderID string
	Strategy   string
}

type Router interface {
	Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error)
	GetAvailableModels() []string
}

type HealthAwareRouter struct {
	registry    *providers.Registry
	healthStore health.Store
	strategy    string
	rrCounter   uint64
}

func NewHealthAwareRouter(registry *providers.Registry, healthStore health.Store, strategy string) *HealthAwareRouter {
	return &HealthAwareRouter{
		registry:    registry,
		healthStore: healthStore,
		strategy:    strategy,
	}
}

func (r *HealthAwareRouter) Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error) {
	if req.RouteOverride != "" {
		return r.selectOverride(req.RouteOverride)
	}

	healthyProviders := r.getHealthyProviders()
	if len(healthyProviders) == 0 {
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
	}

	var selected providers.ProviderAdapter
	strategyUsed := r.strategy

	switch r.strategy {
	case "least-latency":
		selected = r.selectLeastLatency(healthyProviders)
		if selected == nil {
			// Cold start fallback
			selected = r.selectRoundRobin(healthyProviders)
			strategyUsed = "least-latency-cold-start-rr"
		}
	case "round-robin":
		selected = r.selectRoundRobin(healthyProviders)
	default:
		// Default to round-robin if unknown
		selected = r.selectRoundRobin(healthyProviders)
		strategyUsed = "round-robin-fallback"
	}

	return selected, RoutingDecision{
		ProviderID: selected.ID(),
		Strategy:   strategyUsed,
	}, nil
}

func (r *HealthAwareRouter) getHealthyProviders() []providers.ProviderAdapter {
	var healthy []providers.ProviderAdapter
	for _, p := range r.registry.List() {
		snap := r.healthStore.Snapshot(p.ID())
		if snap.Status != health.StatusUnhealthy {
			healthy = append(healthy, p)
		}
	}
	return healthy
}

func (r *HealthAwareRouter) selectOverride(providerID string) (providers.ProviderAdapter, RoutingDecision, error) {
	adapter, err := r.registry.Get(providerID)
	if err != nil {
		return nil, RoutingDecision{}, errors.ErrUnknownProviderOverride
	}

	snap := r.healthStore.Snapshot(providerID)
	if snap.Status == health.StatusUnhealthy {
		return nil, RoutingDecision{}, errors.ErrUnhealthyProviderOverride
	}

	return adapter, RoutingDecision{
		ProviderID: adapter.ID(),
		Strategy:   "override",
	}, nil
}

func (r *HealthAwareRouter) selectRoundRobin(candidates []providers.ProviderAdapter) providers.ProviderAdapter {
	count := atomic.AddUint64(&r.rrCounter, 1)
	idx := (count - 1) % uint64(len(candidates))
	return candidates[idx]
}

func (r *HealthAwareRouter) selectLeastLatency(candidates []providers.ProviderAdapter) providers.ProviderAdapter {
	var best providers.ProviderAdapter
	var lowestLatency int64 = -1

	for _, p := range candidates {
		snap := r.healthStore.Snapshot(p.ID())
		if snap.EWMALatency > 0 {
			if lowestLatency == -1 || int64(snap.EWMALatency) < lowestLatency {
				lowestLatency = int64(snap.EWMALatency)
				best = p
			}
		}
	}

	return best
}

func (r *HealthAwareRouter) GetAvailableModels() []string {
	return r.registry.GetAllModels()
}
