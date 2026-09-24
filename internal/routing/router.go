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
	ProviderID            string
	Strategy              string
	ComboID               string
	UpstreamModel         string
	IsFusion              bool
	FusionProviders       []FusionProvider
	FusionJudge           string
	CompositeScoreSummary *CompositeScoreSummary
}

type FusionProvider struct {
	ProviderID string
	Adapter    providers.ProviderAdapter
	Model      string
}

type Router interface {
	Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error)
	SelectExcluding(ctx context.Context, req *llm.LLMRequest, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error)
	GetProviderCapabilities() []providers.ProviderCapabilities
	GetAvailableModels() []string
}

type HealthAwareRouter struct {
	registry     *providers.Registry
	healthStore  health.Store
	strategy     string
	rrCounter    uint64
	compositeCfg *CompositeConfig
}

func NewHealthAwareRouter(registry *providers.Registry, healthStore health.Store, strategy string, compositeCfg *CompositeConfig) *HealthAwareRouter {
	return &HealthAwareRouter{
		registry:     registry,
		healthStore:  healthStore,
		strategy:     strategy,
		compositeCfg: compositeCfg,
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
		return r.selectOverride(req.RouteOverride, req)
	}
	if combo, ok := r.registry.ModelCatalog().GetCombo(req.Model); ok {
		return r.selectCombo(ctx, req, combo, excluded)
	}
	return r.selectProvider(req, excluded)
}

func (r *HealthAwareRouter) selectProvider(req *llm.LLMRequest, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	eligible := r.registry.EligibleProviders(req.Model, providers.OperationChatCompletions)
	if len(eligible) == 0 {
		return nil, RoutingDecision{}, errors.ErrNoEligibleProvider
	}
	capable := filterCandidates(eligible, req)
	if len(capable) == 0 {
		return nil, RoutingDecision{}, capabilityError(req)
	}
	healthyProviders := r.getHealthyProviders(capable, excluded)
	if len(healthyProviders) == 0 {
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
	}
	return r.selectWithStrategy(healthyProviders, req)
}

func (r *HealthAwareRouter) selectWithStrategy(candidates []providers.ProviderAdapter, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error) {
	strategyUsed := r.strategy
	var selected providers.ProviderAdapter
	switch r.strategy {
	case "least-latency":
		selected = r.selectLeastLatency(candidates)
		if selected == nil {
			selected = r.selectRoundRobin(candidates)
			strategyUsed = "least-latency-cold-start-rr"
		}
	case "round-robin":
		selected = r.selectRoundRobin(candidates)
	case "composite-score":
		return r.selectComposite(candidates, req)
	default:
		selected = r.selectRoundRobin(candidates)
		strategyUsed = "round-robin-fallback"
	}
	return selected, RoutingDecision{ProviderID: selected.ID(), Strategy: strategyUsed}, nil
}

func (r *HealthAwareRouter) selectComposite(candidates []providers.ProviderAdapter, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error) {
	cfg := DefaultCompositeConfig()
	if r.compositeCfg != nil {
		cfg = *r.compositeCfg
	}
	selected, summary, err := SelectComposite(candidates, r.healthStore, req, cfg)
	if err != nil && (err != errors.ErrCompositeScoreBelowThreshold || selected == nil) {
		return nil, RoutingDecision{}, err
	}
	decision := RoutingDecision{ProviderID: selected.ID(), Strategy: "composite-score", CompositeScoreSummary: &summary}
	return selected, decision, err
}

func extractRequirements(req *llm.LLMRequest) (requiresStream, requiresTools, requiresImage bool) {
	requiresStream = req.Stream
	requiresTools = req.ToolRequirements.UsesProtocol()
	for _, message := range req.Messages {
		for _, part := range message.MultiContent {
			if part.Type == llm.ContentTypeImageURL {
				requiresImage = true
				break
			}
		}
	}
	return
}

