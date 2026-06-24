package pipeline

import (
	"context"
	"veloxmesh/internal/llm"
)

// Rule represents a single step in the heuristic/LLM rules pipeline.
type Rule interface {
	ProcessRequest(ctx context.Context, req *llm.LLMRequest) error
	ProcessResponse(ctx context.Context, resp *llm.LLMResponse) error
}

// Pipeline executes a chain of rules.
type Pipeline struct {
	rules []Rule
}

func New() *Pipeline {
	return &Pipeline{
		rules: make([]Rule, 0),
	}
}

func (p *Pipeline) AddRule(r Rule) {
	p.rules = append(p.rules, r)
}

func (p *Pipeline) ProcessRequest(ctx context.Context, req *llm.LLMRequest) error {
	for _, r := range p.rules {
		if err := r.ProcessRequest(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pipeline) ProcessResponse(ctx context.Context, resp *llm.LLMResponse) error {
	for i := len(p.rules) - 1; i >= 0; i-- {
		if err := p.rules[i].ProcessResponse(ctx, resp); err != nil {
			return err
		}
	}
	return nil
}
