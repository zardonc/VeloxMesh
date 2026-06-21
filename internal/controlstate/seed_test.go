package controlstate_test

import (
	"context"
	"testing"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
)

func TestSeedFromStaticConfig(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	migrator := sqlite.NewMigrator(repo.DBForTest())
	_ = migrator.Migrate(ctx)

	key := []byte("01234567890123456789012345678901")
	cipher, _ := controlstate.NewAESGCMSecretCipher(key, "v1")

	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{
				ID:           "test-static",
				Type:         "openai-compatible",
				BaseURL:      "https://api.test/v1",
				APIKey:       "sk-static-key",
				Models:       []string{"m1"},
				DefaultModel: "m1",
			},
		},
	}

	opts := controlstate.SeedOptions{
		Enabled:       true,
		EncryptionKey: string(key),
	}

	// 1. Should seed if empty
	err = controlstate.SeedFromStaticConfig(ctx, repo, cfg, cipher, opts)
	if err != nil {
		t.Fatalf("Seed failed: %v", err)
	}

	provs, _ := repo.Providers().List(ctx, controlstate.ProviderFilter{})
	if len(provs) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(provs))
	}

	// 2. Second seed should do nothing because durable providers exist
	cfg.Providers[0].BaseURL = "https://api.test/v2" // try to overwrite
	err = controlstate.SeedFromStaticConfig(ctx, repo, cfg, cipher, opts)
	if err != nil {
		t.Fatalf("Second seed failed: %v", err)
	}

	provs2, _ := repo.Providers().List(ctx, controlstate.ProviderFilter{})
	if provs2[0].BaseURL != "https://api.test/v1" {
		t.Errorf("Durable record was overwritten by static config")
	}

	// 3. Seed disabled should do nothing
	repo2, _ := sqlite.Open("file:test2?mode=memory&cache=shared")
	defer repo2.Close()
	migrator2 := sqlite.NewMigrator(repo2.DBForTest())
	_ = migrator2.Migrate(ctx)

	opts.Enabled = false
	err = controlstate.SeedFromStaticConfig(ctx, repo2, cfg, cipher, opts)
	if err != nil {
		t.Fatalf("Seed disabled failed: %v", err)
	}
	provs3, _ := repo2.Providers().List(ctx, controlstate.ProviderFilter{})
	if len(provs3) != 0 {
		t.Errorf("Expected 0 providers when seed disabled, got %d", len(provs3))
	}
}
