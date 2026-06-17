package adaptertest

import (
	"context"
	"strings"
	"testing"

	"veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type SuccessCase struct {
	Name                   string
	Request                *llm.LLMRequest
	SetupFake              func()
	AssertRequest          func(t *testing.T)
	ExpectedFinishReason   string
	ExpectedMessageContent string
	ExpectedModel          string
}

type ErrorCase struct {
	Name         string
	Request      *llm.LLMRequest
	SetupFake    func()
	ExpectedCode string
}

type HealthCase struct {
	Name           string
	SetupFake      func()
	ExpectedStatus providers.HealthStatus
}

type ConformanceSpec struct {
	Adapter              providers.ProviderAdapter
	ExpectedID           string
	ExpectedModels       []string
	ExpectedCapabilities providers.CapabilitySet

	HealthCases  []HealthCase
	SuccessCases []SuccessCase
	ErrorCases   []ErrorCase

	ForbiddenSecretSubstrings []string
}

func RunConformance(t *testing.T, spec ConformanceSpec) {
	t.Run("ID", func(t *testing.T) {
		if spec.Adapter.ID() != spec.ExpectedID {
			t.Errorf("expected ID %q, got %q", spec.ExpectedID, spec.Adapter.ID())
		}
	})

	t.Run("Models", func(t *testing.T) {
		models := spec.Adapter.Models()
		if len(models) != len(spec.ExpectedModels) {
			t.Errorf("expected %d models, got %d", len(spec.ExpectedModels), len(models))
		}
		for i, m := range spec.ExpectedModels {
			if i < len(models) && models[i] != m {
				t.Errorf("expected model %q at index %d, got %q", m, i, models[i])
			}
		}

		// Defensive copy check
		if len(models) > 0 {
			original := models[0]
			models[0] = "mutated"
			if spec.Adapter.Models()[0] == "mutated" {
				t.Error("Adapter.Models() does not return a defensive copy")
			}
			models[0] = original
		}
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := spec.Adapter.Capabilities()
		AssertCapabilitiesEqual(t, spec.ExpectedCapabilities, caps)
	})

	t.Run("Health", func(t *testing.T) {
		for _, hc := range spec.HealthCases {
			t.Run(hc.Name, func(t *testing.T) {
				if hc.SetupFake != nil {
					hc.SetupFake()
				}
				status := spec.Adapter.HealthCheck(context.Background())
				if status.Available != hc.ExpectedStatus.Available {
					t.Errorf("expected health Available=%v, got %v", hc.ExpectedStatus.Available, status.Available)
				}
			})
		}
	})

	t.Run("Success", func(t *testing.T) {
		for _, sc := range spec.SuccessCases {
			t.Run(sc.Name, func(t *testing.T) {
				if sc.SetupFake != nil {
					sc.SetupFake()
				}

				resp, err := spec.Adapter.Complete(context.Background(), sc.Request)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if sc.AssertRequest != nil {
					sc.AssertRequest(t)
				}

				if resp == nil {
					t.Fatal("expected non-nil response")
				}
				if resp.Model != sc.ExpectedModel {
					t.Errorf("expected model %q, got %q", sc.ExpectedModel, resp.Model)
				}
				if len(resp.Choices) == 0 {
					t.Fatal("expected at least one choice")
				}
				choice := resp.Choices[0]
				if choice.FinishReason != sc.ExpectedFinishReason {
					t.Errorf("expected finish reason %q, got %q", sc.ExpectedFinishReason, choice.FinishReason)
				}
				if choice.Message.Role != llm.RoleAssistant {
					t.Errorf("expected role assistant, got %q", choice.Message.Role)
				}
				if choice.Message.Content != sc.ExpectedMessageContent {
					t.Errorf("expected content %q, got %q", sc.ExpectedMessageContent, choice.Message.Content)
				}
			})
		}
	})

	t.Run("Errors", func(t *testing.T) {
		for _, ec := range spec.ErrorCases {
			t.Run(ec.Name, func(t *testing.T) {
				if ec.SetupFake != nil {
					ec.SetupFake()
				}

				req := ec.Request
				if req == nil && len(spec.SuccessCases) > 0 {
					req = spec.SuccessCases[0].Request
				}
				if req == nil {
					req = &llm.LLMRequest{
						Model: "default-model",
						Messages: []llm.Message{
							{Role: llm.RoleUser, Content: "Hello"},
						},
					}
				}

				_, err := spec.Adapter.Complete(context.Background(), req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				AssertGatewayError(t, err, ec.ExpectedCode)
				AssertSecretSafeError(t, err, spec.ForbiddenSecretSubstrings)
			})
		}
	})
}

// AssertGatewayError verifies that the error is a GatewayError with the expected code.
func AssertGatewayError(t *testing.T, err error, expectedCode string) {
	gwErr, ok := err.(*errors.GatewayError)
	if !ok {
		t.Fatalf("expected *errors.GatewayError, got %T: %v", err, err)
	}
	if gwErr.Code != expectedCode {
		t.Errorf("expected error code %q, got %q", expectedCode, gwErr.Code)
	}
}

// AssertSecretSafeError verifies that the error message does not contain any forbidden substrings.
func AssertSecretSafeError(t *testing.T, err error, forbiddenSubstrings []string) {
	if err == nil {
		return
	}
	msg := err.Error()
	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(msg, forbidden) {
			t.Errorf("error message contains forbidden substring %q: %v", forbidden, msg)
		}
	}
}

// AssertCapabilitiesEqual compares two CapabilitySet objects.
func AssertCapabilitiesEqual(t *testing.T, expected, actual providers.CapabilitySet) {
	if expected.ProviderType != actual.ProviderType {
		t.Errorf("expected ProviderType %q, got %q", expected.ProviderType, actual.ProviderType)
	}
	if expected.Streaming != actual.Streaming {
		t.Errorf("expected Streaming %v, got %v", expected.Streaming, actual.Streaming)
	}
	if expected.ToolCalling != actual.ToolCalling {
		t.Errorf("expected ToolCalling %v, got %v", expected.ToolCalling, actual.ToolCalling)
	}

	if len(expected.SupportedOperations) != len(actual.SupportedOperations) {
		t.Errorf("expected %d SupportedOperations, got %d", len(expected.SupportedOperations), len(actual.SupportedOperations))
	} else {
		for i, op := range expected.SupportedOperations {
			if actual.SupportedOperations[i] != op {
				t.Errorf("expected SupportedOperation %q at index %d, got %q", op, i, actual.SupportedOperations[i])
			}
		}
	}

	// Similar checks for modalities and parameters
	if len(expected.InputModalities) != len(actual.InputModalities) {
		t.Errorf("expected %d InputModalities, got %d", len(expected.InputModalities), len(actual.InputModalities))
	}
	if len(expected.OutputModalities) != len(actual.OutputModalities) {
		t.Errorf("expected %d OutputModalities, got %d", len(expected.OutputModalities), len(actual.OutputModalities))
	}
	if len(expected.GenerationParameters) != len(actual.GenerationParameters) {
		t.Errorf("expected %d GenerationParameters, got %d", len(expected.GenerationParameters), len(actual.GenerationParameters))
	}
}
