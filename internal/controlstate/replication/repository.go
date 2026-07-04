package replication

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/coordination"
)

type replicatedRepository struct {
	underlying controlstate.Repository
	coord      coordination.Coordinator
	producer   StreamProducer
}

const publishTimeout = 500 * time.Millisecond

func NewRepository(underlying controlstate.Repository, coord coordination.Coordinator, producer StreamProducer) controlstate.Repository {
	return &replicatedRepository{
		underlying: underlying,
		coord:      coord,
		producer:   producer,
	}
}

func (r *replicatedRepository) publish(ctx context.Context, evt ChangeEvent) {
	if r.producer == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if _, err := r.producer.Append(publishCtx, evt); err == nil {
		return
	}

	fallback := r.underlying.FallbackLog()
	if fallback == nil {
		return
	}
	payload, err := json.Marshal(SyncPayload{Event: evt})
	if err != nil {
		return
	}
	now := time.Now().UTC()
	_ = fallback.Insert(ctx, &controlstate.FallbackLogRecord{
		ID:        fmt.Sprintf("sync-publish-%s-%s-%s", evt.Repository, evt.TargetID, now.Format(time.RFC3339Nano)),
		Type:      "sync",
		Payload:   string(payload),
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	})
}

func (r *replicatedRepository) Providers() controlstate.ProviderRepository {
	return &providerRepo{underlying: r.underlying.Providers(), r: r}
}

func (r *replicatedRepository) Combos() controlstate.ComboRepository {
	return &comboRepo{underlying: r.underlying.Combos(), r: r}
}

func (r *replicatedRepository) Routing() controlstate.RoutingRepository {
	return &routingRepo{underlying: r.underlying.Routing(), r: r}
}

func (r *replicatedRepository) APIKeys() controlstate.APIKeyRepository {
	return &apiKeyRepo{underlying: r.underlying.APIKeys(), r: r}
}

func (r *replicatedRepository) Rates() controlstate.RateRepository {
	return &rateRepo{underlying: r.underlying.Rates(), r: r}
}

func (r *replicatedRepository) Usage() controlstate.UsageRepository {
	return &usageRepo{underlying: r.underlying.Usage(), r: r}
}

func (r *replicatedRepository) Audit() controlstate.AuditRepository {
	return &auditRepo{underlying: r.underlying.Audit(), r: r}
}

func (r *replicatedRepository) Idempotency() controlstate.IdempotencyRepository {
	return &idempotencyRepo{underlying: r.underlying.Idempotency(), r: r}
}

func (r *replicatedRepository) SemanticCache() controlstate.SemanticCacheRepository {
	return &semanticCacheRepo{underlying: r.underlying.SemanticCache(), r: r}
}

func (r *replicatedRepository) SemanticRules() controlstate.SemanticRuleStore {
	return &semanticRulesRepo{underlying: r.underlying.SemanticRules(), r: r}
}

func (r *replicatedRepository) FallbackLog() controlstate.FallbackLogRepository {
	return &fallbackLogRepo{underlying: r.underlying.FallbackLog(), r: r}
}

func (r *replicatedRepository) LimitRules() controlstate.LimitRuleRepository {
	return &limitRulesRepo{underlying: r.underlying.LimitRules(), r: r}
}

func (r *replicatedRepository) SessionBlacklist() controlstate.SessionBlacklistRepository {
	return &sessionBlacklistRepo{underlying: r.underlying.SessionBlacklist(), r: r}
}

func (r *replicatedRepository) SchedulerTrainingSamples() controlstate.SchedulerTrainingSampleRepository {
	return r.underlying.SchedulerTrainingSamples()
}

func (r *replicatedRepository) BeginTx(ctx context.Context) (controlstate.Transaction, error) {
	if !r.coord.IsWritable() {
		return nil, ErrWriteNotWritable
	}
	tx, err := r.underlying.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	return &transactionWrapper{underlying: tx, r: r}, nil
}

func (r *replicatedRepository) Settle(ctx context.Context, usage *controlstate.UsageRecord) error {
	if !r.coord.IsWritable() {
		return ErrWriteNotWritable
	}
	err := r.underlying.Settle(ctx, usage)
	if err == nil {
		evt, _ := NewChangeEvent("repository", "SETTLE", usage.ID, usage)
		r.publish(ctx, evt)
	}
	return err
}

func (r *replicatedRepository) Close() error {
	return r.underlying.Close()
}

// Transaction wrapper
type transactionWrapper struct {
	underlying controlstate.Transaction
	r          *replicatedRepository
}

func (t *transactionWrapper) Commit() error {
	err := t.underlying.Commit()
	if err == nil {
		evt, _ := NewChangeEvent("transaction", "COMMIT", "", nil)
		t.r.publish(context.Background(), evt)
	}
	return err
}

func (t *transactionWrapper) Rollback() error {
	return t.underlying.Rollback()
}
