package gateway

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/controlstate/replication"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

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
			result, err := s.executeFusionStream(ctx, req, decision, rt)
			if err != nil {
				return nil, nil, err
			}
			if p.HasResponseRulesEnabled() {
				return s.bufferFusionStreamWithResponseRules(result, p, scope, state)
			}
			if err := p.ProcessResponse(ctx, scope, state, result.respMeta); err != nil {
				result.finishError(err)
				if err == replication.ErrWriteNotWritable {
					return nil, nil, errors.ErrServiceNotWritable
				}
				return nil, nil, err
			}
			return result.forward(), result.respMeta, nil
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
		terminal := s.newStreamTerminal(streamTerminalConfig{
			ctx: ctx, req: req, decision: decision, settlementDecision: decision,
			model: req.Model, start: start, release: release, attempts: attempts,
			trace: rt, traceStrategy: decision.Strategy, traceScoreSummary: streamScoreSummary(decision), settle: true,
		})

		upstreamReq := *req
		if decision.UpstreamModel != "" {
			upstreamReq.Model = decision.UpstreamModel
		}
		ch, queuedMeta, err := s.runScheduledStream(ctx, &upstreamReq, func(runCtx context.Context, scheduledReq *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
			events, err := streamAdapter.Stream(runCtx, scheduledReq)
			return events, nil, err
		})

		if err != nil {
			terminal.complete(err, nil, 0)
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
		terminal.setResponse(respMeta)

		if p.HasResponseRulesEnabled() {
			return s.bufferStreamWithResponseRules(streamRuleContext{
				ctx: ctx, req: req, decision: decision, respMeta: respMeta, pipeline: p,
				scope: scope, state: state, events: ch, start: start, release: release,
				attempts: attempts, trace: rt, terminal: terminal,
			})
		}

		if err := p.ProcessResponse(ctx, scope, state, respMeta); err != nil {
			go drainStream(ch)
			s.finishStreamRequest(streamFinish{
				streamRuleContext: streamRuleContext{
					ctx: ctx, req: req, decision: decision, respMeta: respMeta, pipeline: p,
					scope: scope, state: state, events: ch, start: start, release: release,
					attempts: attempts, trace: rt, terminal: terminal,
				},
				streamErr: err,
			})
			if err == replication.ErrWriteNotWritable {
				return nil, nil, errors.ErrServiceNotWritable
			}
			return nil, nil, err
		}

		outCh := make(chan llm.StreamEvent)
		go func() {
			defer close(outCh)

			var streamErr error
			var finalUsage *llm.Usage

			var ttft time.Duration
			firstChunk := true

			for event := range ch {
				if firstChunk {
					ttft = time.Since(start)
					firstChunk = false
				}
				if event.Usage != nil {
					finalUsage = event.Usage
				}
				if event.Error != nil {
					streamErr = event.Error
					terminal.complete(streamErr, finalUsage, ttft)
				}
				if event.Done {
					terminal.complete(nil, finalUsage, ttft)
				}
				if !forwardStreamEvent(ctx, outCh, event) {
					streamErr = context.Canceled
					break
				}
			}
			if streamErr == nil && ctx.Err() != nil {
				streamErr = ctx.Err()
			}

			terminal.complete(streamErr, finalUsage, ttft)
		}()

		return outCh, respMeta, nil
	}

	rt.EndWithError(lastErr)
	return nil, nil, lastErr
}

type streamRuleContext struct {
	ctx      context.Context
	req      *llm.LLMRequest
	decision routing.RoutingDecision
	respMeta *llm.LLMResponse
	pipeline *pipeline.Pipeline
	scope    pipeline.RequestScope
	state    *pipeline.RunState
	events   <-chan llm.StreamEvent
	start    time.Time
	release  admission.ReleaseFunc
	attempts int
	trace    *observability.RequestTrace
	terminal *streamTerminalState
}

type bufferedStreamResult struct {
	content      string
	events       []llm.StreamEvent
	streamErr    error
	finalUsage   *llm.Usage
	ttft         time.Duration
	status       int
	errCategory  string
	hasToolCalls bool
}

