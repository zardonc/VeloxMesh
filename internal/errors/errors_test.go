package errors

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestGatewayError(t *testing.T) {
	err := NewGatewayError("test_code", "test message", http.StatusBadRequest)
	if err.Code != "test_code" {
		t.Errorf("expected code test_code, got %s", err.Code)
	}
	if err.Message != "test message" {
		t.Errorf("expected message test message, got %s", err.Message)
	}
	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, err.HTTPStatus)
	}
	expectedStr := "[test_code] test message"
	if err.Error() != expectedStr {
		t.Errorf("expected error string %q, got %q", expectedStr, err.Error())
	}
}

func TestAffectsProviderHealth(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "standard go error",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "provider auth error",
			err:      NewGatewayError(ProviderAuthError, "auth failed", http.StatusBadGateway),
			expected: true,
		},
		{
			name:     "provider rate limit",
			err:      NewGatewayError(ProviderRateLimit, "rate limit", http.StatusBadGateway),
			expected: true,
		},
		{
			name:     "provider invalid request",
			err:      NewGatewayError(ProviderInvalidRequest, "bad request", http.StatusBadRequest),
			expected: false,
		},
		{
			name:     "scheduler backpressure",
			err:      NewGatewayError(SchedulerBackpressure, "queue pressure", http.StatusTooManyRequests),
			expected: false,
		},
		{
			name:     "scheduler queue full",
			err:      NewGatewayError(SchedulerQueueFull, "queue full", http.StatusServiceUnavailable),
			expected: false,
		},
		{
			name:     "scheduler queue unavailable",
			err:      NewGatewayError(SchedulerQueueUnavailable, "queue unavailable", http.StatusServiceUnavailable),
			expected: false,
		},
		{
			name:     "provider invalid model",
			err:      NewGatewayError(ProviderInvalidModel, "model not found", http.StatusBadRequest),
			expected: true,
		},
		{
			name:     "unknown gateway error",
			err:      NewGatewayError("unknown_code", "unknown", http.StatusInternalServerError),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AffectsProviderHealth(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsRetryableProviderError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"standard error", errors.New("some_random_error"), false},
		{"provider rate limit", NewGatewayError(ProviderRateLimit, "test", 429), true},
		{"provider timeout", NewGatewayError(ProviderTimeout, "test", 504), true},
		{"provider unavailable", NewGatewayError(ProviderUnavailable, "test", 503), true},
		{"provider bad response", NewGatewayError(ProviderBadResponse, "test", 502), true},
		{"provider error", NewGatewayError(ProviderError, "test", 500), true},
		{"provider invalid request", NewGatewayError(ProviderInvalidRequest, "test", 400), false},
		{"provider invalid model", NewGatewayError(ProviderInvalidModel, "test", 400), false},
		{"provider auth error", NewGatewayError(ProviderAuthError, "test", 401), false},
		{"scheduler backpressure", NewGatewayError(SchedulerBackpressure, "test", 429), false},
		{"scheduler queue full", NewGatewayError(SchedulerQueueFull, "test", 503), false},
		{"scheduler queue unavailable", NewGatewayError(SchedulerQueueUnavailable, "test", 503), false},
		{"routing no healthy provider", ErrNoHealthyProvider, false},
		{"routing unknown override", ErrUnknownProviderOverride, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableProviderError(tt.err)
			if result != tt.retryable {
				t.Errorf("expected retryable=%v, got %v for error: %v", tt.retryable, result, tt.err)
			}
		})
	}
}
