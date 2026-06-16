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
	router          routing.Router
	admission       admission.Controller
	healthStore     health.Store
	fallbackEnabled bool
	maxAttempts     int
}

func NewService(r routing.Router, a admission.Controller, hs health.Store, fallbackEnabled bool, maxAttempts int) *Service {
	return &Service{
		router:          r,
		admission:       a,
		healthStore:     hs,
		fallbackEnabled: fallbackEnabled,
		maxAttempts:     maxAttempts,
	}
}

func (s *Service) HealthStore() health.Store {
	return s.healthStore
}

func (s *Service) HandleChatCompletion(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	attempted := make(map[string]bool)
	attempts := 0
	var lastErr error

	maxAllowedAttempts := 1
	if s.fallbackEnabled && req.RouteOverride == "" && !req.Stream {
		maxAllowedAttempts = s.maxAttempts
	}

	for attempts < maxAllowedAttempts {
		attempts++

		adapter, decision, err := s.router.SelectExcluding(ctx, req, attempted)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr // Return the last provider error rather than no_healthy_provider
			}
			return nil, err
		}

		release, _, err := s.admission.Admit(ctx, req, decision)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}

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

		// We still need to record the attempt metric if needed, but that's in Wave 2.
		// For now we just use existing RequestOutcome which records it.

		if err != nil {
			release() // Release admission quickly
			observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, status)
			lastErr = err

			// Check context cancel
			if ctx.Err() != nil {
				return nil, err
			}

			if errors.IsRetryableProviderError(err) {
				attempted[decision.ProviderID] = true
				continue
			}
			return nil, err
		}

		release() // Release admission quickly
		resp.Provider = decision.ProviderID
		resp.Strategy = decision.Strategy
		resp.AttemptCount = attempts
		resp.FallbackUsed = attempts > 1

		// Record success
		observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, 200)
		observability.DefaultMetrics.RecordProviderLatency(decision.ProviderID, float64(latency.Milliseconds()))

		return resp, nil
	}

	return nil, lastErr
}

func (s *Service) GetAvailableModels() []string {
	return s.router.GetAvailableModels()
}
