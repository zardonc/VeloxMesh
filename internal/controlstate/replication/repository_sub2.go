package replication

import (
	"context"

	"veloxmesh/internal/controlstate"
)

// Rate Repo
type rateRepo struct {
	underlying controlstate.RateRepository
	r          *replicatedRepository
}

func (r *rateRepo) Save(ctx context.Context, rate *controlstate.ProviderModelRate) error {
	if !r.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := r.underlying.Save(ctx, rate)
	if err == nil {
		evt, _ := NewChangeEvent("rates", "UPDATE", rate.ProviderID+":"+rate.Model, rate)
		r.r.publish(ctx, evt)
	}
	return err
}
func (r *rateRepo) Get(ctx context.Context, providerID, model string) (*controlstate.ProviderModelRate, error) {
	return r.underlying.Get(ctx, providerID, model)
}
func (r *rateRepo) Delete(ctx context.Context, providerID, model string) error {
	if !r.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := r.underlying.Delete(ctx, providerID, model)
	if err == nil {
		evt, _ := NewChangeEvent("rates", "DELETE", providerID+":"+model, nil)
		r.r.publish(ctx, evt)
	}
	return err
}

// Usage Repo
type usageRepo struct {
	underlying controlstate.UsageRepository
	r          *replicatedRepository
}

func (u *usageRepo) Log(ctx context.Context, record *controlstate.UsageRecord) error {
	if !u.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := u.underlying.Log(ctx, record)
	if err == nil {
		evt, _ := NewChangeEvent("usage", "LOG", record.ID, record)
		u.r.publish(ctx, evt)
	}
	return err
}

// Audit Repo
type auditRepo struct {
	underlying controlstate.AuditRepository
	r          *replicatedRepository
}

func (a *auditRepo) Log(ctx context.Context, event *controlstate.AuditEvent) error {
	if !a.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := a.underlying.Log(ctx, event)
	if err == nil {
		evt, _ := NewChangeEvent("audit", "LOG", event.ID, event)
		a.r.publish(ctx, evt)
	}
	return err
}
func (a *auditRepo) List(ctx context.Context, targetID string) ([]*controlstate.AuditEvent, error) {
	return a.underlying.List(ctx, targetID)
}
func (a *auditRepo) PurgeOld(ctx context.Context, beforeTimestamp string) (int64, error) {
	if !a.r.coord.IsWritable() {
		return 0, ErrWriteNotWritable
	}
	n, err := a.underlying.PurgeOld(ctx, beforeTimestamp)
	if err == nil {
		evt, _ := NewChangeEvent("audit", "PURGE", beforeTimestamp, nil)
		a.r.publish(ctx, evt)
	}
	return n, err
}

// Idempotency Repo
type idempotencyRepo struct {
	underlying controlstate.IdempotencyRepository
	r          *replicatedRepository
}

func (i *idempotencyRepo) Get(ctx context.Context, key string) (*controlstate.IdempotencyRecord, error) {
	return i.underlying.Get(ctx, key)
}
func (i *idempotencyRepo) Save(ctx context.Context, record *controlstate.IdempotencyRecord) error {
	if !i.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := i.underlying.Save(ctx, record)
	if err == nil {
		evt, _ := NewChangeEvent("idempotency", "CREATE", record.Key, record)
		i.r.publish(ctx, evt)
	}
	return err
}

// SemanticCache Repo
type semanticCacheRepo struct {
	underlying controlstate.SemanticCacheRepository
	r          *replicatedRepository
}

func (s *semanticCacheRepo) Store(ctx context.Context, entry *controlstate.SemanticCacheEntry) error {
	if !s.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := s.underlying.Store(ctx, entry)
	if err == nil {
		evt, _ := NewChangeEvent("semantic_cache", "CREATE", entry.ID, entry)
		s.r.publish(ctx, evt)
	}
	return err
}
func (s *semanticCacheRepo) ListCandidates(ctx context.Context, scope, model string) ([]*controlstate.SemanticCacheEntry, error) {
	return s.underlying.ListCandidates(ctx, scope, model)
}
func (s *semanticCacheRepo) RecordHit(ctx context.Context, id string) error {
	if !s.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := s.underlying.RecordHit(ctx, id)
	if err == nil {
		evt, _ := NewChangeEvent("semantic_cache", "UPDATE", id, nil) // update hits
		s.r.publish(ctx, evt)
	}
	return err
}
func (s *semanticCacheRepo) Disable(ctx context.Context, id string) error {
	if !s.r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := s.underlying.Disable(ctx, id)
	if err == nil {
		evt, _ := NewChangeEvent("semantic_cache", "DELETE", id, nil) // logic disable
		s.r.publish(ctx, evt)
	}
	return err
}
