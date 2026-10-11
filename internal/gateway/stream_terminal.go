package gateway

import (
	"context"
	stdlibErrors "errors"
	"net/http"
	"sync"

	gatewayErrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

type terminalOutcomeKind string

const (
	terminalCompleted        terminalOutcomeKind = "completed"
	terminalProviderError    terminalOutcomeKind = "provider_error"
	terminalClientCancelled  terminalOutcomeKind = "client_cancelled"
	terminalPolicyRejected   terminalOutcomeKind = "policy_rejected"
	terminalInternalAbnormal terminalOutcomeKind = "internal_abnormal"
	terminalFinishStop                           = "stop"
)

type terminalClientResult struct {
	finishReason string
	errorCode    string
	errorMessage string
	httpStatus   int
}

type terminalOutcome struct {
	kind                  terminalOutcomeKind
	client                terminalClientResult
	usage                 llm.Usage
	hasUsage              bool
	affectsProviderHealth bool
	diagnosticCause       error
}

func classifyTerminal(err error, usage *llm.Usage) terminalOutcome {
	usageSnapshot, hasUsage := snapshotTerminalUsage(usage)
	if err == nil {
		return terminalOutcome{
			kind: terminalCompleted,
			client: terminalClientResult{
				finishReason: terminalFinishStop,
				httpStatus:   http.StatusOK,
			},
			usage:    usageSnapshot,
			hasUsage: hasUsage,
		}
	}

	client := clientTerminalResult(err)
	if stdlibErrors.Is(err, context.Canceled) {
		return terminalOutcome{
			kind:            terminalClientCancelled,
			client:          client,
			usage:           usageSnapshot,
			hasUsage:        hasUsage,
			diagnosticCause: err,
		}
	}

	var gatewayError *gatewayErrors.GatewayError
	if stdlibErrors.As(err, &gatewayError) {
		if gatewayError.Code == gatewayErrors.ErrPolicyBlocked.Code {
			return terminalOutcome{
				kind:            terminalPolicyRejected,
				client:          client,
				usage:           usageSnapshot,
				hasUsage:        hasUsage,
				diagnosticCause: err,
			}
		}
		if isProviderTerminalError(gatewayError.Code) {
			return terminalOutcome{
				kind:                  terminalProviderError,
				client:                client,
				usage:                 usageSnapshot,
				hasUsage:              hasUsage,
				affectsProviderHealth: gatewayErrors.AffectsProviderHealth(err),
				diagnosticCause:       err,
			}
		}
	}

	return terminalOutcome{
		kind:                  terminalInternalAbnormal,
		client:                client,
		usage:                 usageSnapshot,
		hasUsage:              hasUsage,
		affectsProviderHealth: gatewayErrors.AffectsProviderHealth(err),
		diagnosticCause:       err,
	}
}

func snapshotTerminalUsage(usage *llm.Usage) (llm.Usage, bool) {
	if usage == nil {
		return llm.Usage{}, false
	}
	return *usage, true
}

func clientTerminalResult(err error) terminalClientResult {
	translated := gatewayErrors.TranslateError(err)
	return terminalClientResult{
		errorCode:    translated.Code,
		errorMessage: translated.Message,
		httpStatus:   translated.HTTPStatus,
	}
}

func isProviderTerminalError(code string) bool {
	switch code {
	case gatewayErrors.ProviderAuthError, gatewayErrors.ProviderRateLimit, gatewayErrors.ProviderInvalidRequest, gatewayErrors.ProviderInvalidModel:
		return true
	case gatewayErrors.ProviderTimeout, gatewayErrors.ProviderUnavailable, gatewayErrors.ProviderBadResponse, gatewayErrors.ProviderError:
		return true
	default:
		return false
	}
}

type terminalCallback func(terminalOutcome)

type terminalFinalizerOptions struct {
	providerHealth         terminalCallback
	circuitBreaker         terminalCallback
	metrics                terminalCallback
	traceClose             terminalCallback
	settlementDecision     terminalCallback
	admissionRelease       terminalCallback
	clientTerminalEmission terminalCallback
}

type terminalFinalizer struct {
	mu        sync.Mutex
	completed bool
	callbacks terminalFinalizerOptions
}

func newTerminalFinalizer(options terminalFinalizerOptions) *terminalFinalizer {
	return &terminalFinalizer{callbacks: options}
}

// Submit applies callbacks only to the first pre-classified terminal outcome.
func (f *terminalFinalizer) Submit(outcome terminalOutcome) bool {
	f.mu.Lock()
	if f.completed {
		f.mu.Unlock()
		return false
	}
	f.completed = true
	f.mu.Unlock()

	f.run(outcome)
	return true
}

func (f *terminalFinalizer) run(outcome terminalOutcome) {
	runTerminalCallback(f.callbacks.providerHealth, outcome)
	runTerminalCallback(f.callbacks.circuitBreaker, outcome)
	runTerminalCallback(f.callbacks.metrics, outcome)
	runTerminalCallback(f.callbacks.traceClose, outcome)
	runTerminalCallback(f.callbacks.settlementDecision, outcome)
	runTerminalCallback(f.callbacks.admissionRelease, outcome)
	runTerminalCallback(f.callbacks.clientTerminalEmission, outcome)
}

func runTerminalCallback(callback terminalCallback, outcome terminalOutcome) {
	if callback == nil {
		return
	}
	callback(outcome)
}
