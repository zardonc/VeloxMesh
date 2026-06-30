package pipeline

import (
	"context"
	"errors"
	"log/slog"

	"veloxmesh/internal/llm"
)

// Pipeline executes a chain of semantic rules using a registry and resolved configuration.
type Pipeline struct {
	registry *Registry
	config   *SemanticPipelineConfig
}

func New(registry *Registry, config *SemanticPipelineConfig) *Pipeline {
	if config == nil {
		config = DefaultSemanticPipelineConfig()
	}
	return &Pipeline{
		registry: registry,
		config:   config,
	}
}

// Request processing order per D-05 + D-10: Filter(pre), PII, Rewrite, RTK, Headroom, Caveman, Ponytail
var requestOrder = []RuleName{
	RuleFilter,
	RulePII,
	RuleRewrite,
	RuleRTK,
	RuleHeadroom,
	RuleCaveman,
	RulePonytail,
}

// Response processing order per D-06: Caveman or Ponytail, Filter(post), PII Restore
var responseOrder = []RuleName{
	RuleCaveman,
	RulePonytail,
	RuleFilter,
	RulePII, // PII Restore
}

func (p *Pipeline) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest) error {
	for _, ruleName := range requestOrder {
		ruleCfg := p.config.Rules[ruleName]
		if !ruleCfg.Enabled {
			continue
		}

		handler := p.registry.Get(ruleName)
		if handler == nil {
			continue // No handler registered for this rule
		}

		err := handler.ProcessRequest(ctx, scope, state, req, ruleCfg)
		if err != nil {
			if errors.Is(err, ErrFilterBlock) {
				return err // Intentional block decision
			}
			// Safe failure per D-13, D-14
			slog.Error("semantic handler failed",
				"rule", string(ruleName),
				"user_id", scope.UserID,
				"request_id", scope.RequestID,
				"error", err.Error(),
			)
			// Continue executing the next handlers
		}
	}
	return nil
}

func (p *Pipeline) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse) error {
	for _, ruleName := range responseOrder {
		ruleCfg := p.config.Rules[ruleName]
		if !ruleCfg.Enabled {
			continue
		}

		handler := p.registry.Get(ruleName)
		if handler == nil {
			continue
		}

		err := handler.ProcessResponse(ctx, scope, state, resp, ruleCfg)
		if err != nil {
			if errors.Is(err, ErrFilterBlock) {
				return err
			}
			slog.Error("semantic handler failed",
				"rule", string(ruleName),
				"user_id", scope.UserID,
				"request_id", scope.RequestID,
				"error", err.Error(),
			)
			// Continue
		}
	}
	return nil
}
