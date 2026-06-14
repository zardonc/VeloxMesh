package routing

import (
	"context"
	"fmt"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type RoutingDecision struct {
	ProviderID string
	Strategy   string
}

type Router interface {
	Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error)
}

type StaticRouter struct {
	registry *providers.Registry
}

func NewStaticRouter(registry *providers.Registry) *StaticRouter {
	return &StaticRouter{registry: registry}
}

func (r *StaticRouter) Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, RoutingDecision, error) {
	var adapter providers.ProviderAdapter
	var err error
	var strategy string

	if req.RouteOverride != "" {
		adapter, err = r.registry.Get(req.RouteOverride)
		if err != nil {
			return nil, RoutingDecision{}, fmt.Errorf("route override failed: %w", err)
		}
		strategy = "override"
	} else {
		adapter, err = r.registry.GetDefault()
		if err != nil {
			return nil, RoutingDecision{}, fmt.Errorf("default route failed: %w", err)
		}
		strategy = "default"
	}

	return adapter, RoutingDecision{
		ProviderID: adapter.ID(),
		Strategy:   strategy,
	}, nil
}
