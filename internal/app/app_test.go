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

type dummyCipher struct{
	controlstate.SecretCipher
}

func (d *dummyCipher) DecryptProviderSecret(secret *controlstate.EncryptedSecret) ([]byte, error) {
	return []byte("test-key"), nil
}

func TestApp_ReloadProviders(t *testing.T) {
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
