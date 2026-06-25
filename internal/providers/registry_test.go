package providers_test

import (
	"context"
	"testing"
	"veloxmesh/internal/config"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type mockAdapter struct {
	id     string
	models []string
}

func (m *mockAdapter) ID() string {
	return m.id
}

func (m *mockAdapter) Models() []string {
	return m.models
}

func (m *mockAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, nil
}

func (m *mockAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:         providers.ProviderTypeOpenAICompatible,
		SupportedOperations:  []providers.Operation{providers.OperationChatCompletions},
		InputModalities:      []providers.Modality{providers.ModalityText},
		OutputModalities:     []providers.Modality{providers.ModalityText},
		GenerationParameters: []providers.GenerationParameter{providers.GenerationParameterTemperature},
	}
}

func (m *mockAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}

func TestRegistry(t *testing.T) {
	cfg := &config.Config{DefaultProvider: "p1"}
	p1 := &mockAdapter{id: "p1", models: []string{"modelA", "modelB"}}
	p2 := &mockAdapter{id: "p2", models: []string{"modelB", "modelC"}}

	registry := providers.NewRegistry(cfg, []providers.ProviderAdapter{p1, p2}, nil)

	t.Run("Get", func(t *testing.T) {
		got, err := registry.Get("p2")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got.ID() != "p2" {
			t.Errorf("expected p2, got %s", got.ID())
		}
	})

	t.Run("Get unknown", func(t *testing.T) {
		_, err := registry.Get("unknown")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("GetDefault", func(t *testing.T) {
		got, err := registry.GetDefault()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if got.ID() != "p1" {
			t.Errorf("expected p1, got %s", got.ID())
		}
	})

	t.Run("List and IDs stable order", func(t *testing.T) {
		ids := registry.IDs()
		if len(ids) != 2 || ids[0] != "p1" || ids[1] != "p2" {
			t.Errorf("unexpected IDs: %v", ids)
		}

		list := registry.List()
		if len(list) != 2 || list[0].ID() != "p1" || list[1].ID() != "p2" {
			t.Errorf("unexpected List: %v", list)
		}
	})

	t.Run("GetAllModels deduplicates and preserves order", func(t *testing.T) {
		models := registry.GetAllModels()
		expected := []string{"modelA", "modelB", "modelC"}
		if len(models) != len(expected) {
			t.Fatalf("expected %d models, got %d", len(expected), len(models))
		}
		for i, m := range expected {
			if models[i] != m {
				t.Errorf("expected model %d to be %s, got %s", i, m, models[i])
			}
		}
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps, err := registry.Capabilities("p1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if caps.ProviderType != providers.ProviderTypeOpenAICompatible {
			t.Errorf("expected openai-compatible, got %s", caps.ProviderType)
		}
		caps.SupportedOperations[0] = "mutated"
		capsAgain, err := registry.Capabilities("p1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if capsAgain.SupportedOperations[0] == "mutated" {
			t.Error("capability slices were not copied")
		}
	})

	t.Run("Capabilities unknown", func(t *testing.T) {
		_, err := registry.Capabilities("unknown")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("AllCapabilities", func(t *testing.T) {
		allCaps := registry.AllCapabilities()
		if len(allCaps) != 2 {
			t.Fatalf("expected 2 capabilities, got %d", len(allCaps))
		}
		if allCaps[0].ID != "p1" || allCaps[1].ID != "p2" {
			t.Errorf("unexpected ordering in AllCapabilities: %v", allCaps)
		}
		if allCaps[0].Capabilities.ProviderType != providers.ProviderTypeOpenAICompatible {
			t.Errorf("expected openai-compatible for p1, got %s", allCaps[0].Capabilities.ProviderType)
		}
		if len(allCaps[0].Models) != 2 {
			t.Errorf("expected 2 models for p1, got %d", len(allCaps[0].Models))
		}

		// Ensure copy
		allCaps[0].Models[0] = "mutated"
		if registry.GetAllModels()[0] == "mutated" {
			t.Error("models slice was not a copy")
		}
		allCaps[0].Capabilities.InputModalities[0] = "mutated"
		allCapsAgain := registry.AllCapabilities()
		if allCapsAgain[0].Capabilities.InputModalities[0] == "mutated" {
			t.Error("capability metadata was not a copy")
		}
	})

	t.Run("EligibleProviders", func(t *testing.T) {
		eligible := registry.EligibleProviders("modelB", providers.OperationChatCompletions)
		if len(eligible) != 2 {
			t.Fatalf("expected 2 eligible providers for modelB, got %d", len(eligible))
		}
		if eligible[0].ProviderID != "p1" || eligible[1].ProviderID != "p2" {
			t.Errorf("unexpected eligible providers for modelB: %v", eligible)
		}

		// Ensure copy safety
		eligible[0].ProviderID = "mutated"
		eligible[0].Capabilities.SupportedOperations[0] = "mutated_op"
		again := registry.EligibleProviders("modelB", providers.OperationChatCompletions)
		if again[0].ProviderID == "mutated" || again[0].Capabilities.SupportedOperations[0] == "mutated_op" {
			t.Error("EligibleProviders did not return a safe copy")
		}

		none := registry.EligibleProviders("unknown", providers.OperationChatCompletions)
		if len(none) != 0 {
			t.Errorf("expected 0 eligible providers for unknown model, got %d", len(none))
		}
	})

	t.Run("ProviderSupports", func(t *testing.T) {
		if !registry.ProviderSupports("p1", "modelA", providers.OperationChatCompletions) {
			t.Error("expected p1 to support modelA and chat_completions")
		}
		if registry.ProviderSupports("p1", "modelC", providers.OperationChatCompletions) {
			t.Error("expected p1 to NOT support modelC")
		}
		if registry.ProviderSupports("p2", "modelA", providers.OperationChatCompletions) {
			t.Error("expected p2 to NOT support modelA")
		}
		if registry.ProviderSupports("p1", "modelA", "unknown_operation") {
			t.Error("expected p1 to NOT support unknown_operation")
		}
	})

	t.Run("DefaultModel", func(t *testing.T) {
		model, ok := registry.DefaultModel("p1")
		// Config didn't specify default models for p1/p2, so should be false
		if ok {
			t.Errorf("expected no default model for p1, got %s", model)
		}

		cfgWithDefaults := &config.Config{
			DefaultProvider: "p1",
			Providers: []config.ProviderConfig{
				{ID: "p1", DefaultModel: "modelA"},
				{ID: "p2", DefaultModel: "modelB"},
			},
		}
		reg2 := providers.NewRegistry(cfgWithDefaults, []providers.ProviderAdapter{p1, p2}, nil)
		m1, ok1 := reg2.DefaultModel("p1")
		if !ok1 || m1 != "modelA" {
			t.Errorf("expected p1 to have default modelA, got %s (ok=%v)", m1, ok1)
		}
		m2, ok2 := reg2.DefaultModel("p2")
		if !ok2 || m2 != "modelB" {
			t.Errorf("expected p2 to have default modelB, got %s (ok=%v)", m2, ok2)
		}
		_, okUnknown := reg2.DefaultModel("unknown")
		if okUnknown {
			t.Error("expected no default model for unknown provider")
		}
	})
}
