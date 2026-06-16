package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veloxmesh/internal/llm"
)

func TestAdapter_Complete(t *testing.T) {
	tests := []struct {
		name           string
		request        *llm.LLMRequest
		mockStatus     int
		mockResponse   any
		expectedText   string
		expectedReason string
		expectError    bool
	}{
		{
			name: "successful text completion with end_turn",
			request: &llm.LLMRequest{
				Model: "claude-3-5-sonnet-20240620",
				Messages: []llm.Message{
					{Role: llm.RoleSystem, Content: "You are a helpful assistant."},
					{Role: llm.RoleUser, Content: "Hello!"},
				},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"id":    "msg_123",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-3-5-sonnet-20240620",
				"content": []map[string]any{
					{"type": "text", "text": "Hi there!"},
				},
				"stop_reason": "end_turn",
				"usage": map[string]int{
					"input_tokens":  10,
					"output_tokens": 5,
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
				"id":    "msg_124",
				"type":  "message",
				"role":  "assistant",
				"model": "claude-3-5-sonnet-20240620",
				"content": []map[string]any{
					{"type": "text", "text": "1 2 3"},
				},
				"stop_reason": "max_tokens",
			},
			expectedText:   "1 2 3",
			expectedReason: "length",
		},
		{
			name: "rate limit error",
			request: &llm.LLMRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
			},
			mockStatus: http.StatusTooManyRequests,
			mockResponse: map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "rate_limit_error",
					"message": "Rate limit exceeded",
				},
			},
			expectError: true,
		},
		{
			name: "auth error",
			request: &llm.LLMRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
			},
			mockStatus: http.StatusUnauthorized,
			mockResponse: map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "authentication_error",
					"message": "invalid api key",
				},
			},
			expectError: true,
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

			adapter := NewAdapter("anthropic-1", server.URL+"/", "test-key", "claude-3-5-sonnet-20240620")

			resp, err := adapter.Complete(context.Background(), tt.request)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
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
