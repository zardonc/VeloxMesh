package gateway

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/routing"
)

func TestFusionTerminalUsesSharedOutcomeContract(t *testing.T) {
	usage := &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}
	tests := []struct {
		name              string
		err               error
		settles           int
		expectedSuccesses int
		expectedFailures  int
	}{
		{name: "completed settles once", settles: 1, expectedSuccesses: 1},
		{name: "provider error does not settle", err: gatewayerrors.NewGatewayError(gatewayerrors.ProviderUnavailable, "unavailable", 503), expectedFailures: 1},
		{name: "client cancellation does not settle", err: context.Canceled, expectedSuccesses: 1},
		{name: "policy rejection does not settle", err: gatewayerrors.ErrPolicyBlocked, expectedSuccesses: 1},
		{name: "internal abnormal does not settle", err: stderrors.New("internal failure"), expectedFailures: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := testSettlementRepo()
			store := health.NewInMemoryStore()
			store.EnsureProvider("judge", 3, 1)
			store.BeginRequest("judge")
			service := newSettlementService(repo, nil)
			service.healthStore = store
			service.cb = NewCircuitBreaker(CircuitBreakerConfig{})
			releases := 0
			response := &llm.LLMResponse{}
			terminal := service.newFusionStreamTerminal(fusionStreamTerminalConfig{
				ctx:           context.Background(),
				req:           settlementRequest(),
				comboDecision: routing.RoutingDecision{ProviderID: "fusion-combo", Strategy: "fusion"},
				judgeDecision: routing.RoutingDecision{ProviderID: "judge", Strategy: "fusion"},
				response:      response,
				start:         time.Now(),
				release:       func() { releases++ },
			})

			terminal.complete(test.err, usage, 0)
			terminal.complete(context.Canceled, usage, 0)

			snapshot := store.Snapshot("judge")
			if releases != 1 {
				t.Fatalf("release calls=%d, want 1", releases)
			}
			if snapshot.TotalSuccesses != test.expectedSuccesses || snapshot.TotalFailures != test.expectedFailures {
				t.Fatalf("judge snapshot=%#v", snapshot)
			}
			if repo.settleCalls != test.settles {
				t.Fatalf("settlement calls=%d, want %d", repo.settleCalls, test.settles)
			}
			if test.settles == 1 && repo.settled[0].ProviderID != "fusion-combo" {
				t.Fatalf("settlement provider=%q, want fusion-combo", repo.settled[0].ProviderID)
			}
			if response.Usage == nil || response.Usage.TotalTokens != usage.TotalTokens {
				t.Fatalf("terminal usage was not retained: %#v", response.Usage)
			}
		})
	}
}