func (r *HealthAwareRouter) selectCombo(ctx context.Context, req *llm.LLMRequest, combo *providers.Combo, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	if combo.Strategy == "fusion" && req.ToolRequirements.UsesProtocol() {
		return nil, RoutingDecision{}, unsupportedToolCallingError()
	}
	switch combo.Strategy {
	case "round-robin":
		return r.selectRoundRobinCombo(req, combo, excluded)
	case "capacity-auto-switch":
		return r.selectCapacityCombo(req, combo, excluded)
	case "fusion":
		return r.selectFusionCombo(ctx, req, combo, excluded)
	default:
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
	}
}

func (r *HealthAwareRouter) selectRoundRobinCombo(req *llm.LLMRequest, combo *providers.Combo, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	if req.ToolRequirements.UsesProtocol() && !r.comboMembersSupport(combo, req) {
		return nil, RoutingDecision{}, capabilityError(req)
	}
	count := atomic.AddUint64(&r.rrCounter, 1)
	targetModel := combo.Members[(count-1)%uint64(len(combo.Members))]
	eligible := r.registry.EligibleProviders(targetModel, providers.OperationChatCompletions)
	healthyProviders := r.getHealthyProviders(filterCandidates(eligible, req), excluded)
	if len(healthyProviders) == 0 {
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
	}
	selected := r.selectLeastLatency(healthyProviders)
	if selected == nil {
		selected = r.selectRoundRobin(healthyProviders)
	}
	return selected, RoutingDecision{ProviderID: selected.ID(), Strategy: "combo:round-robin", ComboID: combo.ID, UpstreamModel: targetModel}, nil
}

func (r *HealthAwareRouter) selectCapacityCombo(req *llm.LLMRequest, combo *providers.Combo, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	foundCapable := false
	for _, member := range combo.Members {
		eligible := r.registry.EligibleProviders(member, providers.OperationChatCompletions)
		capable := filterCandidates(eligible, req)
		if len(capable) == 0 {
			continue
		}
		foundCapable = true
		healthyProviders := r.getHealthyProviders(capable, excluded)
		if len(healthyProviders) == 0 {
			continue
		}
		selected := r.selectLeastLatency(healthyProviders)
		if selected == nil {
			selected = r.selectRoundRobin(healthyProviders)
		}
		return selected, RoutingDecision{ProviderID: selected.ID(), Strategy: "combo:capacity-auto-switch", ComboID: combo.ID, UpstreamModel: member}, nil
	}
	if !foundCapable {
		return nil, RoutingDecision{}, capabilityError(req)
	}
	return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
}

func (r *HealthAwareRouter) selectFusionCombo(ctx context.Context, req *llm.LLMRequest, combo *providers.Combo, excluded map[string]bool) (providers.ProviderAdapter, RoutingDecision, error) {
	fusionProviders := make([]FusionProvider, 0, len(combo.Members))
	for _, member := range combo.Members {
		eligible := r.registry.EligibleProviders(member, providers.OperationChatCompletions)
		healthyProviders := r.getHealthyProviders(filterCandidates(eligible, req), excluded)
		if len(healthyProviders) == 0 {
			continue
		}
		selected := r.selectLeastLatency(healthyProviders)
		if selected == nil {
			selected = r.selectRoundRobin(healthyProviders)
		}
		fusionProviders = append(fusionProviders, FusionProvider{ProviderID: selected.ID(), Adapter: selected, Model: member})
	}
	if len(fusionProviders) == 0 {
		return nil, RoutingDecision{}, errors.ErrNoHealthyProvider
	}
	return nil, RoutingDecision{ProviderID: "fusion-ensemble", Strategy: "combo:fusion", ComboID: combo.ID, IsFusion: true, FusionProviders: fusionProviders, FusionJudge: combo.Judge}, nil
}

func (r *HealthAwareRouter) comboMembersSupport(combo *providers.Combo, req *llm.LLMRequest) bool {
	for _, member := range combo.Members {
		eligible := r.registry.EligibleProviders(member, providers.OperationChatCompletions)
		if len(eligible) == 0 || len(filterCandidates(eligible, req)) != len(eligible) {
			return false
		}
	}
	return true
}

