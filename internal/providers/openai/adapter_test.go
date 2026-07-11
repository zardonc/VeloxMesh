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
	"veloxmesh/internal/providers/adaptertest"
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

func TestAdapter_CompleteMapsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"model": "gpt-4",
			"choices": []map[string]any{{
				"message": map[string]string{"content": "Hi"},
			}},
			"usage": map[string]int{
				"prompt_tokens":     3,
				"completion_tokens": 5,
				"total_tokens":      8,
			},
		})
	}))
	defer server.Close()

	resp, err := NewAdapter("test-openai", server.URL, "test-key", "gpt-4").Complete(context.Background(), &llm.LLMRequest{
		Model: "gpt-4", Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 3 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 8 {
		t.Fatalf("usage not mapped: %#v", resp.Usage)
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

	adapter := NewAdapter("test-openai", server.URL, "test-key", "gpt-4")

	spec := adaptertest.ConformanceSpec{
		Adapter:        adapter,
		ExpectedID:     "test-openai",
		ExpectedModels: []string{"gpt-4"},
		ExpectedCapabilities: providers.CapabilitySet{
			ProviderType:        providers.ProviderTypeOpenAICompatible,
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
		ForbiddenSecretSubstrings: []string{"test-key", "Bearer "},
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
					Model: "gpt-4",
					Messages: []llm.Message{
						{Role: llm.RoleUser, Content: "Hello"},
					},
				},
				SetupFake: func() {
					mockStatus = http.StatusOK
					mockResponse = map[string]any{
						"model": "gpt-4",
						"choices": []map[string]any{
							{
								"message": map[string]any{
									"role":    "assistant",
									"content": "Hi there!",
								},
								"finish_reason": "stop",
							},
						},
					}
				},
				ExpectedModel:          "gpt-4",
				ExpectedMessageContent: "Hi there!",
				ExpectedFinishReason:   "stop",
			},
		},
		ErrorCases: []adaptertest.ErrorCase{
			{
				Name: "auth error",
				SetupFake: func() {
					mockStatus = http.StatusUnauthorized
					mockResponse = map[string]any{}
				},
				ExpectedCode: gatewayErr.ProviderAuthError,
			},
			{
				Name: "rate limit error",
				SetupFake: func() {
					mockStatus = http.StatusTooManyRequests
					mockResponse = map[string]any{}
				},
				ExpectedCode: gatewayErr.ProviderRateLimit,
			},
			{
				Name: "bad response",
				SetupFake: func() {
					mockStatus = http.StatusOK
					mockResponse = map[string]any{
						"choices": []map[string]any{},
					}
				},
				ExpectedCode: gatewayErr.ProviderBadResponse,
			},
			{
				Name: "invalid model",
				SetupFake: func() {
					mockStatus = http.StatusNotFound
					mockResponse = map[string]any{}
				},
				ExpectedCode: gatewayErr.ProviderInvalidModel,
			},
		},
	}

	adaptertest.RunConformance(t, spec)
}

func TestAdapter_Stream(t *testing.T) {
	mockResponse := "data: {\"model\":\"gpt-4\",\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\ndata: {\"model\":\"gpt-4\",\"choices\":[{\"delta\":{\"content\":\"World\"}}]}\n\ndata: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	adapter := NewAdapter("test-openai", server.URL, "test-key", "gpt-4")

	req := &llm.LLMRequest{
		Model: "gpt-4",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Say Hello World"},
		},
	}

	ch, err := adapter.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var contents []string
	var done bool
	for event := range ch {
		if event.Error != nil {
			t.Fatalf("unexpected event error: %v", event.Error)
		}
		if event.Done {
			done = true
		} else {
			contents = append(contents, event.DeltaContent)
		}
	}

	if !done {
		t.Error("expected done event")
	}

	if len(contents) != 2 || contents[0] != "Hello " || contents[1] != "World" {
		t.Errorf("unexpected chunks: %v", contents)
	}
}

func TestAdapter_StreamTreatsEOFAsDone(t *testing.T) {
	mockResponse := "data: {\"model\":\"gpt-4\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	ch, err := NewAdapter("test-openai", server.URL, "test-key", "gpt-4").Stream(context.Background(), &llm.LLMRequest{
		Model: "gpt-4", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	done := false
	for event := range ch {
		if event.Error != nil {
			t.Fatalf("unexpected event error: %v", event.Error)
		}
		if event.Done {
			done = true
		}
	}
	if !done {
		t.Fatalf("expected done event on EOF")
	}
}

func TestAdapter_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("expected path /embeddings, got %s", r.URL.Path)
		}
		var reqBody map[string]any
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "text-embedding-3-small" {
			t.Errorf("expected model text-embedding-3-small, got %v", reqBody["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float32{0.1, 0.2, 0.3},
				},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]any{
				"prompt_tokens": 10,
				"total_tokens":  10,
			},
		})
	}))
	defer server.Close()

	adapter := NewAdapter("test-openai", server.URL, "test-key", "text-embedding-3-small")
	req := &llm.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{"Hello world"},
	}

	resp, err := adapter.Embed(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Model != "text-embedding-3-small" {
		t.Errorf("expected model text-embedding-3-small, got %s", resp.Model)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(resp.Data))
	}

	if len(resp.Data[0].Embedding) != 3 || resp.Data[0].Embedding[0] != 0.1 {
		t.Errorf("unexpected embedding: %v", resp.Data[0].Embedding)
	}
}
