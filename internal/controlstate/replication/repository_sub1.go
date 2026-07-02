package replication

import (
	"context"

	"veloxmesh/internal/controlstate"
)

type providerRepo struct {
	underlying controlstate.ProviderRepository
	r          *replicatedRepository
}

func (p *providerRepo) Get(ctx context.Context, id string) (*controlstate.ProviderRecord, error) {
	return p.underlying.Get(ctx, id)
}

func (p *providerRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	return p.underlying.List(ctx, filter)
}

func (p *providerRepo) Create(ctx context.Context, mut *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	if !p.r.coord.IsWritable() {
		return nil, ErrWriteNotWritable
	}
	rec, err := p.underlying.Create(ctx, mut)
	if err == nil {
		evt, _ := NewChangeEvent("providers", "CREATE", rec.ID, mut)
		_, _ = p.r.producer.Append(ctx, evt)
	}
	return rec, err
}

func (p *providerRepo) Update(ctx context.Context, mut *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	if !p.r.coord.IsWritable() {
		return nil, ErrWriteNotWritable
	}
	rec, err := p.underlying.Update(ctx, mut)
	if err == nil {
		evt, _ := NewChangeEvent("providers", "UPDATE", rec.ID, mut)
		_, _ = p.r.producer.Append(ctx, evt)
	}
	return rec, err
}

func (p *providerRepo) Delete(ctx context.Context, id string) error {
	if !p.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := p.underlying.Delete(ctx, id)
	if err == nil {
		evt, _ := NewChangeEvent("providers", "DELETE", id, nil)
		_, _ = p.r.producer.Append(ctx, evt)
	}
	return err
}

func (p *providerRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	return p.underlying.GetEncryptedSecret(ctx, id)
}

func (p *providerRepo) PutEncryptedSecret(ctx context.Context, id string, ciphertext, nonce []byte, keyID string) error {
	if !p.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := p.underlying.PutEncryptedSecret(ctx, id, ciphertext, nonce, keyID)
	if err == nil {
		payload := map[string]interface{}{
			"id": id,
			"ciphertext": ciphertext,
			"nonce": nonce,
			"key_id": keyID,
		}
		evt, _ := NewChangeEvent("providers_secrets", "UPDATE", id, payload)
		_, _ = p.r.producer.Append(ctx, evt)
	}
	return err
}

// Combo Repo
type comboRepo struct {
	underlying controlstate.ComboRepository
	r          *replicatedRepository
}

func (c *comboRepo) Get(ctx context.Context, id string) (*controlstate.ComboRecord, error) {
	return c.underlying.Get(ctx, id)
}
func (c *comboRepo) List(ctx context.Context, filter controlstate.ComboFilter) ([]*controlstate.ComboRecord, error) {
	return c.underlying.List(ctx, filter)
}
func (c *comboRepo) Create(ctx context.Context, mut *controlstate.ComboMutation) (*controlstate.ComboRecord, error) {
	if !c.r.coord.IsWritable() {
		return nil, ErrWriteNotWritable
	}
	rec, err := c.underlying.Create(ctx, mut)
	if err == nil {
		evt, _ := NewChangeEvent("combos", "CREATE", rec.ID, mut)
		_, _ = c.r.producer.Append(ctx, evt)
	}
	return rec, err
}
func (c *comboRepo) Update(ctx context.Context, mut *controlstate.ComboMutation) (*controlstate.ComboRecord, error) {
	if !c.r.coord.IsWritable() {
		return nil, ErrWriteNotWritable
	}
	rec, err := c.underlying.Update(ctx, mut)
	if err == nil {
		evt, _ := NewChangeEvent("combos", "UPDATE", rec.ID, mut)
		_, _ = c.r.producer.Append(ctx, evt)
	}
	return rec, err
}
func (c *comboRepo) Delete(ctx context.Context, id string) error {
	if !c.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := c.underlying.Delete(ctx, id)
	if err == nil {
		evt, _ := NewChangeEvent("combos", "DELETE", id, nil)
		_, _ = c.r.producer.Append(ctx, evt)
	}
	return err
}

// Routing Repo
type routingRepo struct {
	underlying controlstate.RoutingRepository
	r          *replicatedRepository
}

func (ro *routingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error) {
	return ro.underlying.Get(ctx)
}
func (ro *routingRepo) Save(ctx context.Context, config *controlstate.RoutingConfig) error {
	if !ro.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := ro.underlying.Save(ctx, config)
	if err == nil {
		evt, _ := NewChangeEvent("routing", "UPDATE", "", config)
		_, _ = ro.r.producer.Append(ctx, evt)
	}
	return err
}

// API Key Repo
type apiKeyRepo struct {
	underlying controlstate.APIKeyRepository
	r          *replicatedRepository
}

func (a *apiKeyRepo) GetByHash(ctx context.Context, hash string) (*controlstate.APIKeyRecord, error) {
	return a.underlying.GetByHash(ctx, hash)
}
func (a *apiKeyRepo) List(ctx context.Context) ([]*controlstate.APIKeyRecord, error) {
	return a.underlying.List(ctx)
}
func (a *apiKeyRepo) Create(ctx context.Context, key *controlstate.APIKeyRecord) error {
	if !a.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := a.underlying.Create(ctx, key)
	if err == nil {
		evt, _ := NewChangeEvent("api_keys", "CREATE", key.ID, key)
		_, _ = a.r.producer.Append(ctx, evt)
	}
	return err
}
func (a *apiKeyRepo) Update(ctx context.Context, key *controlstate.APIKeyRecord) error {
	if !a.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := a.underlying.Update(ctx, key)
	if err == nil {
		evt, _ := NewChangeEvent("api_keys", "UPDATE", key.ID, key)
		_, _ = a.r.producer.Append(ctx, evt)
	}
	return err
}
func (a *apiKeyRepo) Delete(ctx context.Context, id string) error {
	if !a.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := a.underlying.Delete(ctx, id)
	if err == nil {
		evt, _ := NewChangeEvent("api_keys", "DELETE", id, nil)
		_, _ = a.r.producer.Append(ctx, evt)
	}
	return err
}
