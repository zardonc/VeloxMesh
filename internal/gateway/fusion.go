package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/controlstate/replication"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

func (s *Service) executeFusion(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision) (*llm.LLMResponse, time.Duration, error) {
	if len(decision.FusionProviders) == 0 {
		return nil, 0, errors.NewGatewayError("fusion_no_providers", "No healthy providers available for fusion", 503)
	}
	if decision.FusionJudge == "" {
		return nil, 0, errors.NewGatewayError("fusion_no_judge", "Fusion strategy requires a judge model", 400)
	}

	start := time.Now()
	validResults, promptTokens, completionTokens := s.runFusionMembers(ctx, req, decision)
	if len(validResults) == 0 {
		return nil, 0, errors.NewGatewayError("fusion_failed", "All fusion members failed to return a response", 502)
	}

	judgeReq := buildFusionJudgeRequest(req, decision.FusionJudge, validResults)
	judgeAdapter, judgeDecision, err := s.router.SelectExcluding(ctx, &judgeReq, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to route to judge model: %w", err)
	}
	judgeDecision.Strategy = "combo:fusion:judge"
	judgeDecision.UpstreamModel = judgeReq.Model

	judgeResp, err := s.executeFusionProvider(ctx, &judgeReq, judgeDecision, judgeAdapter)
	if err != nil {
		return nil, 0, fmt.Errorf("judge model failed: %w", err)
	}

	if judgeResp.Usage != nil {
		promptTokens += judgeResp.Usage.PromptTokens
		completionTokens += judgeResp.Usage.CompletionTokens
	}
	judgeResp.GatewayID = req.RequestID
	judgeResp.Model = req.Model
	judgeResp.Strategy = "combo:fusion"
	judgeResp.Provider = "fusion-ensemble"
	judgeResp.Usage = &llm.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}

	return judgeResp, time.Since(start), nil
}

func (s *Service) runFusionMembers(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision) ([]string, int, int) {
	var wg sync.WaitGroup
	results := make([]string, len(decision.FusionProviders))
	errs := make([]error, len(decision.FusionProviders))
	var promptTokens, completionTokens int
	var mu sync.Mutex

	for i, target := range decision.FusionProviders {
		wg.Add(1)
		go func(idx int, p routing.FusionProvider) {
			defer wg.Done()
			memberReq := *req
			memberReq.Stream = false
			memberReq.Model = p.Model
			memberDecision := fusionMemberDecision(decision, p)

			mResp, err := s.executeFusionProvider(ctx, &memberReq, memberDecision, p.Adapter)
			if err != nil {
				errs[idx] = err
				return
			}

			mu.Lock()
			if mResp.Usage != nil {
				promptTokens += mResp.Usage.PromptTokens
				completionTokens += mResp.Usage.CompletionTokens
			}
			if len(mResp.Choices) > 0 {
				results[idx] = mResp.Choices[0].Message.Content
			}
			mu.Unlock()
		}(i, target)
	}
	wg.Wait()

	var validResults []string
	for i, res := range results {
		if errs[i] == nil && res != "" {
			validResults = append(validResults, fmt.Sprintf("Response %d:\n%s", i+1, res))
		}
	}
	return validResults, promptTokens, completionTokens
}

func fusionMemberDecision(combo routing.RoutingDecision, p routing.FusionProvider) routing.RoutingDecision {
	providerID := p.ProviderID
	if providerID == "" && p.Adapter != nil {
		providerID = p.Adapter.ID()
	}
	return routing.RoutingDecision{
		ProviderID:    providerID,
		Strategy:      "combo:fusion:member",
		ComboID:       combo.ComboID,
		UpstreamModel: p.Model,
	}
}

func buildFusionJudgeRequest(req *llm.LLMRequest, judgeModel string, validResults []string) llm.LLMRequest {
	judgePrompt := fmt.Sprintf("Synthesize the following responses into a single, high-quality answer. If there are contradictions, resolve them logically.\n\n%s", strings.Join(validResults, "\n\n---\n\n"))
	judgeReq := *req
	var judgeMsgs []llm.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			judgeMsgs = append(judgeMsgs, m)
		}
	}
	// append the user query
	originalQuery := ""
	if len(req.Messages) > 0 {
		originalQuery = req.Messages[len(req.Messages)-1].Content
	}
	judgeMsgs = append(judgeMsgs, llm.Message{Role: "user", Content: "Original Query: " + originalQuery + "\n\n" + judgePrompt})
	judgeReq.Messages = judgeMsgs
	judgeReq.Model = judgeModel
	return judgeReq
}

