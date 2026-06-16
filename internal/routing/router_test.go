package routing_test

import (
	"context"
	"testing"
	"time"
	"veloxmesh/internal/config"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type mockAdapter struct {
	id     string
	models []string
}

func (m *mockAdapter) ID() string {
	return m.id
}

func (m *mockAdapter) Models() []string {
	return m.models
}

func (m *mockAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, nil
}

func (m *mockAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{ProviderType: providers.ProviderTypeOpenAICompatible}
}

func (m *mockAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}

func TestHealthAwareRouter_Select(t *testing.T) {
	cfg := &config.Config{DefaultProvider: "p1"}
	p1 := &mockAdapter{id: "p1"}
	p2 := &mockAdapter{id: "p2"}
	registry := providers.NewRegistry(cfg, p1, p2)
	healthStore := health.NewInMemoryStore()

	healthStore.EnsureProvider("p1", 3, 1)
	healthStore.EnsureProvider("p2", 3, 1)

	ctx := context.Background()

	t.Run("RoundRobin", func(t *testing.T) {
		router := routing.NewHealthAwareRouter(registry, healthStore, "round-robin")
		req := &llm.LLMRequest{}

		a1, dec1, err := router.Select(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		a2, dec2, err := router.Select(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		if a1.ID() == a2.ID() {
			t.Errorf("expected round robin to select different providers, got %s both times", a1.ID())
		}
		if dec1.Strategy != "round-robin" || dec2.Strategy != "round-robin" {
			t.Errorf("expected strategy round-robin")
		}
	})

	t.Run("LeastLatency", func(t *testing.T) {
		router := routing.NewHealthAwareRouter(registry, healthStore, "least-latency")
		req := &llm.LLMRequest{}

		// Give p2 lower latency
		healthStore.BeginRequest("p1")
		healthStore.EndRequest("p1", 200*time.Millisecond, nil)
		healthStore.BeginRequest("p2")
		healthStore.EndRequest("p2", 100*time.Millisecond, nil)

		a, dec, err := router.Select(ctx, req)
		if err != nil {
			t.Fatal(err)
		}

		if a.ID() != "p2" {
			t.Errorf("expected p2 (lowest latency), got %s", a.ID())
		}
		if dec.Strategy != "least-latency" {
			t.Errorf("expected strategy least-latency")
		}
	})

	t.Run("LeastLatency Cold Start", func(t *testing.T) {
		store2 := health.NewInMemoryStore()
		store2.EnsureProvider("p1", 3, 1)
		store2.EnsureProvider("p2", 3, 1)
		router := routing.NewHealthAwareRouter(registry, store2, "least-latency")
		req := &llm.LLMRequest{}

		_, dec, err := router.Select(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if dec.Strategy != "least-latency-cold-start-rr" {
			t.Errorf("expected cold start fallback, got %s", dec.Strategy)
		}
	})

	t.Run("Unhealthy Skipped", func(t *testing.T) {
		store2 := health.NewInMemoryStore()
		store2.EnsureProvider("p1", 3, 1)
		store2.EnsureProvider("p2", 3, 1)
		router := routing.NewHealthAwareRouter(registry, store2, "round-robin")

		// Make p1 unhealthy
		store2.BeginRequest("p1")
		store2.EndRequest("p1", 0, errors.NewGatewayError("test", "test", 500))
		store2.BeginRequest("p1")
		store2.EndRequest("p1", 0, errors.NewGatewayError("test", "test", 500))
		store2.BeginRequest("p1")
		store2.EndRequest("p1", 0, errors.NewGatewayError("test", "test", 500))

		req := &llm.LLMRequest{}
		for i := 0; i < 3; i++ {
			a, _, err := router.Select(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			if a.ID() != "p2" {
				t.Errorf("expected p2 because p1 is unhealthy, got %s", a.ID())
			}
		}
	})

	t.Run("All Unhealthy", func(t *testing.T) {
		store2 := health.NewInMemoryStore()
		store2.EnsureProvider("p1", 3, 1)
		router2 := routing.NewHealthAwareRouter(providers.NewRegistry(cfg, p1), store2, "round-robin")

		// Make p1 unhealthy
		for i := 0; i < 3; i++ {
			store2.BeginRequest("p1")
			store2.EndRequest("p1", 0, errors.NewGatewayError("test", "test", 500))
		}

		req := &llm.LLMRequest{}
		_, _, err := router2.Select(ctx, req)
		if err != errors.ErrNoHealthyProvider {
			t.Errorf("expected ErrNoHealthyProvider, got %v", err)
		}
	})

	t.Run("Override Unhealthy", func(t *testing.T) {
		store2 := health.NewInMemoryStore()
		store2.EnsureProvider("p1", 3, 1)
		router2 := routing.NewHealthAwareRouter(providers.NewRegistry(cfg, p1), store2, "round-robin")

		for i := 0; i < 3; i++ {
			store2.BeginRequest("p1")
			store2.EndRequest("p1", 0, errors.NewGatewayError("test", "test", 500))
		}

		req := &llm.LLMRequest{RouteOverride: "p1"}
		_, _, err := router2.Select(ctx, req)
		if err != errors.ErrUnhealthyProviderOverride {
			t.Errorf("expected ErrUnhealthyProviderOverride, got %v", err)
		}
	})

	t.Run("Override Unknown", func(t *testing.T) {
		router2 := routing.NewHealthAwareRouter(registry, healthStore, "round-robin")
		req := &llm.LLMRequest{RouteOverride: "unknown"}
		_, _, err := router2.Select(ctx, req)
		if err != errors.ErrUnknownProviderOverride {
			t.Errorf("expected ErrUnknownProviderOverride, got %v", err)
		}
	})

	t.Run("SelectExcluding", func(t *testing.T) {
		store2 := health.NewInMemoryStore()
		store2.EnsureProvider("p1", 3, 1)
		store2.EnsureProvider("p2", 3, 1)
		router2 := routing.NewHealthAwareRouter(registry, store2, "round-robin")
		req := &llm.LLMRequest{}

		excluded := map[string]bool{"p1": true}
		a, _, err := router2.SelectExcluding(ctx, req, excluded)
		if err != nil {
			t.Fatal(err)
		}
		if a.ID() != "p2" {
			t.Errorf("expected p2 since p1 is excluded, got %s", a.ID())
		}

		excludedAll := map[string]bool{"p1": true, "p2": true}
		_, _, err = router2.SelectExcluding(ctx, req, excludedAll)
		if err != errors.ErrNoHealthyProvider {
			t.Errorf("expected ErrNoHealthyProvider when all excluded, got %v", err)
		}

		// Ensure excluded overrides health (unhealthy p2 + excluded p1)
		store2.BeginRequest("p2")
		store2.EndRequest("p2", 0, errors.NewGatewayError("test", "test", 500))
		store2.BeginRequest("p2")
		store2.EndRequest("p2", 0, errors.NewGatewayError("test", "test", 500))
		store2.BeginRequest("p2")
		store2.EndRequest("p2", 0, errors.NewGatewayError("test", "test", 500))

		_, _, err = router2.SelectExcluding(ctx, req, excluded)
		if err != errors.ErrNoHealthyProvider {
			t.Errorf("expected ErrNoHealthyProvider when available is unhealthy, got %v", err)
		}
	})
}
