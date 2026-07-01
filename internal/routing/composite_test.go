package routing

import (
	"context"
	"testing"
	"time"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type mockProvider struct {
	id string
}

func (m mockProvider) ID() string { return m.id }
func (m mockProvider) Models() []string { return []string{"gpt-4"} }
func (m mockProvider) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) { return nil, nil }
func (m mockProvider) HealthCheck(ctx context.Context) providers.HealthStatus { return providers.HealthStatus{Available: true} }
func (m mockProvider) Capabilities() providers.CapabilitySet { return providers.CapabilitySet{} }

func TestSelectComposite(t *testing.T) {
	healthStore := health.NewInMemoryStore()
	healthStore.EnsureProvider("p1", 3, 1)
	healthStore.EnsureProvider("p2", 3, 1)

	// Warm up both to bypass D-05 round-robin
	healthStore.RecordModelOutcome("p1", "gpt-4", true)
	healthStore.RecordModelOutcome("p1", "gpt-4", true)
	healthStore.RecordModelOutcome("p1", "gpt-4", true)
	healthStore.RecordModelOutcome("p1", "gpt-4", true)
	healthStore.RecordModelOutcome("p1", "gpt-4", true)

	healthStore.RecordModelOutcome("p2", "gpt-4", true)
	healthStore.RecordModelOutcome("p2", "gpt-4", true)
	healthStore.RecordModelOutcome("p2", "gpt-4", true)
	healthStore.RecordModelOutcome("p2", "gpt-4", true)
	healthStore.RecordModelOutcome("p2", "gpt-4", true)

	// p1 has better latency
	healthStore.EndRequest("p1", 50*time.Millisecond, nil)
	healthStore.EndRequest("p2", 200*time.Millisecond, nil)

	candidates := []providers.ProviderAdapter{
		mockProvider{"p1"},
		mockProvider{"p2"},
	}

	req := &llm.LLMRequest{Model: "gpt-4"}
	cfg := DefaultCompositeConfig()

	selected, summary, err := SelectComposite(candidates, healthStore, req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if selected.ID() != "p1" {
		t.Errorf("expected p1 to be selected, got %s", selected.ID())
	}
	if summary.ProviderID != "p1" {
		t.Errorf("expected summary provider ID to be p1")
	}

	// Make p1 degraded, should penalize and p2 wins
	healthStore.RecordProbe("p1", false, 0, "fail")
	selected, _, err = SelectComposite(candidates, healthStore, req, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.ID() != "p2" {
		t.Errorf("expected p2 to win due to p1 degradation, got %s", selected.ID())
	}
}
