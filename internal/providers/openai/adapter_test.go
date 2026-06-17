package openai

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
	adapter := NewAdapter("test-openai", "https://example.test/v1", "test-key", "gpt-4")
	caps := adapter.Capabilities()

	if caps.ProviderType != providers.ProviderTypeOpenAICompatible {
		t.Errorf("expected provider type %q, got %q", providers.ProviderTypeOpenAICompatible, caps.ProviderType)
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
		expectError     bool
		expectedErrCode string
	}{
		{
			name: "success",
			request: &llm.LLMRequest{
				Model: "gpt-4",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Hello"},
				},
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"model": "gpt-4",
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"content": "Hi",
						},
					},
				},
			},
			expectedText: "Hi",
		},
		{
			name:            "auth error",
			request:         &llm.LLMRequest{},
			mockStatus:      http.StatusUnauthorized,
			mockResponse:    map[string]any{},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderAuthError,
		},
		{
			name:            "rate limit",
			request:         &llm.LLMRequest{},
			mockStatus:      http.StatusTooManyRequests,
			mockResponse:    map[string]any{},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderRateLimit,
		},
		{
			name:            "invalid model",
			request:         &llm.LLMRequest{},
			mockStatus:      http.StatusNotFound,
			mockResponse:    map[string]any{},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderInvalidModel,
		},
		{
			name:       "bad response",
			request:    &llm.LLMRequest{},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"choices": []map[string]any{}, // empty choices
			},
			expectError:     true,
			expectedErrCode: gatewayErr.ProviderBadResponse,
		},
		{
			name: "parameter forwarding",
			request: &llm.LLMRequest{
				Model: "gpt-4",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Hello"},
				},
				Temperature: func() *float64 { f := 0.7; return &f }(),
				MaxTokens:   func() *int { i := 100; return &i }(),
			},
			mockStatus: http.StatusOK,
			mockResponse: map[string]any{
				"model": "gpt-4",
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"content": "Hi",
						},
					},
				},
			},
			expectedText: "Hi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.name == "parameter forwarding" {
					var reqBody map[string]any
					json.NewDecoder(r.Body).Decode(&reqBody)
					if reqBody["temperature"] != 0.7 {
						t.Errorf("expected temperature 0.7, got %v", reqBody["temperature"])
					}
					if reqBody["max_tokens"] != float64(100) {
						t.Errorf("expected max_tokens 100, got %v", reqBody["max_tokens"])
					}
				}
				w.WriteHeader(tt.mockStatus)
				json.NewEncoder(w).Encode(tt.mockResponse)
			}))
			defer server.Close()

			adapter := NewAdapter("test-openai", server.URL, "test-key", "gpt-4")
			resp, err := adapter.Complete(context.Background(), tt.request)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				gwErr, ok := err.(*gatewayErr.GatewayError)
				if !ok {
					t.Fatalf("expected GatewayError, got %T: %v", err, err)
				}
				if gwErr.Code != tt.expectedErrCode {
					t.Errorf("expected error code %s, got %s", tt.expectedErrCode, gwErr.Code)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Choices) == 0 {
				t.Fatalf("expected choices, got none")
			}
			if resp.Choices[0].Message.Content != tt.expectedText {
				t.Errorf("expected %s, got %s", tt.expectedText, resp.Choices[0].Message.Content)
			}
		})
	}
}