func (s *Service) executeFusionProvider(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, adapter providers.ProviderAdapter) (*llm.LLMResponse, error) {
	if !s.cb.Allow(decision.ProviderID) {
		return nil, errors.NewGatewayError("provider_circuit_open", "Provider circuit is open", 503)
	}
	release, _, err := s.admission.Admit(ctx, req, decision)
	if err != nil {
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
	resp, err := s.runScheduledChat(ctx, &upstreamReq, adapter.Complete)
	s.finishFusionProvider(req, decision, release, time.Since(start), err)
	return resp, err
}

func (s *Service) finishFusionProvider(req *llm.LLMRequest, decision routing.RoutingDecision, release admission.ReleaseFunc, latency time.Duration, err error) {
	healthErr := err
	status := 200
	errCategory := ""
	if err != nil {
		status, errCategory = streamErrorStatus(err)
		if !errors.AffectsProviderHealth(err) {
			healthErr = nil
		}
	}
	s.healthStore.EndRequest(decision.ProviderID, latency, healthErr)
	s.cb.RecordResult(decision.ProviderID, healthErr == nil)
	s.healthStore.RecordModelOutcome(decision.ProviderID, req.Model, healthErr == nil)
	observability.DefaultMetrics.RecordRequestOutcome(req.RequestID, decision.ProviderID, req.Model, decision.Strategy, status, errCategory, "none", float64(latency.Milliseconds()))
	release()
	observability.DefaultMetrics.IncRequestCount(decision.ProviderID, req.Model, status)
	if status == 200 {
		observability.DefaultMetrics.RecordProviderLatency(decision.ProviderID, float64(latency.Milliseconds()))
	}
}

type fusionStreamResult struct {
	service       *Service
	ctx           context.Context
	req           *llm.LLMRequest
	comboDecision routing.RoutingDecision
	judgeDecision routing.RoutingDecision
	respMeta      *llm.LLMResponse
	events        <-chan llm.StreamEvent
	start         time.Time
	release       admission.ReleaseFunc
	trace         *observability.RequestTrace
	done          sync.Once
}

type streamFinishInput struct {
	usage       *llm.Usage
	ttft        time.Duration
	status      int
	errCategory string
	streamErr   error
}

func (s *Service) executeFusionStream(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision, rt *observability.RequestTrace) (*fusionStreamResult, error) {
	if len(decision.FusionProviders) == 0 {
		return nil, errors.NewGatewayError("fusion_no_providers", "No healthy providers available for fusion", 503)
	}
	if decision.FusionJudge == "" {
		return nil, errors.NewGatewayError("fusion_no_judge", "Fusion strategy requires a judge model", 400)
	}

	validResults, promptTokens, completionTokens := s.runFusionMembers(ctx, req, decision)
	if len(validResults) == 0 {
		return nil, errors.NewGatewayError("fusion_failed", "All fusion members failed to return a response", 502)
	}

	judgeReq := buildFusionJudgeRequest(req, decision.FusionJudge, validResults)
	judgeReq.Stream = true

	judgeAdapter, judgeDecision, err := s.router.SelectExcluding(ctx, &judgeReq, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to route to judge model: %w", err)
	}
	judgeDecision.Strategy = "combo:fusion:judge"
	judgeDecision.UpstreamModel = judgeReq.Model

	streamAdapter, ok := judgeAdapter.(providers.StreamAdapter)
	if !ok {
		return nil, fmt.Errorf("judge provider does not support streaming")
	}
	if !s.cb.Allow(judgeDecision.ProviderID) {
		return nil, errors.NewGatewayError("provider_circuit_open", "Provider circuit is open", 503)
	}
	release, queuedMeta, err := s.admission.Admit(ctx, &judgeReq, judgeDecision)
	if err != nil {
		return nil, err
	}

	observability.DefaultMetrics.RecordRoutingStrategy(judgeDecision.Strategy)
	observability.DefaultMetrics.RecordHealthStatus(judgeDecision.ProviderID, string(s.healthStore.Snapshot(judgeDecision.ProviderID).Status))
	s.healthStore.BeginRequest(judgeDecision.ProviderID)
	start := time.Now()
	streamCh, _, err := s.runScheduledStream(ctx, &judgeReq, func(runCtx context.Context, scheduledReq *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
		events, streamErr := streamAdapter.Stream(runCtx, scheduledReq)
		return events, nil, streamErr
	})
	judgeRespMeta := &llm.LLMResponse{
		GatewayID:    req.RequestID,
		Model:        req.Model,
		Provider:     "fusion-ensemble",
		Strategy:     "combo:fusion",
		AttemptCount: 1,
	}
	judgeRespMeta.QueueWaitMs = queuedMeta.QueueWaitMs

	result := &fusionStreamResult{
		service: s, ctx: ctx, req: req, comboDecision: decision, judgeDecision: judgeDecision,
		respMeta: judgeRespMeta, start: start, release: release, trace: rt,
	}
	if err != nil {
		result.finishError(err)
		return nil, fmt.Errorf("judge model failed: %w", err)
	}
	result.events = addFusionUsageToStream(streamCh, promptTokens, completionTokens)
	return result, nil
}

func addFusionUsageToStream(streamCh <-chan llm.StreamEvent, promptTokens, completionTokens int) <-chan llm.StreamEvent {
	outCh := make(chan llm.StreamEvent)
	go func() {
		defer close(outCh)
		for event := range streamCh {
			if event.Usage != nil {
				usage := *event.Usage
				usage.PromptTokens += promptTokens
				usage.CompletionTokens += completionTokens
				usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
				event.Usage = &usage
			}
			outCh <- event
		}
	}()
	return outCh
}

func (r *fusionStreamResult) forward() <-chan llm.StreamEvent {
	outCh := make(chan llm.StreamEvent)
	go func() {
		defer close(outCh)
		result := bufferedStreamResult{status: 200}
		firstChunk := true
		for event := range r.events {
			if firstChunk {
				result.ttft = time.Since(r.start)
				firstChunk = false
			}
			if event.Usage != nil {
				result.finalUsage = event.Usage
			}
			if event.Error != nil {
				result.streamErr = event.Error
				result.status, result.errCategory = streamErrorStatus(event.Error)
			}
			outCh <- event
		}
		r.finish(streamFinishInput{
			usage: result.finalUsage, ttft: result.ttft, status: result.status,
			errCategory: result.errCategory, streamErr: result.streamErr,
		})
	}()
	return outCh
}

func (s *Service) bufferFusionStreamWithResponseRules(r *fusionStreamResult, p *pipeline.Pipeline, scope pipeline.RequestScope, state *pipeline.RunState, rt *observability.RequestTrace) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	result := collectBufferedStream(r.events, r.start)
	if result.streamErr == nil && !result.hasToolCalls {
		r.respMeta.Choices = []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: result.content}}}
		r.respMeta.Usage = result.finalUsage
		if err := p.ProcessResponse(r.ctx, scope, state, r.respMeta); err != nil {
			result.status, result.errCategory = streamErrorStatus(err)
			r.finish(streamFinishInput{usage: result.finalUsage, ttft: result.ttft, status: result.status, errCategory: result.errCategory, streamErr: err})
			if err == replication.ErrWriteNotWritable {
				return nil, nil, errors.ErrServiceNotWritable
			}
			return nil, nil, err
		}
	}
	if result.hasToolCalls {
		slog.Warn("skipping streaming response rules for tool-call stream", "request_id", r.req.RequestID, "provider", r.judgeDecision.ProviderID)
	}
	r.trace = rt
	r.finish(streamFinishInput{usage: result.finalUsage, ttft: result.ttft, status: result.status, errCategory: result.errCategory, streamErr: result.streamErr})
	if result.streamErr != nil {
		return nil, nil, result.streamErr
	}
	if result.hasToolCalls {
		return replayStreamEvents(result.events), r.respMeta, nil
	}
	return singleTextStream(r.respMeta.Choices, result.finalUsage), r.respMeta, nil
}

