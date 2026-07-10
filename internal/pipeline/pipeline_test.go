package pipeline

import (
	"context"
	"errors"
	"testing"
	"veloxmesh/internal/llm"
	verrors "veloxmesh/internal/errors"
)

type mockOrderHandler struct {
	name       RuleName
	orderLog   *[]RuleName
	errToThrow error
}

func (m *mockOrderHandler) Name() RuleName {
	return m.name
}

func (m *mockOrderHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	*m.orderLog = append(*m.orderLog, m.name)
	return m.errToThrow
}

func (m *mockOrderHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	*m.orderLog = append(*m.orderLog, m.name)
	return m.errToThrow
}

func TestPipelineExecutionOrder(t *testing.T) {
	registry := NewRegistry()
	var orderLog []RuleName

	handlers := []RuleName{
		RuleFilter, RulePII, RuleRewrite, RuleRTK, RuleHeadroom, RuleCaveman, RulePonytail,
	}

	for _, h := range handlers {
		registry.Register(&mockOrderHandler{
			name:     h,
			orderLog: &orderLog,
		})
	}

	cfg := DefaultSemanticPipelineConfig()
	for _, h := range handlers {
		cfg.Rules[h] = RuleConfig{Enabled: true}
	}

	// Ponytail vs Caveman mutual exclusivity means they can't both be enabled.
	// We'll test with Caveman enabled.
	cfg.Rules[RulePonytail] = RuleConfig{Enabled: false}

	p := New(registry, cfg)
	
	ctx := context.Background()
	scope := RequestScope{UserID: "u1", RequestID: "r1"}
	state := &RunState{PIIMappings: make(map[string]string)}

	err := p.ProcessRequest(ctx, scope, state, &llm.LLMRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedReqOrder := []RuleName{RuleFilter, RulePII, RuleRewrite, RuleRTK, RuleHeadroom, RuleCaveman}
	if len(orderLog) != len(expectedReqOrder) {
		t.Fatalf("expected length %d, got %d", len(expectedReqOrder), len(orderLog))
	}
	for i, name := range expectedReqOrder {
		if orderLog[i] != name {
			t.Errorf("expected %s at position %d, got %s", name, i, orderLog[i])
		}
	}

	orderLog = nil // reset

	err = p.ProcessResponse(ctx, scope, state, &llm.LLMResponse{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedRespOrder := []RuleName{RuleCaveman, RuleFilter, RulePII}
	if len(orderLog) != len(expectedRespOrder) {
		t.Fatalf("expected length %d, got %d", len(expectedRespOrder), len(orderLog))
	}
	for i, name := range expectedRespOrder {
		if orderLog[i] != name {
			t.Errorf("expected %s at position %d, got %s", name, i, orderLog[i])
		}
	}
}

func TestPipelineErrorHandling(t *testing.T) {
	registry := NewRegistry()
	var orderLog []RuleName

	registry.Register(&mockOrderHandler{
		name:       RuleFilter,
		orderLog:   &orderLog,
		errToThrow: errors.New("some regular error"),
	})
	registry.Register(&mockOrderHandler{
		name:     RulePII,
		orderLog: &orderLog,
	})

	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RuleFilter] = RuleConfig{Enabled: true}
	cfg.Rules[RulePII] = RuleConfig{Enabled: true}

	p := New(registry, cfg)
	ctx := context.Background()
	scope := RequestScope{UserID: "u1", RequestID: "r1"}
	state := &RunState{PIIMappings: make(map[string]string)}

	err := p.ProcessRequest(ctx, scope, state, &llm.LLMRequest{})
	if err != nil {
		t.Fatalf("expected no error (handler errors should be skipped), got %v", err)
	}

	if len(orderLog) != 2 || orderLog[0] != RuleFilter || orderLog[1] != RulePII {
		t.Errorf("expected both filter and pii to run despite filter error, got %v", orderLog)
	}
}

func TestPipelineFilterBlock(t *testing.T) {
	registry := NewRegistry()
	var orderLog []RuleName

	registry.Register(&mockOrderHandler{
		name:       RuleFilter,
		orderLog:   &orderLog,
		errToThrow: verrors.ErrPolicyBlocked,
	})
	registry.Register(&mockOrderHandler{
		name:     RulePII,
		orderLog: &orderLog,
	})

	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RuleFilter] = RuleConfig{Enabled: true}
	cfg.Rules[RulePII] = RuleConfig{Enabled: true}

	p := New(registry, cfg)
	ctx := context.Background()
	scope := RequestScope{UserID: "u1", RequestID: "r1"}
	state := &RunState{PIIMappings: make(map[string]string)}

	err := p.ProcessRequest(ctx, scope, state, &llm.LLMRequest{})
	if err == nil || !errors.Is(err, verrors.ErrPolicyBlocked) {
		t.Fatalf("expected ErrPolicyBlocked, got %v", err)
	}

	if len(orderLog) != 1 || orderLog[0] != RuleFilter {
		t.Errorf("expected execution to stop at filter, got %v", orderLog)
	}
}
