package errors

import (
	"context"
	"errors"
	"fmt"
)

// GatewayError represents a structured error returned by the gateway.
type GatewayError struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	HTTPStatus int               `json:"status"`
	Headers    map[string]string `json:"-"`
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func NewGatewayError(code, message string, httpStatus int) *GatewayError {
	return &GatewayError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Common routing errors
var (
	ErrNoHealthyProvider            = NewGatewayError("no_healthy_provider", "no healthy providers available", 503)
	ErrNoEligibleProvider           = NewGatewayError("no_eligible_provider", "no configured provider supports the requested model and operation", 400)
	ErrUnknownProviderOverride      = NewGatewayError("unknown_provider_override", "requested provider override is unknown", 400)
	ErrUnhealthyProviderOverride    = NewGatewayError("unhealthy_provider_override", "requested provider override is unhealthy", 503)
	ErrIneligibleProviderOverride   = NewGatewayError("ineligible_provider_override", "requested provider override does not support the requested model and operation", 400)
	ErrCompositeScoreBelowThreshold = NewGatewayError("composite_score_below_threshold", "no provider met the minimum composite score threshold", 503)

	// Control state runtime errors
	ErrNoActiveProviderConfig     = NewGatewayError("no_active_provider_config", "no active provider configuration exists; create and enable a provider through /admin/v1/providers", 503)
	ErrMissingProviderSecret      = NewGatewayError("missing_provider_secret", "missing provider secret", 500)
	ErrMissingProviderModelConfig = NewGatewayError("missing_provider_model_config", "missing provider model config", 400)
	ErrProviderActivationFailed   = NewGatewayError("provider_activation_failed", "provider activation failed", 500)
	ErrServiceNotWritable         = NewGatewayError("service_unavailable", "service temporarily unavailable for writes", 503)

	// Pipeline errors
	ErrPolicyBlocked = NewGatewayError("policy_blocked", "request blocked by semantic policy", 403)
)

// Shared Provider Error Categories
const (
	ProviderAuthError         = "provider_auth_error"
	ProviderRateLimit         = "provider_rate_limit"
	ProviderInvalidRequest    = "provider_invalid_request"
	ProviderInvalidModel      = "provider_invalid_model"
	ProviderTimeout           = "provider_timeout"
	ProviderUnavailable       = "provider_unavailable"
	ProviderBadResponse       = "provider_bad_response"
	ProviderError             = "provider_error"
	SchedulerBackpressure     = "scheduler_backpressure"
	SchedulerQueueFull        = "scheduler_queue_full"
	SchedulerQueueUnavailable = "scheduler_queue_unavailable"
	SchedulerDuplicateTask    = "scheduler_duplicate_task"
)

// AffectsProviderHealth determines whether a given error should count as a provider failure
// that increments consecutive failure counters and causes health degradation.
func AffectsProviderHealth(err error) bool {
	if err == nil {
		return false
	}
	gwErr, ok := err.(*GatewayError)
	if !ok {
		// Non-gateway errors (e.g., standard context timeout/cancellation not wrapped) might be client-side
		// but typically we wrap everything upstream. We will err on the side of health impact.
		return true
	}

	switch gwErr.Code {
	case ProviderInvalidRequest:
		// Invalid requests caused by client input should not poison provider health
		return false
	case SchedulerBackpressure, SchedulerQueueFull, SchedulerQueueUnavailable:
		return false
	case ProviderInvalidModel:
		// Invalid model implies misconfiguration in provider setup, so it should degrade health
		return true
	case ProviderAuthError, ProviderRateLimit, ProviderTimeout, ProviderUnavailable, ProviderBadResponse, ProviderError:
		return true
	}

	// Unrecognized gateway errors default to affecting health
	return true
}

// IsRetryableProviderError determines if a provider error is transient and safe to retry.
// It returns false for client-side errors, auth errors, and non-gateway errors (like context cancellation).
func IsRetryableProviderError(err error) bool {
	if err == nil {
		return false
	}
	gwErr, ok := err.(*GatewayError)
	if !ok {
		return false
	}

	switch gwErr.Code {
	case ProviderRateLimit, ProviderTimeout, ProviderUnavailable, ProviderBadResponse, ProviderError:
		return true
	}

	// Default to false for unrecognized or non-transient errors like InvalidRequest, InvalidModel, AuthError
	return false
}

// TranslateError converts any standard error into a *GatewayError.
// If it's already a GatewayError, it is returned as-is.
func TranslateError(err error) *GatewayError {
	if err == nil {
		return nil
	}

	var gwErr *GatewayError
	if errors.As(err, &gwErr) {
		return gwErr
	}

	if errors.Is(err, context.Canceled) {
		return NewGatewayError("client_disconnected", "client disconnected", 499)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return NewGatewayError("gateway_timeout", "gateway processing timeout", 504)
	}

	return NewGatewayError(ProviderError, fmt.Sprintf("Upstream error: %v", err), 502)
}
