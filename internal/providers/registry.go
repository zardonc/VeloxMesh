package providers

import (
	"fmt"
	"veloxmesh/internal/config"
	// We'll import openai dynamically or wire it up in app.go, but for Phase 1 we can just wire it here.
)

type Registry struct {
	providers map[string]ProviderAdapter
	defaultID string
}

func NewRegistry(cfg *config.Config, adapters ...ProviderAdapter) *Registry {
	r := &Registry{
		providers: make(map[string]ProviderAdapter),
		defaultID: cfg.DefaultProvider,
	}

	for _, a := range adapters {
		r.providers[a.ID()] = a
	}

	return r
}

func (r *Registry) Get(id string) (ProviderAdapter, error) {
	if p, ok := r.providers[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("provider not found: %s", id)
}

func (r *Registry) GetDefault() (ProviderAdapter, error) {
	return r.Get(r.defaultID)
}

func (r *Registry) HasConfiguredProviders() bool {
	return len(r.providers) > 0
}

func (r *Registry) GetAllModels() []string {
	var allModels []string
	seen := make(map[string]bool)
	for _, p := range r.providers {
		for _, m := range p.Models() {
			if !seen[m] {
				seen[m] = true
				allModels = append(allModels, m)
			}
		}
	}
	return allModels
}
