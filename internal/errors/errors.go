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
