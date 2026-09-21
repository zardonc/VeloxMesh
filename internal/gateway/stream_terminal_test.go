package gateway

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	gatewayErrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

var terminalLifecycleOrder = []string{
	"classification",
	"provider_health",
	"circuit_breaker",
	"metrics",
	"trace_close",
	"settlement_decision",
	"admission_release",
	"client_terminal_emission",
}

type terminalCallbackRecorder struct {
	order    []string
	outcomes []terminalOutcome
}

func TestClassifyTerminal(t *testing.T) {
	usage := &llm.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}
	tests := []struct {
		name         string
		err          error
		kind         terminalOutcomeKind
		status       int
		healthImpact bool
		expectUsage  bool
		expectDiag   bool
	}{
		{name: "clean eof", kind: terminalCompleted, status: http.StatusOK, expectUsage: true},
		{
			name: "retryable provider error", err: gatewayErrors.NewGatewayError(gatewayErrors.ProviderUnavailable, "unavailable", http.StatusServiceUnavailable),
			kind: terminalProviderError, status: http.StatusServiceUnavailable, healthImpact: true, expectUsage: true, expectDiag: true,
		},
		{
			name: "non retryable provider error", err: gatewayErrors.NewGatewayError(gatewayErrors.ProviderInvalidRequest, "invalid", http.StatusBadRequest),
			kind: terminalProviderError, status: http.StatusBadRequest, expectUsage: true, expectDiag: true,
		},
		{name: "client cancellation", err: context.Canceled, kind: terminalClientCancelled, status: 499, expectUsage: true, expectDiag: true},
		{name: "policy rejection", err: gatewayErrors.ErrPolicyBlocked, kind: terminalPolicyRejected, status: http.StatusForbidden, expectUsage: true, expectDiag: true},
		{
			name: "unknown internal error", err: errors.New("opaque failure"), kind: terminalInternalAbnormal,
			status: http.StatusBadGateway, healthImpact: true, expectUsage: true, expectDiag: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := classifyTerminal(tt.err, usage)
			if outcome.kind != tt.kind || outcome.client.httpStatus != tt.status {
				t.Fatalf("outcome=%#v, want kind=%q status=%d", outcome, tt.kind, tt.status)
			}
			if outcome.affectsProviderHealth != tt.healthImpact || outcome.hasUsage != tt.expectUsage {
				t.Fatalf("outcome=%#v, want health=%t usage=%t", outcome, tt.healthImpact, tt.expectUsage)
			}
			if (outcome.diagnosticCause != nil) != tt.expectDiag {
				t.Fatalf("diagnostic presence=%t, want %t", outcome.diagnosticCause != nil, tt.expectDiag)
			}
			if outcome.hasUsage && outcome.usage != *usage {
				t.Fatalf("usage=%#v, want %#v", outcome.usage, *usage)
			}
		})
	}
}

func TestTerminalFinalizerFirstCandidateWins(t *testing.T) {
	tests := []struct {
		name   string
		first  terminalOutcome
		second terminalOutcome
	}{
		{
			name:   "provider error then completion",
			first:  classifyTerminal(gatewayErrors.NewGatewayError(gatewayErrors.ProviderUnavailable, "provider unavailable", 503), nil),
			second: classifyTerminal(nil, nil),
		},
		{
			name:   "cancellation then provider error",
			first:  classifyTerminal(context.Canceled, nil),
			second: classifyTerminal(gatewayErrors.NewGatewayError(gatewayErrors.ProviderTimeout, "provider timeout", 504), nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &terminalCallbackRecorder{}
			finalizer := newTerminalFinalizer(recorder.options())
			if !finalizer.Submit(tt.first) {
				t.Fatal("first terminal outcome was rejected")
			}
			if finalizer.Submit(tt.second) {
				t.Fatal("duplicate terminal outcome was accepted")
			}
			assertTerminalCallbacks(t, recorder, tt.first)
		})
	}
}

func TestTerminalFinalizerConcurrentSubmit(t *testing.T) {
	var providerHealthCount atomic.Int32
	var circuitBreakerCount atomic.Int32
	var metricsCount atomic.Int32
	var traceCloseCount atomic.Int32
	var settlementCount atomic.Int32
	var admissionCount atomic.Int32
	var clientCount atomic.Int32
	callback := func(counter *atomic.Int32) terminalCallback {
		return func(terminalOutcome) { counter.Add(1) }
	}
	finalizer := newTerminalFinalizer(terminalFinalizerOptions{
		providerHealth:         callback(&providerHealthCount),
		circuitBreaker:         callback(&circuitBreakerCount),
		metrics:                callback(&metricsCount),
		traceClose:             callback(&traceCloseCount),
		settlementDecision:     callback(&settlementCount),
		admissionRelease:       callback(&admissionCount),
		clientTerminalEmission: callback(&clientCount),
	})

	start := make(chan struct{})
	results := make(chan bool, 2)
	var waitGroup sync.WaitGroup
	for _, outcome := range []terminalOutcome{classifyTerminal(nil, &llm.Usage{TotalTokens: 3}), classifyTerminal(context.Canceled, nil)} {
		waitGroup.Go(func() {
			<-start
			results <- finalizer.Submit(outcome)
		})
	}
	close(start)
	waitGroup.Wait()
	close(results)

	accepted := 0
	for result := range results {
		if result {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted=%d, want 1", accepted)
	}
	for _, count := range []int32{providerHealthCount.Load(), circuitBreakerCount.Load(), metricsCount.Load(), traceCloseCount.Load(), settlementCount.Load(), admissionCount.Load(), clientCount.Load()} {
		if count != 1 {
			t.Fatalf("callback count=%d, want 1", count)
		}
	}
}

func (r *terminalCallbackRecorder) options() terminalFinalizerOptions {
	return terminalFinalizerOptions{
		providerHealth:         r.callback("provider_health"),
		circuitBreaker:         r.callback("circuit_breaker"),
		metrics:                r.callback("metrics"),
		traceClose:             r.callback("trace_close"),
		settlementDecision:     r.callback("settlement_decision"),
		admissionRelease:       r.callback("admission_release"),
		clientTerminalEmission: r.callback("client_terminal_emission"),
	}
}

func (r *terminalCallbackRecorder) callback(name string) terminalCallback {
	return func(outcome terminalOutcome) {
		r.order = append(r.order, name)
		r.outcomes = append(r.outcomes, outcome)
	}
}

func assertTerminalCallbacks(t *testing.T, recorder *terminalCallbackRecorder, expected terminalOutcome) {
	t.Helper()
	gotOrder := append([]string{"classification"}, recorder.order...)
	if !reflect.DeepEqual(gotOrder, terminalLifecycleOrder) {
		t.Fatalf("callback order=%v, want %v", gotOrder, terminalLifecycleOrder)
	}
	for _, outcome := range recorder.outcomes {
		if !reflect.DeepEqual(outcome, expected) {
			t.Fatalf("callback outcome=%#v, want %#v", outcome, expected)
		}
	}
	if len(recorder.outcomes) != len(terminalLifecycleOrder)-1 {
		t.Fatalf("callback outcomes=%d, want %d", len(recorder.outcomes), len(terminalLifecycleOrder)-1)
	}
}
