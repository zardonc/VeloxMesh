package providers

import (
	"fmt"
	"veloxmesh/internal/config"
	// We'll import openai dynamically or wire it up in app.go, but for Phase 1 we can just wire it here.
)

type Registry struct {
	providers map[string]ProviderAdapter
	ids       []string
	defaultID string
}

func NewRegistry(cfg *config.Config, adapters ...ProviderAdapter) *Registry {
	r := &Registry{
		providers: make(map[string]ProviderAdapter),
		defaultID: cfg.DefaultProvider,
	}

	for _, a := range adapters {
		r.providers[a.ID()] = a
		r.ids = append(r.ids, a.ID())
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

func (r *Registry) List() []ProviderAdapter {
	list := make([]ProviderAdapter, 0, len(r.ids))
	for _, id := range r.ids {
		list = append(list, r.providers[id])
	}
	return list
}

func (r *Registry) IDs() []string {
	ids := make([]string, len(r.ids))
	copy(ids, r.ids)
	return ids
}

func (r *Registry) GetAllModels() []string {
	var allModels []string
	seen := make(map[string]bool)
	for _, id := range r.ids {
		p := r.providers[id]
		for _, m := range p.Models() {
			if !seen[m] {
				seen[m] = true
				allModels = append(allModels, m)
			}
		}
	}
	return allModels
}

// ProviderCapabilities groups a provider's ID, its supported models, and its capabilities.
type ProviderCapabilities struct {
	ID           string
	Models       []string
	Capabilities CapabilitySet
}

// Capabilities returns the capability set for a specific provider by ID.
func (r *Registry) Capabilities(id string) (CapabilitySet, error) {
	p, err := r.Get(id)
	if err != nil {
		return CapabilitySet{}, err
	}
	return p.Capabilities().Clone(), nil
}

// AllCapabilities returns the capabilities for all registered providers in stable order.
func (r *Registry) AllCapabilities() []ProviderCapabilities {
	caps := make([]ProviderCapabilities, 0, len(r.ids))
	for _, id := range r.ids {
		p := r.providers[id]

		models := p.Models()
		modelsCopy := make([]string, len(models))
		copy(modelsCopy, models)

		caps = append(caps, ProviderCapabilities{
			ID:           id,
			Models:       modelsCopy,
			Capabilities: p.Capabilities().Clone(),
		})
	}
	return caps
}
