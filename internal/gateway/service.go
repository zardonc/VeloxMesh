package gateway

import (
	"context"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/routing"
)

type Service struct {
	router    routing.Router
	admission admission.Controller
}

func NewService(r routing.Router, a admission.Controller) *Service {
	return &Service{
		router:    r,
		admission: a,
	}
}

func (s *Service) HandleChatCompletion(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	adapter, decision, err := s.router.Select(ctx, req)
	if err != nil {
		return nil, err
	}

	release, _, err := s.admission.Admit(ctx, req, decision)
	if err != nil {
		return nil, err
	}
	defer release()

	resp, err := adapter.Complete(ctx, req)
	if err != nil {
		observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, 502)
		return nil, err
	}
	resp.Provider = decision.ProviderID

	// Record success
	observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, 200)
	// We don't have the exact duration here, but the handler measures E2E latency.
	// For now, we can leave latency to the handler or do it here.

	return resp, nil
}
