package app

import (
	"context"
	"testing"

	"veloxmesh/internal/controlstate"
)

type dummyRepo struct {
	controlstate.Repository
	prov controlstate.ProviderRepository
}

func (d *dummyRepo) Providers() controlstate.ProviderRepository {
	return d.prov
}

func (d *dummyRepo) Routing() controlstate.RoutingRepository {
	return &dummyRoutingRepo{}
}

type dummyRoutingRepo struct {
}

func (d *dummyRoutingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error) {
	return nil, controlstate.ErrRoutingConfigNotFound
}

func (d *dummyRoutingRepo) Save(ctx context.Context, config *controlstate.RoutingConfig) error {
	return nil
}

type dummyProvRepo struct {
	controlstate.ProviderRepository
	records []*controlstate.ProviderRecord
}

func (d *dummyProvRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	var res []*controlstate.ProviderRecord
	for _, rec := range d.records {
		if filter.Enabled != nil && *filter.Enabled != rec.Enabled {
			continue
		}
		res = append(res, rec)
	}
	return res, nil
}

func (d *dummyProvRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	return []byte("enc"), []byte("nonce"), "key", nil
}

type dummyCipher struct {
	controlstate.SecretCipher
}

func (d *dummyCipher) DecryptProviderSecret(secret *controlstate.EncryptedSecret) ([]byte, error) {
	return []byte("test-key"), nil
}

func TestApp_ReloadProviders(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")

	a, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	repo := &dummyRepo{
		prov: &dummyProvRepo{
			records: []*controlstate.ProviderRecord{
				{
					ID:      "openai-1",
					Type:    "openai-compatible",
					Enabled: true,
					BaseURL: "https://api.openai.com/v1",
					Models:  []string{"gpt-4"},
					Secret:  controlstate.ProviderSecretMetadata{SecretConfigured: true},
				},
			},
		},
	}
	cipher := &dummyCipher{}

	err = a.ReloadProviders(context.Background(), repo, cipher)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Verify the router has the new models
	models := a.RuntimeProviderManager.GetAvailableModels()
	if len(models) != 1 || models[0] != "gpt-4" {
		t.Errorf("expected gpt-4 model, got %v", models)
	}
}
