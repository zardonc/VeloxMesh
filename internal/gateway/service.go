package gateway

import (
	"context"
	"encoding/json"
	"time"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type Service struct {
	router          routing.Router
	admission       admission.Controller
	healthStore     health.Store
	fallbackEnabled bool
	maxAttempts     int
	cb              *CircuitBreaker
	repo            controlstate.Repository
	semanticCache   *cache.SemanticCacheService
	pipeline        *pipeline.Pipeline
}

func NewService(r routing.Router, a admission.Controller, hs health.Store, fallbackEnabled bool, maxAttempts int, repo controlstate.Repository, semanticCache *cache.SemanticCacheService) *Service {
	// Initialize breaker with some sane defaults, can be overridden or tied to snapshot later
	breakerCfg := CircuitBreakerConfig{
		FailureThreshold: 5,
		RecoveryTimeout:  30 * time.Second,
	}
	return &Service{
		router:          r,
		admission:       a,
		healthStore:     hs,
		fallbackEnabled: fallbackEnabled,
		maxAttempts:     maxAttempts,
		cb:              NewCircuitBreaker(breakerCfg),
		repo:            repo,
		semanticCache:   semanticCache,
		pipeline:        pipeline.New(),
	}
}

func (s *Service) settle(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, usage *llm.Usage, latency time.Duration) {
	if s.repo == nil {
		return
	}

	record := &controlstate.UsageRecord{
		ID:         req.RequestID,
		ProviderID: decision.ProviderID,
		Model:      req.Model,
		DurationMs: latency.Milliseconds(),
		Timestamp:  time.Now().UTC(),
	}

	if usage != nil {
		record.PromptTokens = usage.PromptTokens
		record.ResponseTokens = usage.CompletionTokens
		record.TotalTokens = usage.TotalTokens
	} else {
		record.Status = controlstate.SettlementStatusMissingUsage
	}

	if identity := middleware.GetAuthIdentity(ctx); identity != nil && identity.ID != "dev-key" && identity.ID != "admin-key" {
		record.APIKeyID = &identity.ID
	}

	if record.Status == controlstate.SettlementStatusMissingUsage {
		_ = s.repo.Usage().Log(context.Background(), record)
		return
	}

	_ = s.repo.Settle(context.Background(), record)
}

func (s *Service) HealthStore() health.Store {
	return s.healthStore
}

func (s *Service) Router() routing.Router {
	return s.router
}

func (s *Service) HandleChatCompletion(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	attempted := make(map[string]bool)
	attempts := 0
	var lastErr error

	maxAllowedAttempts := 1

	enabled, attemptsLimit := s.fallbackEnabled, s.maxAttempts
	type FallbackProvider interface {
		FallbackConfig() (bool, int)
		CircuitBreakerConfig() (int, time.Duration)
	}
	if fp, ok := s.router.(FallbackProvider); ok {
		enabled, attemptsLimit = fp.FallbackConfig()
		threshold, recovery := fp.CircuitBreakerConfig()
		s.cb.UpdateConfig(CircuitBreakerConfig{
			FailureThreshold: threshold,
			RecoveryTimeout:  recovery,
		})
	}

	if enabled && req.RouteOverride == "" && !req.Stream {
		maxAllowedAttempts = attemptsLimit
	}

	var identityScope string
	if identity := middleware.GetAuthIdentity(ctx); identity != nil {
		identityScope = identity.ID
	}

	// 1. Cache Lookup
	if s.semanticCache != nil && !req.Stream && req.RouteOverride == "" && identityScope != "" && identityScope != "admin-key" {
		b, _ := json.Marshal(req.Messages)
		text := string(b)
		entry, err := s.semanticCache.Lookup(ctx, identityScope, req.Model, text)
		if err == nil && entry != nil {
			// Cache hit
			observability.DefaultMetrics.RecordRequestOutcome(
				req.RequestID,
				"cache",
				req.Model,
				"semantic_cache",
				200,
				"",
				0, // latency negligible
			)

			// create a minimal LLMResponse from cached response
			var choices []llm.Choice
			_ = json.Unmarshal([]byte(entry.Response), &choices)

			resp := &llm.LLMResponse{
				GatewayID:    req.RequestID,
				Model:        req.Model,
				Provider:     "cache",
				Strategy:     "semantic_cache",
				AttemptCount: 1,
				FallbackUsed: false,
				Choices:      choices,
				Usage: &llm.Usage{
					PromptTokens:     0,
					CompletionTokens: 0,
					TotalTokens:      0,
				},
				CacheHit:   true,
				CacheLevel: "semantic",
			}
			return resp, nil
		}
	}

	var reqTextForStore string
	if s.semanticCache != nil && !req.Stream {
		b, _ := json.Marshal(req.Messages)
		reqTextForStore = string(b)
	}

	for attempts < maxAllowedAttempts {
		if err := s.pipeline.ProcessRequest(ctx, req); err != nil {
			return nil, err
		}

		adapter, decision, err := s.router.SelectExcluding(ctx, req, attempted)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr // Return the last provider error rather than no_healthy_provider
			}
			return nil, err
		}

		if decision.IsFusion {
			release, _, err := s.admission.Admit(ctx, req, decision)
			if err != nil {
				return nil, err
			}
			resp, err := s.executeFusion(ctx, req, decision)
			release()
			if err != nil {
				return nil, err
			}
			return resp, nil
		}

		if !s.cb.Allow(decision.ProviderID) {
			attempted[decision.ProviderID] = true
			if req.RouteOverride != "" {
				return nil, errors.NewGatewayError("provider_circuit_open", "Provider circuit is open", 503)
			}
			continue
		}

		attempts++

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
		s.cb.RecordResult(decision.ProviderID, healthErr == nil)

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
		resp.Usage = resp.Usage // Usually adapters set Usage directly on resp

		if err := s.pipeline.ProcessResponse(ctx, resp); err != nil {
			return nil, err
		}

		// Record success
		observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, 200)
		observability.DefaultMetrics.RecordProviderLatency(decision.ProviderID, float64(latency.Milliseconds()))

		s.settle(ctx, req, decision, resp.Usage, latency)

		// Cache Store
		if s.semanticCache != nil && !req.Stream && req.RouteOverride == "" && identityScope != "" && identityScope != "admin-key" {
			// Only cache if there's a valid choice
			if len(resp.Choices) > 0 {
				bResp, _ := json.Marshal(resp.Choices)
				usageID := req.RequestID // from settle
				_ = s.semanticCache.Store(ctx, req.RequestID, identityScope, req.Model, reqTextForStore, string(bResp), &usageID)
			}
		}

		return resp, nil
	}

	return nil, lastErr
}