func (s *Service) bufferStreamWithResponseRules(in streamRuleContext) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	result := collectBufferedStream(in.events, in.start)
	if result.streamErr == nil && in.ctx.Err() != nil {
		result.streamErr = in.ctx.Err()
	}
	if result.streamErr == nil && !result.hasToolCalls {
		in.respMeta.Choices = []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: result.content}}}
		in.respMeta.Usage = result.finalUsage
		if err := in.pipeline.ProcessResponse(in.ctx, in.scope, in.state, in.respMeta); err != nil {
			s.finishStreamRequest(streamFinish{streamRuleContext: in, usage: result.finalUsage, ttft: result.ttft, streamErr: err})
			if err == replication.ErrWriteNotWritable {
				return nil, nil, errors.ErrServiceNotWritable
			}
			return nil, nil, err
		}
	}
	if result.hasToolCalls {
		slog.Warn("skipping streaming response rules for tool-call stream", "request_id", in.req.RequestID, "provider", in.decision.ProviderID)
	}
	s.finishStreamRequest(streamFinish{streamRuleContext: in, usage: result.finalUsage, ttft: result.ttft, status: result.status, errCategory: result.errCategory, streamErr: result.streamErr})
	if result.streamErr != nil {
		return nil, nil, result.streamErr
	}
	if result.hasToolCalls {
		return replayStreamEvents(in.ctx, result.events), in.respMeta, nil
	}
	return singleTextStream(in.respMeta.Choices, result.finalUsage), in.respMeta, nil
}

func collectBufferedStream(ch <-chan llm.StreamEvent, start time.Time) bufferedStreamResult {
	var content strings.Builder
	result := bufferedStreamResult{status: 200}
	firstChunk := true
	terminalSeen := false

	for event := range ch {
		if firstChunk {
			result.ttft = time.Since(start)
			firstChunk = false
		}
		if len(event.ToolCalls) > 0 {
			result.hasToolCalls = true
		}
		if event.Usage != nil {
			result.finalUsage = event.Usage
		}
		if event.Error != nil && !terminalSeen {
			result.streamErr = event.Error
			result.status, result.errCategory = streamErrorStatus(event.Error)
			terminalSeen = true
		}
		if event.Done && !terminalSeen {
			terminalSeen = true
		}
		if !event.Done {
			content.WriteString(event.DeltaContent)
		}
		result.events = append(result.events, event)
	}
	result.content = content.String()
	return result
}

func streamErrorStatus(err error) (int, string) {
	if gwErr, ok := err.(*errors.GatewayError); ok {
		return gwErr.HTTPStatus, gwErr.Code
	}
	return 502, "provider_error"
}

type streamFinish struct {
	streamRuleContext
	usage       *llm.Usage
	ttft        time.Duration
	status      int
	errCategory string
	streamErr   error
}

func (s *Service) finishStreamRequest(f streamFinish) {
	if f.terminal != nil {
		f.terminal.complete(f.streamErr, f.usage, f.ttft)
	}
}

func replayStreamEvents(ctx context.Context, events []llm.StreamEvent) <-chan llm.StreamEvent {
	out := make(chan llm.StreamEvent)
	go func() {
		defer close(out)
		for _, event := range events {
			if !forwardStreamEvent(ctx, out, event) {
				return
			}
		}
	}()
	return out
}

func forwardStreamEvent(ctx context.Context, out chan<- llm.StreamEvent, event llm.StreamEvent) bool {
	select {
	case out <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func drainStream(ch <-chan llm.StreamEvent) {
	for range ch {
	}
}

func singleTextStream(choices []llm.Choice, usage *llm.Usage) <-chan llm.StreamEvent {
	out := make(chan llm.StreamEvent, 2)
	if len(choices) > 0 && choices[0].Message.Content != "" {
		out <- llm.StreamEvent{DeltaContent: choices[0].Message.Content, Usage: usage}
	}
	out <- llm.StreamEvent{Done: true}
	close(out)
	return out
}