func filterCandidates(eligible []providers.ModelProvider, req *llm.LLMRequest) []providers.ModelProvider {
	requiresStream, _, requiresImage := extractRequirements(req)
	capable := make([]providers.ModelProvider, 0, len(eligible))
	for _, candidate := range eligible {
		if !candidate.Capabilities.SatisfiesRequirements(requiresStream, false, requiresImage) {
			continue
		}
		if candidate.Capabilities.SupportsToolProtocol(req.ToolRequirements, req.Stream) {
			capable = append(capable, candidate)
		}
	}
	return capable
}

func unsupportedToolProtocolError(requirements llm.ToolProtocolRequirements) *errors.GatewayError {
	if requirements.HasExplicitChoice {
		return errors.NewGatewayError(errors.UnsupportedToolChoice, "requested provider/model does not support the requested tool choice", 400)
	}
	return unsupportedToolCallingError()
}

func unsupportedToolCallingError() *errors.GatewayError {
	return errors.NewGatewayError(errors.UnsupportedToolCalling, "requested provider/model does not support tool calling", 400)
}
func capabilityError(req *llm.LLMRequest) error {
	if req.ToolRequirements.UsesProtocol() {
		return unsupportedToolProtocolError(req.ToolRequirements)
	}
	return errors.ErrNoEligibleProvider
}

func (r *HealthAwareRouter) getHealthyProviders(eligible []providers.ModelProvider, excluded map[string]bool) []providers.ProviderAdapter {
	healthy := make([]providers.ProviderAdapter, 0, len(eligible))
	for _, provider := range eligible {
		if excluded != nil && excluded[provider.ProviderID] {
			continue
		}
		snapshot := r.healthStore.Snapshot(provider.ProviderID)
		if snapshot.Status == health.StatusUnhealthy {
			continue
		}
		adapter, err := r.registry.Get(provider.ProviderID)
		if err == nil {
			healthy = append(healthy, adapter)
		}
	}
	return healthy
}

func (r *HealthAwareRouter) selectOverride(providerID string, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error) {
	adapter, err := r.registry.Get(providerID)
	if err != nil {
		return nil, RoutingDecision{}, errors.ErrUnknownProviderOverride
	}
	catalog := r.registry.ModelCatalog()
	if !catalog.ProviderSupports(providerID, req.Model, providers.OperationChatCompletions) {
		return nil, RoutingDecision{}, errors.ErrIneligibleProviderOverride
	}
	capabilities, found := catalog.ProviderCapabilities(providerID, req.Model)
	requiresStream, _, requiresImage := extractRequirements(req)
	if !found || !capabilities.SatisfiesRequirements(requiresStream, false, requiresImage) {
		return nil, RoutingDecision{}, errors.ErrIneligibleProviderOverride
	}
	if !capabilities.SupportsToolProtocol(req.ToolRequirements, req.Stream) {
		return nil, RoutingDecision{}, capabilityError(req)
	}
	if r.healthStore.Snapshot(providerID).Status == health.StatusUnhealthy {
		return nil, RoutingDecision{}, errors.ErrUnhealthyProviderOverride
	}
	return adapter, RoutingDecision{ProviderID: adapter.ID(), Strategy: "override"}, nil
}

func (r *HealthAwareRouter) selectRoundRobin(candidates []providers.ProviderAdapter) providers.ProviderAdapter {
	count := atomic.AddUint64(&r.rrCounter, 1)
	return candidates[(count-1)%uint64(len(candidates))]
}

func (r *HealthAwareRouter) selectLeastLatency(candidates []providers.ProviderAdapter) providers.ProviderAdapter {
	var best providers.ProviderAdapter
	var lowestLatency int64 = -1
	for _, candidate := range candidates {
		latency := int64(r.healthStore.Snapshot(candidate.ID()).EWMALatency)
		if latency > 0 && (lowestLatency == -1 || latency < lowestLatency) {
			lowestLatency = latency
			best = candidate
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
