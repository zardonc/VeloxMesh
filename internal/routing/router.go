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
	ProviderID      string
	Strategy        string
	ComboID         string
	IsFusion        bool
	FusionProviders []providers.ProviderAdapter
	FusionJudge     string
}

type Router interface {
	Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error)
	SelectExcluding(ctx context.Context, req *llm.LLMRequest, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error)
	GetProviderCapabilities() []providers.ProviderCapabilities
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
	return r.SelectExcluding(ctx, req, nil)
}

func (r *HealthAwareRouter) SelectExcluding(ctx context.Context, req *llm.LLMRequest, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	if !r.registry.HasConfiguredProviders() {
		return nil, RoutingDecision{}, errors.ErrNoActiveProviderConfig
	}

	if req.RouteOverride != "" {
		return r.selectOverride(req.RouteOverride, req.Model)
	}

	combo, isCombo := r.registry.ModelCatalog().GetCombo(req.Model)
	if isCombo {
		return r.selectCombo(ctx, combo, excluded)
	}

	eligible := r.registry.EligibleProviders(req.Model, providers.OperationChatCompletions)
	if len(eligible) == 0 {
		return nil, RoutingDecision{}, errors.ErrNoEligibleProvider
	}

	healthyProviders := r.getHealthyProviders(eligible, excluded)
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

func (r *HealthAwareRouter) selectCombo(ctx context.Context, combo *providers.Combo, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	switch combo.Strategy {
	case "round-robin":
		// round-robin across members
		count := atomic.AddUint64(&r.rrCounter, 1)
		idx := (count - 1) % uint64(len(combo.Members))
		targetModel := combo.Members[idx]

		eligible := r.registry.EligibleProviders(targetModel, providers.OperationChatCompletions)
		healthyProviders := r.getHealthyProviders(eligible, excluded)
		if len(healthyProviders) == 0 {
			return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
		}

		selected := r.selectLeastLatency(healthyProviders)
		if selected == nil {
			selected = r.selectRoundRobin(healthyProviders)
		}

		return selected, RoutingDecision{
			ProviderID: selected.ID(),
			Strategy:   "combo:round-robin",
			ComboID:    combo.ID,
		}, nil

	case "capacity-auto-switch":
		// iterate through members until we find one with a healthy, non-excluded provider
		for _, member := range combo.Members {
			eligible := r.registry.EligibleProviders(member, providers.OperationChatCompletions)
			healthyProviders := r.getHealthyProviders(eligible, excluded)
			if len(healthyProviders) > 0 {
				selected := r.selectLeastLatency(healthyProviders)
				if selected == nil {
					selected = r.selectRoundRobin(healthyProviders)
				}
				return selected, RoutingDecision{
					ProviderID: selected.ID(),
					Strategy:   "combo:capacity-auto-switch",
					ComboID:    combo.ID,
				}, nil
			}
		}
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider

	case "fusion":
		// Fusion requires multiple providers
		var fusionAdapters []providers.ProviderAdapter
		for _, member := range combo.Members {
			eligible := r.registry.EligibleProviders(member, providers.OperationChatCompletions)
			healthyProviders := r.getHealthyProviders(eligible, excluded) // should we exclude for fusion? yes, if one failed. Actually fusion handles its own partial failures typically, but let's respect excluded.
			if len(healthyProviders) > 0 {
				selected := r.selectLeastLatency(healthyProviders)
				if selected == nil {
					selected = r.selectRoundRobin(healthyProviders)
				}
				fusionAdapters = append(fusionAdapters, selected)
			}
		}
		if len(fusionAdapters) == 0 {
			return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
		}

		return nil, RoutingDecision{
			ProviderID:      "fusion-ensemble",
			Strategy:        "combo:fusion",
			ComboID:         combo.ID,
			IsFusion:        true,
			FusionProviders: fusionAdapters,
			FusionJudge:     combo.Judge,
		}, nil

	default:
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider // or unsupported strategy
	}
}

func (r *HealthAwareRouter) getHealthyProviders(eligible []providers.ModelProvider, excluded map[string]bool) []providers.ProviderAdapter {
	var healthy []providers.ProviderAdapter
	for _, pInfo := range eligible {
		if excluded != nil && excluded[pInfo.ProviderID] {
			continue
		}
		snap := r.healthStore.Snapshot(pInfo.ProviderID)
		if snap.Status != health.StatusUnhealthy {
			if adapter, err := r.registry.Get(pInfo.ProviderID); err == nil {
				healthy = append(healthy, adapter)
			}
		}
	}
	return healthy
}

func (r *HealthAwareRouter) selectOverride(providerID string, model string) (providers.ProviderAdapter, RoutingDecision, error) {
	adapter, err := r.registry.Get(providerID)
	if err != nil {
		return nil, RoutingDecision{}, errors.ErrUnknownProviderOverride
	}

	if !r.registry.ProviderSupports(providerID, model, providers.OperationChatCompletions) {
		return nil, RoutingDecision{}, errors.ErrIneligibleProviderOverride
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

func (r *HealthAwareRouter) GetProviderCapabilities() []providers.ProviderCapabilities {
	return r.registry.AllCapabilities()
}

func (r *HealthAwareRouter) GetAvailableModels() []string {
	return r.registry.GetAllModels()
}
