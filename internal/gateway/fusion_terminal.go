package gateway

import (
	"context"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/routing"
)

type fusionStreamTerminalConfig struct {
	ctx           context.Context
	req           *llm.LLMRequest
	comboDecision routing.RoutingDecision
	judgeDecision routing.RoutingDecision
	response      *llm.LLMResponse
	start         time.Time
	release       admission.ReleaseFunc
	trace         *observability.RequestTrace
}

func (s *Service) newFusionStreamTerminal(config fusionStreamTerminalConfig) *streamTerminalState {
	model := config.judgeDecision.UpstreamModel
	if model == "" {
		model = config.req.Model
	}
	return s.newStreamTerminal(streamTerminalConfig{
		ctx:                config.ctx,
		req:                config.req,
		decision:           config.judgeDecision,
		settlementDecision: config.comboDecision,
		model:              model,
		start:              config.start,
		release:            config.release,
		response:           config.response,
		trace:              config.trace,
		traceStrategy:      config.comboDecision.Strategy,
		settle:             true,
	})
}
