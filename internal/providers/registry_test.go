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

func (m *mockAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}

func TestRegistry(t *testing.T) {
	cfg := &config.Config{DefaultProvider: "p1"}
	p1 := &mockAdapter{id: "p1", models: []string{"modelA", "modelB"}}
	p2 := &mockAdapter{id: "p2", models: []string{"modelB", "modelC"}}

	registry := providers.NewRegistry(cfg, p1, p2)

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
}
