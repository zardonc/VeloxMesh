package errors

import "fmt"

// GatewayError represents a structured error returned by the gateway.
type GatewayError struct {
	Code    string
	Message string
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
