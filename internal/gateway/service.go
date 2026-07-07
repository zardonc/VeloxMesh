package gateway

import (
	"context"
	"encoding/json"
	stdlib_errors "errors"
	"net/http"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/replication"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
	"veloxmesh/internal/scheduler"
)

type SemanticRuleResolver interface {
	GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error)
	GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error)
}

type Service struct {
	router          routing.Router
	admission       admission.Controller
	healthStore     health.Store
	fallbackEnabled bool
	maxAttempts     int
	cb              *CircuitBreaker
	repo            controlstate.Repository
	semanticCache   *cache.SemanticCacheService
	registry        *pipeline.Registry
	ruleResolver    SemanticRuleResolver
	costAggregator  hotstate.CostAggregator
	schedulerRunner *scheduler.SynchronousRunner
}

func NewService(r routing.Router, a admission.Controller, hs health.Store, fallbackEnabled bool, maxAttempts int, repo controlstate.Repository, semanticCache *cache.SemanticCacheService, registry *pipeline.Registry, ruleResolver SemanticRuleResolver, costAggregator hotstate.CostAggregator) *Service {
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
		registry:        registry,
		ruleResolver:    ruleResolver,
		costAggregator:  costAggregator,
	}
}

func (s *Service) SetSchedulerRunner(runner *scheduler.SynchronousRunner) {
	s.schedulerRunner = runner
}

