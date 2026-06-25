package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/adaptertest"
)

func TestAdapter_Capabilities(t *testing.T) {
	adapter := NewAdapter("gemini-1", "https://example.test/", "test-key", "gemini-1.5-pro")
	caps := adapter.Capabilities()

	if caps.ProviderType != providers.ProviderTypeGemini {
		t.Errorf("expected provider type %q, got %q", providers.ProviderTypeGemini, caps.ProviderType)
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
	if !caps.Streaming {
		t.Error("expected streaming to be true")
	}
	if !caps.ToolCalling {
		t.Error("expected tool calling to be true")
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

func TestAdapter_Conformance(t *testing.T) {
	var mockStatus int
	var mockResponse any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(mockStatus)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	adapter := NewAdapter("gemini-1", server.URL+"/", "test-key", "gemini-1.5-pro")

	spec := adaptertest.ConformanceSpec{
		Adapter:        adapter,
		ExpectedID:     "gemini-1",
		ExpectedModels: []string{"gemini-1.5-pro"},
		ExpectedCapabilities: providers.CapabilitySet{
			ProviderType:        providers.ProviderTypeGemini,
			SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
			InputModalities:     []providers.Modality{providers.ModalityText},
			OutputModalities:    []providers.Modality{providers.ModalityText},
			Streaming:           true,
			ToolCalling:         true,
			GenerationParameters: []providers.GenerationParameter{
				providers.GenerationParameterTemperature,
				providers.GenerationParameterMaxTokens,
			},
		},
		ForbiddenSecretSubstrings: []string{"test-key", "x-goog-api-key"},
		HealthCases: []adaptertest.HealthCase{
			{
				Name: "available",
				SetupFake: func() {
				},
				ExpectedStatus: providers.HealthStatus{Available: true, Message: "Healthy"},
			},
		},
		SuccessCases: []adaptertest.SuccessCase{
			{
				Name: "basic success",
				Request: &llm.LLMRequest{
					Model: "gemini-1.5-pro",
					Messages: []llm.Message{
						{Role: llm.RoleUser, Content: "Hello"},
					},
				},
				SetupFake: func() {
					mockStatus = http.StatusOK
					mockResponse = map[string]any{
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
					}
				},
				ExpectedModel:          "gemini-1.5-pro",
				ExpectedMessageContent: "Hi there!",
				ExpectedFinishReason:   "stop",
			},
		},
		ErrorCases: []adaptertest.ErrorCase{
			{
				Name: "auth error",
				SetupFake: func() {
					mockStatus = http.StatusUnauthorized
					mockResponse = map[string]any{
						"error": map[string]any{
							"code":    401,
							"message": "API key not valid. Please pass a valid API key.",
							"status":  "UNAUTHENTICATED",
						},
					}
				},
				ExpectedCode: gatewayErr.ProviderAuthError,
			},
			{
				Name: "rate limit error",
				SetupFake: func() {
					mockStatus = http.StatusTooManyRequests
					mockResponse = map[string]any{
						"error": map[string]any{
							"code":    429,
							"message": "Quota exceeded",
							"status":  "RESOURCE_EXHAUSTED",
						},
					}
				},
				ExpectedCode: gatewayErr.ProviderRateLimit,
			},
			{
				Name: "bad response",
				SetupFake: func() {
					mockStatus = http.StatusOK
					mockResponse = map[string]any{
						"candidates": []map[string]any{},
					}
				},
				ExpectedCode: gatewayErr.ProviderBadResponse,
			},
		},
	}

	adaptertest.RunConformance(t, spec)
}
