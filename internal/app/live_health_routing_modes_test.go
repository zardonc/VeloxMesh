//go:build phase29preflight

package app

// Failure cases: combo/override routing hides dependency failure; a readable
// unhealthy provider is admitted; one failed dependency blocks a healthy peer.
import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"veloxmesh/internal/config"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/routing"
)

type liveHealthRoutingFixture struct {
	proxy  *liveNetworkProxy
	store  *health.RedisStore
	router *routing.HealthAwareRouter
	model  string
}

func liveHealthRoutes(t *testing.T, env map[string]string) liveHealthRoutingFixture {
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	client, err := hotstate.NewRedisClient(context.Background(), proxy.listener.Addr().String(), "", 0, fmt.Sprint("routing-health-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := health.NewRedisStore(client, "1m")
	store.EnsureProvider("diagnostic", 1, 1)
	model := env["SANS_PRIMARY_DEFAULT_MODEL"]
	adapter := openai.NewAdapter("diagnostic", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], model)
	combos := []providers.Combo{}
	for _, strategy := range []string{"round-robin", "capacity-auto-switch", "fusion"} {
		combos = append(combos, providers.Combo{ID: strategy, Name: strategy, Strategy: strategy, Members: []string{model}, Judge: model})
	}
	registry := providers.NewRegistry(&config.Config{DefaultProvider: "diagnostic"}, []providers.ProviderAdapter{adapter}, combos)
	return liveHealthRoutingFixture{proxy: proxy, store: store, router: routing.NewHealthAwareRouter(registry, store, "round-robin", nil), model: model}
}

func TestLiveHealthDependencyRoutes(t *testing.T) {
	env := liveEnvironment(t)
	fixture := liveHealthRoutes(t, env)
	requests := []llm.LLMRequest{{Model: fixture.model}, {Model: fixture.model, RouteOverride: "diagnostic"},
		{Model: "round-robin"}, {Model: "capacity-auto-switch"}, {Model: "fusion"}}
	for _, request := range requests {
		fixture.proxy.delay.Store(int64(60 * time.Millisecond))
		_, _, err := fixture.router.Select(context.Background(), &request)
		shipLogJSON(t, map[string]any{"type": "health_dependency_route", "model": request.Model, "override": request.RouteOverride, "error": fmt.Sprint(err)})
		var problem *gatewayerrors.GatewayError
		if !errors.As(err, &problem) || problem.Code != "health_state_unavailable" || problem.HTTPStatus != 503 {
			t.Errorf("health read failure lost its category: %v", err)
		}
		_, _, recovered := fixture.router.Select(context.Background(), &request)
		if recovered != nil {
			t.Fatalf("dependency recovery failed: %v", recovered)
		}
	}
	fixture.store.EndRequest("diagnostic", time.Millisecond, fmt.Errorf("injected genuine provider failure"))
	for _, request := range requests {
		_, _, err := fixture.router.Select(context.Background(), &request)
		expected := gatewayerrors.ErrNoHealthyProvider
		if request.RouteOverride != "" {
			expected = gatewayerrors.ErrUnhealthyProviderOverride
		}
		if err != expected {
			t.Errorf("readable unhealthy provider was not rejected: %v", err)
		}
	}
}

func TestLiveHealthDependencyHealthyPeer(t *testing.T) {
	env := liveEnvironment(t)
	fixture := liveHealthRoutes(t, env)
	fixture.store.EnsureProvider("peer", 1, 1)
	adapters := []providers.ProviderAdapter{
		openai.NewAdapter("diagnostic", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], fixture.model),
		openai.NewAdapter("peer", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], fixture.model),
	}
	registry := providers.NewRegistry(&config.Config{DefaultProvider: "diagnostic"}, adapters, nil)
	router := routing.NewHealthAwareRouter(registry, fixture.store, "round-robin", nil)
	fixture.proxy.delay.Store(int64(60 * time.Millisecond))
	selected, _, err := router.Select(context.Background(), &llm.LLMRequest{Model: fixture.model})
	if err != nil || selected == nil || selected.ID() != "peer" {
		t.Fatalf("readable healthy peer did not remain available: adapter=%v error=%v", selected, err)
	}
	shipLogJSON(t, map[string]any{"type": "health_dependency_peer", "selected": selected.ID()})
}
