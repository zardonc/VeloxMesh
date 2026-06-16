package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

func TestAdapter_Complete(t *testing.T) {
	tests := []struct {
		name            string
		request         *llm.LLMRequest
		mockStatus      int
		mockResponse    any
		expectedText    string
		expectedReason  string
		expectError     bool
		expectedErrCode string
	}{
		{
			name: "successful text completion",
			request: &llm.LLMRequest{
				Model: "gemini-1.5-pro",
				Messages: []llm.Message{
					{Role: llm.RoleSystem, Content: "You are a helpful assistant."},
					{Role: llm.RoleUser, Content: "Hello!"},
				},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "Hi there!"},
							},
							"role": "model",
						},
						"finishReason": "STOP",
					},
				},
			},
			expectedText:   "Hi there!",
			expectedReason: "stop",
		},
		{
			name: "max_tokens stop reason",
			request: &llm.LLMRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Count to 100"},
				},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "1 2 3"},
							},
						},
						"finishReason": "MAX_TOKENS",
					},
				},
			},
			expectedText:   "1 2 3",
			expectedReason: "length",
		},
		{
			name: "safety stop reason",
			request: &llm.LLMRequest{
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Bad stuff"},
				},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": ""},
							},
						},
						"finishReason": "SAFETY",
					},
				},
			},
			expectedText:   "",
			expectedReason: "safety",
		},
		{
			name: "rate limit error",
			request: &llm.LLMRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
			},
			mockStatus: http.StatusTooManyRequests,
			mockResponse: map[string]any{
				"error": map[string]any{
					"code":    429,
					"message": "Quota exceeded",
					"status":  "RESOURCE_EXHAUSTED",
				},
			},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderRateLimit,
		},
		{
			name: "auth error",
			request: &llm.LLMRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
			},
			mockStatus: http.StatusUnauthorized,
			mockResponse: map[string]any{
				"error": map[string]any{
					"code":    401,
					"message": "API key not valid. Please pass a valid API key.",
					"status":  "UNAUTHENTICATED",
				},
			},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderAuthError,
		},
		{
			name: "bad response - empty candidates",
			request: &llm.LLMRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"candidates": []map[string]any{},
			},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderBadResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatus)
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			adapter := NewAdapter("gemini-1", server.URL+"/", "test-key", "gemini-1.5-pro")

			resp, err := adapter.Complete(context.Background(), tt.request)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if tt.expectedErrCode != "" {
					gwErr, ok := err.(*gatewayErr.GatewayError)
					if !ok {
						t.Fatalf("expected GatewayError, got %T: %v", err, err)
					}
					if gwErr.Code != tt.expectedErrCode {
						t.Errorf("expected error code %q, got %q", tt.expectedErrCode, gwErr.Code)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(resp.Choices) == 0 {
				t.Fatalf("expected choices, got none")
			}

			choice := resp.Choices[0]
			if choice.Message.Content != tt.expectedText {
				t.Errorf("expected text %q, got %q", tt.expectedText, choice.Message.Content)
			}

			if choice.FinishReason != tt.expectedReason {
				t.Errorf("expected finish reason %q, got %q", tt.expectedReason, choice.FinishReason)
			}
		})
	}
}
