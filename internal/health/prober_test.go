package health_test

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/config"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type mockAdapter struct {
	id     string
	status providers.HealthStatus
}

func (m *mockAdapter) ID() string       { return m.id }
func (m *mockAdapter) Models() []string { return []string{"m1"} }
func (m *mockAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, nil
}

func (m *mockAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{ProviderType: providers.ProviderTypeOpenAICompatible}
}
func (m *mockAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return m.status
}

func TestProber_ProbeProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{ID: "p1"}},
		HealthCheck: config.HealthCheckConfig{
			Timeout: "1s",
		},
	}

	adapter1 := &mockAdapter{
		id:     "p1",
		status: providers.HealthStatus{Available: true, Message: "ok"},
	}
	adapter2 := &mockAdapter{
		id:     "p2",
		status: providers.HealthStatus{Available: false, Message: "degraded"},
	}

	registry := providers.NewRegistry(cfg, adapter1, adapter2)
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	store.EnsureProvider("p2", 3, 1)

	prober := health.NewProber(registry, store, cfg, nil)

	// Test successful probe
	res := prober.ProbeProvider(context.Background(), "p1")
	if !res.Available {
		t.Errorf("expected p1 to be available")
	}
	snap := store.Snapshot("p1")
	if !snap.LastProbeSuccess {
		t.Errorf("expected store to record success for p1")
	}

	// Test failed probe
	res = prober.ProbeProvider(context.Background(), "p2")
	if res.Available {
		t.Errorf("expected p2 to be unavailable")
	}
	if res.Message != "degraded" {
		t.Errorf("expected degraded message, got %s", res.Message)
	}
	snap = store.Snapshot("p2")
	if snap.LastProbeSuccess {
		t.Errorf("expected store to record failure for p2")
	}

	// Test unknown provider
	res = prober.ProbeProvider(context.Background(), "unknown")
	if res.Available {
		t.Errorf("expected unknown to be unavailable")
	}
}

func TestProber_ProbeOnce(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{ID: "p1"}, {ID: "p2"}},
		HealthCheck: config.HealthCheckConfig{
			MaxConcurrency: 2,
		},
	}

	adapter1 := &mockAdapter{
		id:     "p1",
		status: providers.HealthStatus{Available: true},
	}
	adapter2 := &mockAdapter{
		id:     "p2",
		status: providers.HealthStatus{Available: false},
	}

	registry := providers.NewRegistry(cfg, adapter1, adapter2)
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	store.EnsureProvider("p2", 3, 1)

	prober := health.NewProber(registry, store, cfg, nil)

	prober.ProbeOnce(context.Background())

	if !store.Snapshot("p1").LastProbeSuccess {
		t.Errorf("p1 should have successful probe")
	}
	if store.Snapshot("p2").LastProbeSuccess {
		t.Errorf("p2 should have failed probe")
	}
}

func TestProber_StartStop(t *testing.T) {
	cfg := &config.Config{
		HealthCheck: config.HealthCheckConfig{
			Interval: "10ms",
		},
	}

	registry := providers.NewRegistry(cfg)
	store := health.NewInMemoryStore()
	prober := health.NewProber(registry, store, cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		prober.Start(ctx)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond) // Let it run a few ticks
	cancel()

	select {
	case <-done:
		// Clean exit
	case <-time.After(1 * time.Second):
		t.Errorf("Start didn't return after context cancel")
	}
}
