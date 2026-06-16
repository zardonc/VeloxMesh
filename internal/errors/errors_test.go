package errors

import (
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
