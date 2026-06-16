package gateway

import (
	"context"
	"time"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/routing"
)

type Service struct {
	router      routing.Router
	admission   admission.Controller
	healthStore health.Store
}

func NewService(r routing.Router, a admission.Controller, hs health.Store) *Service {
	return &Service{
		router:      r,
		admission:   a,
		healthStore: hs,
	}
}

func (s *Service) HealthStore() health.Store {
	return s.healthStore
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

	observability.DefaultMetrics.RecordRoutingStrategy(decision.Strategy)
	observability.DefaultMetrics.RecordHealthStatus(decision.ProviderID, string(s.healthStore.Snapshot(decision.ProviderID).Status))

	s.healthStore.BeginRequest(decision.ProviderID)

	start := time.Now()
	resp, err := adapter.Complete(ctx, req)
	latency := time.Since(start)

	healthErr := err
	errCategory := ""
	status := 200
	if err != nil {
		if gwErr, ok := err.(*errors.GatewayError); ok {
			errCategory = gwErr.Code
			status = gwErr.HTTPStatus
		} else {
			errCategory = "provider_error"
			status = 502
		}
		if !errors.AffectsProviderHealth(err) {
			healthErr = nil
		}
	}

	// We don't need defer here since we call it immediately before returning
	s.healthStore.EndRequest(decision.ProviderID, latency, healthErr)

	observability.DefaultMetrics.RecordRequestOutcome(
		req.RequestID,
		decision.ProviderID,
		req.Model,
		decision.Strategy,
		status,
		errCategory,
		float64(latency.Milliseconds()),
	)

	if err != nil {
		observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, status)
		return nil, err
	}
	resp.Provider = decision.ProviderID
	resp.Strategy = decision.Strategy

	// Record success
	observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, 200)
	observability.DefaultMetrics.RecordProviderLatency(decision.ProviderID, float64(latency.Milliseconds()))

	return resp, nil
}

func (s *Service) GetAvailableModels() []string {
	return s.router.GetAvailableModels()
}