func (s *Service) settle(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, usage *llm.Usage, latency time.Duration) {
	if s.repo == nil {
		return
	}

	model := req.Model
	if decision.UpstreamModel != "" {
		model = decision.UpstreamModel
	}

	record := &controlstate.UsageRecord{
		ID:         req.RequestID,
		ProviderID: decision.ProviderID,
		Model:      model,
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

	// After successful SQLite settlement, aggregate cost in Redis if available
	if s.costAggregator != nil && record.CreditsConsumed != nil {
		apiKey := "anonymous"
		if record.APIKeyID != nil {
			apiKey = *record.APIKeyID
		}
		if err := s.costAggregator.AggregateCost(context.Background(), record.ProviderID, record.Model, apiKey, *record.CreditsConsumed); err != nil {
			// Log but do not fail the request or hide the SQLite success
			// observability logger could be used here. For now just swallow to fulfill D-05
		}
	}
}

func (s *Service) HealthStore() health.Store {
	return s.healthStore
}

func (s *Service) Router() routing.Router {
	return s.router
}

func (s *Service) buildPipeline(ctx context.Context, identityScope string) *pipeline.Pipeline {
	if s.registry == nil || s.ruleResolver == nil {
		return pipeline.New(s.registry, pipeline.DefaultSemanticPipelineConfig())
	}
	global, _ := s.ruleResolver.GetGlobalDefaults(ctx)
	var user *pipeline.SemanticPipelineConfig
	if identityScope != "" && identityScope != "admin-key" && identityScope != "dev-key" {
		user, _ = s.ruleResolver.GetUserConfig(ctx, identityScope)
	}
	cfg := pipeline.ResolveSemanticRuleConfig(global, user)
	return pipeline.New(s.registry, cfg)
}

func (s *Service) HandleChatCompletion(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	ctx, rt := observability.StartRequestTrace(ctx, req.RequestID, req.Model)

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

	p := s.buildPipeline(ctx, identityScope)
	scope := pipeline.RequestScope{UserID: identityScope, RequestID: req.RequestID}
	state := &pipeline.RunState{}

	if err := p.ProcessRequest(ctx, scope, state, req); err != nil {
		if err == replication.ErrWriteNotWritable {
			return nil, errors.ErrServiceNotWritable
		}
		return nil, err
	}

	// 1. Cache Lookup
	if s.semanticCache != nil && !req.Stream && req.RouteOverride == "" && identityScope != "" && identityScope != "admin-key" {
		b, _ := json.Marshal(req.Messages)
		text := string(b)
		entry, err := s.semanticCache.Lookup(ctx, identityScope, req.Model, text)
		if err == nil && entry != nil {
			// Cache hit
			rt.RecordRouting("semantic_cache", "hit", "", "")
			rt.RecordOutcome("cache", 200, "", 0, 0, 0)

			observability.DefaultMetrics.RecordRequestOutcome(
				req.RequestID,
				"cache",
				req.Model,
				"semantic_cache",
				200,
				"",
				"hit",
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

			if err := p.ProcessResponse(ctx, scope, state, resp); err != nil {
				if err == replication.ErrWriteNotWritable {
					return nil, errors.ErrServiceNotWritable
				}
				return nil, err
			}
			return resp, nil
		}
	}

	var reqTextForStore string
	cacheResult := "none"
	if s.semanticCache != nil && !req.Stream {
		b, _ := json.Marshal(req.Messages)
		reqTextForStore = string(b)
		if identityScope != "" && identityScope != "admin-key" && req.RouteOverride == "" {
			cacheResult = "miss"
		}
	}

	for attempts < maxAllowedAttempts {

		adapter, decision, err := s.router.SelectExcluding(ctx, req, attempted)
		if err != nil {
			if err == errors.ErrCompositeScoreBelowThreshold && attempts < maxAllowedAttempts && decision.ProviderID != "" {
				attempted[decision.ProviderID] = true
				lastErr = err
				continue
			}
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
				rt.RecordOutcome(decision.ProviderID, 503, "provider_circuit_open", 0, 0, 0)
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
		upstreamReq := *req
		if decision.UpstreamModel != "" {
			upstreamReq.Model = decision.UpstreamModel
		}
		resp, err := s.runScheduledChat(ctx, &upstreamReq, func(runCtx context.Context, scheduledReq *llm.LLMRequest) (*llm.LLMResponse, error) {
			return adapter.Complete(runCtx, scheduledReq)
		})
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
		s.healthStore.RecordModelOutcome(decision.ProviderID, req.Model, healthErr == nil)

		observability.DefaultMetrics.RecordRequestOutcome(
			req.RequestID,
			decision.ProviderID,
			req.Model,
			decision.Strategy,
			status,
			errCategory,
			cacheResult,
			float64(latency.Milliseconds()),
		)

		scoreSummary := ""
		if decision.CompositeScoreSummary != nil {
			summaryBytes, _ := json.Marshal(decision.CompositeScoreSummary)
			scoreSummary = string(summaryBytes)
		}

		var fallbackReason string
		if attempts > 1 {
			fallbackReason = "provider_failure_or_rejected"
		}
		rt.RecordRouting(decision.Strategy, cacheResult, fallbackReason, scoreSummary)

		if err != nil {
			rt.RecordOutcome(decision.ProviderID, status, errCategory, float64(latency.Milliseconds()), 0, float64(latency.Milliseconds()))
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
		resp.Model = req.Model
		resp.Strategy = decision.Strategy
		resp.AttemptCount = attempts
		resp.FallbackUsed = attempts > 1
		resp.Usage = resp.Usage // Usually adapters set Usage directly on resp

		if err := p.ProcessResponse(ctx, scope, state, resp); err != nil {
			if err == replication.ErrWriteNotWritable {
				return nil, errors.ErrServiceNotWritable
			}
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

	rt.EndWithError(lastErr)
	return nil, lastErr
}

func (s *Service) GetAvailableModels() []string {
	return s.router.GetAvailableModels()
}

func (s *Service) GetProviderCapabilities() []providers.ProviderCapabilities {
	return s.router.GetProviderCapabilities()
}

func (s *Service) HandleChatCompletionStream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	ctx, rt := observability.StartRequestTrace(ctx, req.RequestID, req.Model)

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

	var identityScope string
	if identity := middleware.GetAuthIdentity(ctx); identity != nil {
		identityScope = identity.ID
	}

	p := s.buildPipeline(ctx, identityScope)
	scope := pipeline.RequestScope{UserID: identityScope, RequestID: req.RequestID}
	state := &pipeline.RunState{}

	if err := p.ProcessRequest(ctx, scope, state, req); err != nil {
		if err == replication.ErrWriteNotWritable {
			return nil, nil, errors.ErrServiceNotWritable
		}
		return nil, nil, err
	}

	for attempts < maxAllowedAttempts {

		adapter, decision, err := s.router.SelectExcluding(ctx, req, attempted)
		if err != nil {
			if err == errors.ErrCompositeScoreBelowThreshold && attempts < maxAllowedAttempts && decision.ProviderID != "" {
				attempted[decision.ProviderID] = true
				lastErr = err
				continue
			}
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
				rt.RecordOutcome(decision.ProviderID, 503, "provider_circuit_open", 0, 0, 0)
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

		upstreamReq := *req
		if decision.UpstreamModel != "" {
			upstreamReq.Model = decision.UpstreamModel
		}
		ch, queuedMeta, err := s.runScheduledStream(ctx, &upstreamReq, func(runCtx context.Context, scheduledReq *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
			events, err := streamAdapter.Stream(runCtx, scheduledReq)
			return events, nil, err
		})

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
			s.healthStore.RecordModelOutcome(decision.ProviderID, req.Model, healthErr == nil)

			observability.DefaultMetrics.RecordRequestOutcome(
				req.RequestID,
				decision.ProviderID,
				req.Model,
				decision.Strategy,
				status,
				errCategory,
				"none",
				float64(latency.Milliseconds()),
			)

			scoreSummary := ""
			if decision.CompositeScoreSummary != nil {
				summaryBytes, _ := json.Marshal(decision.CompositeScoreSummary)
				scoreSummary = string(summaryBytes)
			}
			var fallbackReason string
			if attempts > 1 {
				fallbackReason = "provider_failure_or_rejected"
			}
			rt.RecordRouting(decision.Strategy, "none", fallbackReason, scoreSummary)
			rt.RecordOutcome(decision.ProviderID, status, errCategory, float64(latency.Milliseconds()), 0, float64(latency.Milliseconds()))

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
		if queuedMeta != nil {
			respMeta.QueueWaitMs = queuedMeta.QueueWaitMs
		}

		if err := p.ProcessResponse(ctx, scope, state, respMeta); err != nil {
			if err == replication.ErrWriteNotWritable {
				return nil, nil, errors.ErrServiceNotWritable
			}
			return nil, nil, err
		}

		outCh := make(chan llm.StreamEvent)
		go func() {
			defer close(outCh)

			var streamErr error
			status := 200
			errCategory := ""
			var finalUsage *llm.Usage

			var ttft time.Duration
			firstChunk := true

			for event := range ch {
				if firstChunk {
					ttft = time.Since(start)
					firstChunk = false
				}
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
			s.healthStore.RecordModelOutcome(decision.ProviderID, req.Model, healthErr == nil)

			observability.DefaultMetrics.RecordRequestOutcome(
				req.RequestID,
				decision.ProviderID,
				req.Model,
				decision.Strategy,
				status,
				errCategory,
				"none",
				float64(latency.Milliseconds()),
			)

			scoreSummary := ""
			if decision.CompositeScoreSummary != nil {
				summaryBytes, _ := json.Marshal(decision.CompositeScoreSummary)
				scoreSummary = string(summaryBytes)
			}
			var fallbackReason string
			if attempts > 1 {
				fallbackReason = "provider_failure_or_rejected"
			}
			rt.RecordRouting(decision.Strategy, "none", fallbackReason, scoreSummary)

			var tpot float64
			tokens := 0
			if finalUsage != nil && finalUsage.CompletionTokens > 0 {
				tokens = finalUsage.CompletionTokens
			}
			if tokens > 0 {
				tpot = float64(latency-ttft) / float64(tokens) / float64(time.Millisecond)
			}

			rt.RecordOutcome(decision.ProviderID, status, errCategory, float64(ttft.Milliseconds()), tpot, float64(latency.Milliseconds()))

			release()
			observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, status)
			if status == 200 {
				observability.DefaultMetrics.RecordProviderLatency(decision.ProviderID, float64(latency.Milliseconds()))
				s.settle(ctx, req, decision, finalUsage, latency)
			}
		}()

		return outCh, respMeta, nil
	}

	rt.EndWithError(lastErr)
	return nil, nil, lastErr
}

func (s *Service) runScheduledChat(ctx context.Context, req *llm.LLMRequest, execute func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error)) (*llm.LLMResponse, error) {
	if s.schedulerRunner == nil {
		return execute(ctx, req)
	}
	resp, err := s.schedulerRunner.RunChat(ctx, req, execute)
	if err != nil {
		return nil, schedulerGatewayError(err)
	}
	return resp, nil
}

func (s *Service) runScheduledStream(ctx context.Context, req *llm.LLMRequest, execute func(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error)) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	if s.schedulerRunner == nil {
		return execute(ctx, req)
	}
	ch, resp, err := s.schedulerRunner.RunStream(ctx, req, execute)
	if err != nil {
		return nil, nil, schedulerGatewayError(err)
	}
	return ch, resp, nil
}

func schedulerGatewayError(err error) error {
	switch {
	case stdlib_errors.Is(err, scheduler.ErrQueueBackpressure):
		return errors.NewGatewayError(errors.SchedulerBackpressure, "Scheduler queue is under pressure; retry later", http.StatusTooManyRequests)
	case stdlib_errors.Is(err, scheduler.ErrQueueFull):
		return errors.NewGatewayError(errors.SchedulerQueueFull, "Scheduler queue is full; retry later", http.StatusServiceUnavailable)
	default:
		return err
	}
}