func (s *Service) GetAvailableModels() []string {
	return s.router.GetAvailableModels()
}

func (s *Service) GetProviderCapabilities() []providers.ProviderCapabilities {
	return s.router.GetProviderCapabilities()
}

func (s *Service) HandleChatCompletionStream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	attempted := make(map[string]bool)
	attempts := 0
	var lastErr error

	maxAllowedAttempts := 1

	enabled, attemptsLimit := s.fallbackEnabled, s.maxAttempts
	type FallbackProvider interface {
		FallbackConfig() (bool, int)
		CircuitBreakerConfig() (int, time.Duration)
	}
	if fp, ok := s.router.(FallbackProvider); ok {
		enabled, attemptsLimit = fp.FallbackConfig()
		threshold, recovery := fp.CircuitBreakerConfig()
		s.cb.UpdateConfig(CircuitBreakerConfig{
			FailureThreshold: threshold,
			RecoveryTimeout:  recovery,
		})
	}

	if enabled && req.RouteOverride == "" {
		maxAllowedAttempts = attemptsLimit
	}

	for attempts < maxAllowedAttempts {
		if err := s.pipeline.ProcessRequest(ctx, req); err != nil {
			return nil, nil, err
		}

		adapter, decision, err := s.router.SelectExcluding(ctx, req, attempted)
		if err != nil {
			if lastErr != nil {
				return nil, nil, lastErr
			}
			return nil, nil, err
		}

		if decision.IsFusion {
			release, _, err := s.admission.Admit(ctx, req, decision)
			if err != nil {
				return nil, nil, err
			}
			streamCh, respMeta, err := s.executeFusionStream(ctx, req, decision)
			release()
			if err != nil {
				return nil, nil, err
			}
			return streamCh, respMeta, nil
		}

		streamAdapter, ok := adapter.(providers.StreamAdapter)
		if !ok {
			attempted[decision.ProviderID] = true
			lastErr = errors.NewGatewayError("provider_invalid_request", "Provider does not support streaming", 400)
			continue
		}

		if !s.cb.Allow(decision.ProviderID) {
			attempted[decision.ProviderID] = true
			if req.RouteOverride != "" {
				return nil, nil, errors.NewGatewayError("provider_circuit_open", "Provider circuit is open", 503)
			}
			continue
		}

		attempts++

		release, _, err := s.admission.Admit(ctx, req, decision)
		if err != nil {
			if lastErr != nil {
				return nil, nil, lastErr
			}
			return nil, nil, err
		}

		observability.DefaultMetrics.RecordRoutingStrategy(decision.Strategy)
		observability.DefaultMetrics.RecordHealthStatus(decision.ProviderID, string(s.healthStore.Snapshot(decision.ProviderID).Status))

		s.healthStore.BeginRequest(decision.ProviderID)
		start := time.Now()

		ch, err := streamAdapter.Stream(ctx, req)

		if err != nil {
			latency := time.Since(start)
			healthErr := err
			errCategory := ""
			status := 200
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

			s.healthStore.EndRequest(decision.ProviderID, latency, healthErr)
			s.cb.RecordResult(decision.ProviderID, healthErr == nil)

			observability.DefaultMetrics.RecordRequestOutcome(
				req.RequestID,
				decision.ProviderID,
				req.Model,
				decision.Strategy,
				status,
				errCategory,
				float64(latency.Milliseconds()),
			)

			release()
			observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, status)
			lastErr = err

			if ctx.Err() != nil {
				return nil, nil, err
			}

			if errors.IsRetryableProviderError(err) {
				attempted[decision.ProviderID] = true
				continue
			}
			return nil, nil, err
		}

		respMeta := &llm.LLMResponse{
			GatewayID:    req.RequestID,
			Model:        req.Model,
			Provider:     decision.ProviderID,
			Strategy:     decision.Strategy,
			AttemptCount: attempts,
			FallbackUsed: attempts > 1,
		}

		if err := s.pipeline.ProcessResponse(ctx, respMeta); err != nil {
			return nil, nil, err
		}

		outCh := make(chan llm.StreamEvent)
		go func() {
			defer close(outCh)

			var streamErr error
			status := 200
			errCategory := ""
			var finalUsage *llm.Usage

			for event := range ch {
				if event.Usage != nil && finalUsage == nil {
					finalUsage = event.Usage
				}
				if event.Error != nil {
					streamErr = event.Error
					if gwErr, ok := streamErr.(*errors.GatewayError); ok {
						errCategory = gwErr.Code
						status = gwErr.HTTPStatus
					} else {
						errCategory = "provider_error"
						status = 502
					}
				}
				outCh <- event
			}

			latency := time.Since(start)
			healthErr := streamErr
			if streamErr != nil && !errors.AffectsProviderHealth(streamErr) {
				healthErr = nil
			}

			s.healthStore.EndRequest(decision.ProviderID, latency, healthErr)
			s.cb.RecordResult(decision.ProviderID, healthErr == nil)

			observability.DefaultMetrics.RecordRequestOutcome(
				req.RequestID,
				decision.ProviderID,
				req.Model,
				decision.Strategy,
				status,
				errCategory,
				float64(latency.Milliseconds()),
			)

			release()
			observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, status)
			if status == 200 {
				observability.DefaultMetrics.RecordProviderLatency(decision.ProviderID, float64(latency.Milliseconds()))
				s.settle(ctx, req, decision, finalUsage, latency)
			}
		}()

		return outCh, respMeta, nil
	}

	return nil, nil, lastErr
}
