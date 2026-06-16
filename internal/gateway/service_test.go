package gateway_test

import (
	"context"
	"testing"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type mockAdapter struct {
	id  string
	err error
}

func (m *mockAdapter) ID() string {
	return m.id
}
func (m *mockAdapter) Models() []string {
	return []string{"gpt-4o"}
}
func (m *mockAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{ProviderType: providers.ProviderTypeOpenAICompatible}
}
func (m *mockAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{}, m.err
}
func (m *mockAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{}
}

func TestService_HandleChatCompletion_AttemptLoopHealth(t *testing.T) {
	ctx := context.Background()
	req := &llm.LLMRequest{Model: "gpt-4o"}

	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	store.EnsureProvider("p2", 3, 1)

	p1Err := errors.NewGatewayError(errors.ProviderUnavailable, "p1 offline", 503)
	p1 := &mockAdapter{id: "p1", err: p1Err}
	p2 := &mockAdapter{id: "p2", err: nil} // success

	// Router that returns p1, then p2 (simulating exclusion logic properly implemented in HealthAwareRouter)
	registry := providers.NewRegistry(&config.Config{}, p1, p2)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin")

	admissionCtrl := admission.NewPassThroughController()

	svc := gateway.NewService(router, admissionCtrl, store, true, 2)

	// In round-robin, it should select p1 first (because they are both healthy).
	// Let's ensure p1 is picked first by manipulating internal state if needed, but round-robin
	// over [p1, p2] will pick p1 first typically.
	// We'll just run it. If p1 is picked, it fails retryably, attempts p2, succeeds.
	resp, err := svc.HandleChatCompletion(ctx, req)
	if err != nil {
		t.Fatalf("expected success on fallback, got %v", err)
	}

	if resp.Provider != "p2" && resp.Provider != "p1" {
		t.Errorf("unexpected provider: %s", resp.Provider)
	}

	// Assuming it picked p1 then p2 (or p2 then p1 depending on map iteration)
	// Both should have had Begin/End called. We can check the health snapshots!
	snap1 := store.Snapshot("p1")
	snap2 := store.Snapshot("p2")

	// One of them should have ConsecutiveFailures == 1, the other ConsecutiveFailures == 0
	if snap1.ConsecutiveFailures == 1 && snap2.ConsecutiveFailures == 0 {
		// p1 failed, p2 succeeded
	} else if snap2.ConsecutiveFailures == 1 && snap1.ConsecutiveFailures == 0 {
		// p2 failed, p1 succeeded
	} else {
		t.Errorf("expected one provider to have 1 failure and the other 0, got p1:%d, p2:%d", snap1.ConsecutiveFailures, snap2.ConsecutiveFailures)
	}
}
