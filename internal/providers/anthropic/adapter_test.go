package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

func TestAdapter_Capabilities(t *testing.T) {
	adapter := NewAdapter("anthropic-1", "https://example.test/", "test-key", "claude-3-5-sonnet-20240620")
	caps := adapter.Capabilities()

	if caps.ProviderType != providers.ProviderTypeAnthropic {
		t.Errorf("expected provider type %q, got %q", providers.ProviderTypeAnthropic, caps.ProviderType)
	}
	if len(caps.SupportedOperations) != 1 || caps.SupportedOperations[0] != providers.OperationChatCompletions {
		t.Errorf("expected chat_completions operation, got %v", caps.SupportedOperations)
	}
	if len(caps.InputModalities) != 1 || caps.InputModalities[0] != providers.ModalityText {
		t.Errorf("expected text input modality, got %v", caps.InputModalities)
	}
	if len(caps.OutputModalities) != 1 || caps.OutputModalities[0] != providers.ModalityText {
		t.Errorf("expected text output modality, got %v", caps.OutputModalities)
	}
	if caps.Streaming {
		t.Error("expected streaming to be false")
	}
	if caps.ToolCalling {
		t.Error("expected tool calling to be false")
	}
	expectedParams := []providers.GenerationParameter{
		providers.GenerationParameterTemperature,
		providers.GenerationParameterMaxTokens,
	}
	if len(caps.GenerationParameters) != len(expectedParams) {
		t.Fatalf("expected %d generation parameters, got %d", len(expectedParams), len(caps.GenerationParameters))
	}
	for i, expected := range expectedParams {
		if caps.GenerationParameters[i] != expected {
			t.Errorf("expected generation parameter %d to be %q, got %q", i, expected, caps.GenerationParameters[i])
		}
	}
}

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
				"type": "error",
				"error": map[string]any{
					"type":    "authentication_error",
					"message": "invalid api key",
				},
			},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderAuthError,
		},
		{
			name: "bad response - empty content",
			request: &llm.LLMRequest{
				Messages: []llm.Message{{Role: llm.RoleUser, Content: "Test"}},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"id":      "msg_125",
				"type":    "message",
				"role":    "assistant",
				"model":   "claude-3-5-sonnet-20240620",
				"content": []map[string]any{},
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

			adapter := NewAdapter("anthropic-1", server.URL+"/", "test-key", "claude-3-5-sonnet-20240620")

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
