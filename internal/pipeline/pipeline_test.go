package pipeline

import (
	"context"
	"testing"
	"veloxmesh/internal/llm"
)

type mockRule struct {
	reqCalled  bool
	respCalled bool
}

func (m *mockRule) ProcessRequest(ctx context.Context, req *llm.LLMRequest) error {
	m.reqCalled = true
	return nil
}

func (m *mockRule) ProcessResponse(ctx context.Context, resp *llm.LLMResponse) error {
	m.respCalled = true
	return nil
}

func TestPipelineExecution(t *testing.T) {
	p := New()
	rule1 := &mockRule{}
	rule2 := &mockRule{}

	p.AddRule(rule1)
	p.AddRule(rule2)

	ctx := context.Background()
	req := &llm.LLMRequest{}
	resp := &llm.LLMResponse{}

	if err := p.ProcessRequest(ctx, req); err != nil {
		t.Fatalf("ProcessRequest failed: %v", err)
	}

	if !rule1.reqCalled || !rule2.reqCalled {
		t.Errorf("Expected both rules to process request")
	}

	if err := p.ProcessResponse(ctx, resp); err != nil {
		t.Fatalf("ProcessResponse failed: %v", err)
	}

	if !rule1.respCalled || !rule2.respCalled {
		t.Errorf("Expected both rules to process response")
	}
}
