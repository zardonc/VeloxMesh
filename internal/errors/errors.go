package errors

import "fmt"

// GatewayError represents a structured error returned by the gateway.
type GatewayError struct {
	Code       string
	Message    string
	HTTPStatus int
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
	ErrNoHealthyProvider         = NewGatewayError("no_healthy_provider", "no healthy providers available", 503)
	ErrUnknownProviderOverride   = NewGatewayError("unknown_provider_override", "requested provider override is unknown", 400)
	ErrUnhealthyProviderOverride = NewGatewayError("unhealthy_provider_override", "requested provider override is unhealthy", 503)
)

// Shared Provider Error Categories
const (
	ProviderAuthError      = "provider_auth_error"
	ProviderRateLimit      = "provider_rate_limit"
	ProviderInvalidRequest = "provider_invalid_request"
	ProviderInvalidModel   = "provider_invalid_model"
	ProviderTimeout        = "provider_timeout"
	ProviderUnavailable    = "provider_unavailable"
	ProviderBadResponse    = "provider_bad_response"
	ProviderError          = "provider_error"
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
	case ProviderInvalidModel:
		// Invalid model implies misconfiguration in provider setup, so it should degrade health
		return true
	case ProviderAuthError, ProviderRateLimit, ProviderTimeout, ProviderUnavailable, ProviderBadResponse, ProviderError:
		return true
	}
	
	// Unrecognized gateway errors default to affecting health
	return true
}
