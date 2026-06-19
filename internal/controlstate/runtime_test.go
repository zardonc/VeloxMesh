package controlstate

import (
	"context"
	"testing"

	"veloxmesh/internal/config"
	gwErr "veloxmesh/internal/errors"
	"veloxmesh/internal/providers"
)

func TestBuildProviderAdapters_Success(t *testing.T) {
	records := []*ProviderRecord{
		{
			ID:      "openai-1",
			Type:    "openai-compatible",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4"},
		},
		{
			ID:      "anthropic-1",
			Type:    "anthropic",
			Enabled: true,
			BaseURL: "https://api.anthropic.com",
			Models:  []string{"claude-3"},
		},
		{
			ID:      "gemini-1",
			Type:    "gemini",
			Enabled: true,
			BaseURL: "",
			Models:  []string{"gemini-1.5"},
		},
		{
			ID:      "disabled-1",
			Type:    "openai-compatible",
			Enabled: false,
			Models:  []string{"gpt-3.5"},
		},
	}

	secrets := map[string]string{
		"openai-1":    "sk-openai",
		"anthropic-1": "sk-anthropic",
		"gemini-1":    "sk-gemini",
	}

	adapters, err := BuildProviderAdapters(records, secrets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(adapters) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(adapters))
	}

	// Verify disabled is excluded
	for _, a := range adapters {
		if a.ID() == "disabled-1" {
			t.Errorf("disabled provider was included")
		}
	}
}

func TestBuildProviderAdapters_NoActiveProviders(t *testing.T) {
	records := []*ProviderRecord{
		{
			ID:      "disabled-1",
			Type:    "openai-compatible",
			Enabled: false,
			Models:  []string{"gpt-3.5"},
		},
	}

	adapters, err := BuildProviderAdapters(records, nil)
	if err != nil {
		t.Fatalf("expected no error for empty adapters, got %v", err)
	}
	if len(adapters) != 0 {
		t.Errorf("expected 0 adapters, got %d", len(adapters))
	}
}

func TestBuildProviderAdapters_MissingSecret(t *testing.T) {
	records := []*ProviderRecord{
		{
			ID:      "openai-1",
			Type:    "openai-compatible",
			Enabled: true,
			Models:  []string{"gpt-4"},
		},
	}

	secrets := map[string]string{} // Missing secret

	_, err := BuildProviderAdapters(records, secrets)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	gwErr, ok := err.(*gwErr.GatewayError)
	if !ok {
		t.Fatalf("expected GatewayError, got %T", err)
	}

	if gwErr.Code != "missing_provider_secret" {
		t.Errorf("expected missing_provider_secret, got %s", gwErr.Code)
	}
}

func TestBuildProviderAdapters_MissingModels(t *testing.T) {
	records := []*ProviderRecord{
		{
			ID:      "openai-1",
			Type:    "openai-compatible",
			Enabled: true,
			Models:  []string{}, // Missing models
		},
	}

	secrets := map[string]string{
		"openai-1": "sk-openai",
	}

	_, err := BuildProviderAdapters(records, secrets)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	gwErr, ok := err.(*gwErr.GatewayError)
	if !ok {
		t.Fatalf("expected GatewayError, got %T", err)
	}

	if gwErr.Code != "missing_provider_model_config" {
		t.Errorf("expected missing_provider_model_config, got %s", gwErr.Code)
	}
}

// Dummy repo for testing LoadActiveProviderRecords
type testProviderRepo struct {
	ProviderRepository
	records []*ProviderRecord
}

func (r *testProviderRepo) List(ctx context.Context, filter ProviderFilter) ([]*ProviderRecord, error) {
	var res []*ProviderRecord
	for _, rec := range r.records {
		if filter.Enabled != nil && *filter.Enabled != rec.Enabled {
			continue
		}
		res = append(res, rec)
	}
	return res, nil
}

func TestLoadActiveProviderRecords(t *testing.T) {
	repo := &testProviderRepo{
		records: []*ProviderRecord{
			{ID: "p1", Enabled: true},
			{ID: "p2", Enabled: false},
		},
	}

	res, err := LoadActiveProviderRecords(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 || res[0].ID != "p1" {
		t.Errorf("expected only p1 to be loaded, got %v", res)
	}
}

// mock validator
func mockValidator(ctx context.Context, adapters []providers.ProviderAdapter) error {
	return nil
}

func TestRuntimeProviderManager_ActivateProviderSet(t *testing.T) {
	cfg := &config.Config{
		RoutingStrategy: "round-robin",
	}
	manager := NewRuntimeProviderManager(cfg, nil, nil)

	if snap := manager.Snapshot(); snap != nil && snap.Router != nil {
		t.Fatal("expected initially empty router")
	}

	records := []*ProviderRecord{
		{
			ID:      "openai-1",
			Type:    "openai-compatible",
			Enabled: true,
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-4"},
		},
	}
	secrets := map[string]string{"openai-1": "test-key"}

	err := manager.ActivateProviderSet(context.Background(), records, secrets, mockValidator)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := manager.Snapshot()
	if snap == nil || snap.Router == nil {
		t.Fatal("expected router to be populated")
	}

	models := manager.GetAvailableModels()
	if len(models) != 1 || models[0] != "gpt-4" {
		t.Errorf("expected gpt-4 model, got %v", models)
	}

	// Test activation preserves old state on failure
	err = manager.ActivateProviderSet(context.Background(), records, nil, mockValidator) // nil secrets should cause failure
	if err == nil {
		t.Fatal("expected activation to fail due to missing secrets")
	}

	snap2 := manager.Snapshot()
	if snap2 != snap {
		t.Fatal("expected snapshot to remain unchanged on failure")
	}
}

func TestRuntimeProviderManager_Static(t *testing.T) {
	cfg := &config.Config{
		RoutingStrategy: "round-robin",
	}
	manager := NewRuntimeProviderManager(cfg, nil, nil)

	err := manager.ActivateStatic([]config.ProviderConfig{}, []providers.ProviderAdapter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