func (r *fusionStreamResult) finishError(err error) {
	status, errCategory := streamErrorStatus(err)
	r.finish(streamFinishInput{status: status, errCategory: errCategory, streamErr: err})
}

func (r *fusionStreamResult) finish(in streamFinishInput) {
	r.done.Do(func() {
		latency := time.Since(r.start)
		healthErr := in.streamErr
		if in.streamErr != nil && !errors.AffectsProviderHealth(in.streamErr) {
			healthErr = nil
		}
		if in.status == 0 {
			in.status = 200
		}
		model := r.judgeDecision.UpstreamModel
		if model == "" {
			model = r.req.Model
		}
		r.service.healthStore.EndRequest(r.judgeDecision.ProviderID, latency, healthErr)
		r.service.cb.RecordResult(r.judgeDecision.ProviderID, healthErr == nil)
		r.service.healthStore.RecordModelOutcome(r.judgeDecision.ProviderID, model, healthErr == nil)
		observability.DefaultMetrics.RecordRequestOutcome(r.req.RequestID, r.judgeDecision.ProviderID, model, r.judgeDecision.Strategy, in.status, in.errCategory, "none", float64(latency.Milliseconds()))
		if r.trace != nil {
			tpot := 0.0
			if in.usage != nil && in.usage.CompletionTokens > 0 {
				tpot = float64(latency-in.ttft) / float64(in.usage.CompletionTokens) / float64(time.Millisecond)
			}
			r.trace.RecordRouting(r.comboDecision.Strategy, "none", "", "")
			r.trace.RecordOutcome(r.judgeDecision.ProviderID, in.status, in.errCategory, float64(in.ttft.Milliseconds()), tpot, float64(latency.Milliseconds()))
		}
		r.release()
		observability.DefaultMetrics.IncRequestCount(r.judgeDecision.ProviderID, model, in.status)
		if in.status == 200 {
			observability.DefaultMetrics.RecordProviderLatency(r.judgeDecision.ProviderID, float64(latency.Milliseconds()))
			r.service.settle(r.ctx, r.req, r.comboDecision, in.usage, latency)
		}
		r.respMeta.Usage = in.usage
	})
}
