package controlstate

import (
	"context"
	"fmt"

	"veloxmesh/internal/config"
)

type SeedOptions struct {
	Enabled       bool
	EncryptionKey string
}

func SeedFromStaticConfig(ctx context.Context, repo Repository, cfg *config.Config, cipher SecretCipher, options SeedOptions) error {
	if !options.Enabled {
		return nil
	}

	providers, err := repo.Providers().List(ctx, ProviderFilter{})
	if err != nil {
		return fmt.Errorf("failed to check existing durable providers: %w", err)
	}
	if len(providers) > 0 {
		return nil // D-08: If any durable provider exists, do nothing
	}

	for _, p := range cfg.Providers {
		key := p.ResolveAPIKey()
		if key == "" {
			return fmt.Errorf("static provider %s has no api_key or env resolved, skipping seed", p.ID)
		}

		models := p.Models
		var defModel *string
		if p.DefaultModel != "" {
			defModel = &p.DefaultModel
		}

		var timeout *string
		if p.Timeout != "" {
			timeout = &p.Timeout
		}

		weight := p.Weight
		var w *int
		if weight != 0 {
			w = &weight
		}

		m := &ProviderMutation{
			ID:           p.ID,
			Name:         p.ID, // Fallback name
			Type:         p.Type,
			BaseURL:      p.BaseURL,
			Enabled:      true,
			APIKey:       &key,
			Models:       models,
			DefaultModel: defModel,
			Timeout:      timeout,
			Weight:       w,
		}

		errs := ValidateProviderMutation(m, true)
		if len(errs) > 0 {
			return fmt.Errorf("invalid static provider %s: %v", p.ID, errs)
		}

		enc, err := cipher.EncryptProviderSecret([]byte(key))
		if err != nil {
			return fmt.Errorf("failed to encrypt secret for %s: %w", p.ID, err)
		}

		_, err = repo.Providers().Create(ctx, m)
		if err != nil {
			return fmt.Errorf("failed to create provider %s: %w", p.ID, err)
		}

		err = repo.Providers().PutEncryptedSecret(ctx, p.ID, enc.Ciphertext, enc.Nonce, enc.KeyID)
		if err != nil {
			return fmt.Errorf("failed to save secret for %s: %w", p.ID, err)
		}
	}

	return nil
}

// HasDurableProviders is a helper to check if durable providers exist
func HasDurableProviders(ctx context.Context, repo Repository) (bool, error) {
	providers, err := repo.Providers().List(ctx, ProviderFilter{})
	if err != nil {
		return false, err
	}
	return len(providers) > 0, nil
}
