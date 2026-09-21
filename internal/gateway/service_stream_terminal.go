package gateway

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/routing"
)

type streamTerminalConfig struct {
	ctx                context.Context
	req                *llm.LLMRequest
	decision           routing.RoutingDecision
	settlementDecision routing.RoutingDecision
	model              string
	start              time.Time
	release            admission.ReleaseFunc
	response           *llm.LLMResponse
	attempts           int
	trace              *observability.RequestTrace
	traceStrategy      string
	traceScoreSummary  string
	settle             bool
}

type streamTerminalState struct {
	mu        sync.Mutex
	service   *Service
	config    streamTerminalConfig
	finalizer *terminalFinalizer
	ttft      time.Duration
}

func (s *Service) newStreamTerminal(config streamTerminalConfig) *streamTerminalState {
	state := &streamTerminalState{service: s, config: config}
	state.finalizer = newTerminalFinalizer(terminalFinalizerOptions{
		providerHealth:         state.recordHealth,
		circuitBreaker:         state.recordBreaker,
		metrics:                state.recordMetrics,
		traceClose:             state.recordTrace,
		settlementDecision:     state.recordSettlement,
		admissionRelease:       state.releaseAdmission,
		clientTerminalEmission: state.recordClientResult,
	})
	return state
}

func (t *streamTerminalState) complete(err error, usage *llm.Usage, ttft time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ttft = ttft
	return t.finalizer.Submit(classifyTerminal(err, usage))
}

func (t *streamTerminalState) setResponse(response *llm.LLMResponse) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.config.response = response
}

func (t *streamTerminalState) recordHealth(outcome terminalOutcome) {
	err := terminalHealthError(outcome)
	t.service.healthStore.EndRequest(t.config.decision.ProviderID, t.latency(), err)
	t.service.healthStore.RecordModelOutcome(t.config.decision.ProviderID, t.config.model, err == nil)
}

func (t *streamTerminalState) recordBreaker(outcome terminalOutcome) {
	t.service.cb.RecordResult(t.config.decision.ProviderID, terminalHealthError(outcome) == nil)
}

func (t *streamTerminalState) recordMetrics(outcome terminalOutcome) {
	status, category := terminalStatus(outcome)
	latency := t.latency()
	observability.DefaultMetrics.RecordRequestOutcome(t.config.req.RequestID, t.config.decision.ProviderID, t.config.model, t.config.decision.Strategy, status, category, "none", float64(latency.Milliseconds()))
	observability.DefaultMetrics.IncRequestCount(t.config.decision.ProviderID, t.config.model, status)
	if outcome.kind == terminalCompleted {
		observability.DefaultMetrics.RecordProviderLatency(t.config.decision.ProviderID, float64(latency.Milliseconds()))
	}
}

func (t *streamTerminalState) recordTrace(outcome terminalOutcome) {
	if t.config.trace == nil {
		return
	}
	status, category := terminalStatus(outcome)
	latency := t.latency()
	t.config.trace.RecordRouting(t.config.traceStrategy, "none", streamFallbackReason(t.config.attempts), t.config.traceScoreSummary)
	t.config.trace.RecordOutcome(t.config.decision.ProviderID, status, category, float64(t.ttft.Milliseconds()), streamTPOT(latency, t.ttft, outcome), float64(latency.Milliseconds()))
}

func (t *streamTerminalState) recordSettlement(outcome terminalOutcome) {
	if !t.config.settle {
		return
	}
	t.service.settleTerminal(t.config.ctx, t.config.req, t.config.settlementDecision, outcome, t.latency())
}

func (t *streamTerminalState) releaseAdmission(terminalOutcome) {
	if t.config.release != nil {
		t.config.release()
	}
}

func (t *streamTerminalState) recordClientResult(outcome terminalOutcome) {
	if t.config.response == nil {
		return
	}
	t.config.response.Usage = terminalUsage(outcome)
}

func (t *streamTerminalState) latency() time.Duration {
	return time.Since(t.config.start)
}

func terminalHealthError(outcome terminalOutcome) error {
	if !outcome.affectsProviderHealth {
		return nil
	}
	return outcome.diagnosticCause
}

func terminalStatus(outcome terminalOutcome) (int, string) {
	return outcome.client.httpStatus, outcome.client.errorCode
}

func terminalUsage(outcome terminalOutcome) *llm.Usage {
	if !outcome.hasUsage {
		return nil
	}
	usage := outcome.usage
	return &usage
}

func streamTPOT(latency, ttft time.Duration, outcome terminalOutcome) float64 {
	if !outcome.hasUsage || outcome.usage.CompletionTokens == 0 {
		return 0
	}
	return float64(latency-ttft) / float64(outcome.usage.CompletionTokens) / float64(time.Millisecond)
}

func streamFallbackReason(attempts int) string {
	if attempts > 1 {
		return "provider_failure_or_rejected"
	}
	return ""
}

func streamScoreSummary(decision routing.RoutingDecision) string {
	if decision.CompositeScoreSummary == nil {
		return ""
	}
	summary, _ := json.Marshal(decision.CompositeScoreSummary)
	return string(summary)
}
