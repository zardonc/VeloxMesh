package providers

import "veloxmesh/internal/config"

type ModelProvider struct {
	ProviderID   string
	ProviderType ProviderType
	DefaultModel bool
	Capabilities CapabilitySet
}

func (mp ModelProvider) Clone() ModelProvider {
	return ModelProvider{
		ProviderID:   mp.ProviderID,
		ProviderType: mp.ProviderType,
		DefaultModel: mp.DefaultModel,
		Capabilities: mp.Capabilities.Clone(),
	}
}

type ModelEntry struct {
	ModelID   string
	Providers []ModelProvider
	Combo     *Combo
}

type Combo struct {
	ID       string
	Name     string
	Strategy string
	Members  []string
	Judge    string
}

type ModelCatalog struct {
	entries map[string]ModelEntry
	models  []string // preserves order of first seen
}

func NewModelCatalog(cfg *config.Config, adapters []ProviderAdapter, combos []Combo) *ModelCatalog {
	c := &ModelCatalog{
		entries: make(map[string]ModelEntry),
	}

	for _, a := range adapters {
		var provCfg *config.ProviderConfig
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == a.ID() {
				provCfg = &cfg.Providers[i]
				break
			}
		}

		caps := a.Capabilities()
		for _, modelID := range a.Models() {
			isDefault := false
			if provCfg != nil && provCfg.DefaultModel == modelID {
				isDefault = true
			}

			mp := ModelProvider{
				ProviderID:   a.ID(),
				ProviderType: caps.ProviderType,
				DefaultModel: isDefault,
				Capabilities: caps.Clone(),
			}

			entry, ok := c.entries[modelID]
			if !ok {
				entry = ModelEntry{ModelID: modelID}
				c.models = append(c.models, modelID)
			}
			entry.Providers = append(entry.Providers, mp)
			c.entries[modelID] = entry
		}
	}

	for _, combo := range combos {
		entry, ok := c.entries[combo.Name]
		if !ok {
			entry = ModelEntry{ModelID: combo.Name}
			c.models = append(c.models, combo.Name)
		}
		comboCopy := combo
		entry.Combo = &comboCopy
		c.entries[combo.Name] = entry
	}

	return c
}

func (c *ModelCatalog) GetAllModels() []string {
	models := make([]string, len(c.models))
	copy(models, c.models)
	return models
}

func (c *ModelCatalog) EligibleProviders(model string, operation Operation) []ModelProvider {
	entry, ok := c.entries[model]
	if !ok {
		return nil
	}

	var eligible []ModelProvider
	for _, p := range entry.Providers {
		if p.Capabilities.SupportsOperation(operation) {
			eligible = append(eligible, p.Clone())
		}
	}
	return eligible
}

func (c *ModelCatalog) ProviderSupports(providerID string, model string, operation Operation) bool {
	entry, ok := c.entries[model]
	if !ok {
		return false
	}
	for _, p := range entry.Providers {
		if p.ProviderID == providerID {
			return p.Capabilities.SupportsOperation(operation)
		}
	}
	return false
}

func (c *ModelCatalog) DefaultModel(providerID string) (string, bool) {
	for _, modelID := range c.models {
		for _, p := range c.entries[modelID].Providers {
			if p.ProviderID == providerID && p.DefaultModel {
				return modelID, true
			}
		}
	}
	return "", false
}

func (c *ModelCatalog) GetCombo(model string) (*Combo, bool) {
	entry, ok := c.entries[model]
	if !ok || entry.Combo == nil {
		return nil, false
	}
	return entry.Combo, true
}
