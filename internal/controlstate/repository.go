package controlstate

import (
	"context"
	"errors"
)

var ErrRoutingConfigNotFound = errors.New("routing config not found")

type Repository interface {
	Providers() ProviderRepository
	Routing() RoutingRepository
	APIKeys() APIKeyRepository
	Usage() UsageRepository
	Audit() AuditRepository
	Idempotency() IdempotencyRepository
	BeginTx(ctx context.Context) (Transaction, error)
	Close() error
}

type Transaction interface {
	Commit() error
	Rollback() error
}

type ProviderRepository interface {
	Get(ctx context.Context, id string) (*ProviderRecord, error)
	List(ctx context.Context, filter ProviderFilter) ([]*ProviderRecord, error)
	Create(ctx context.Context, p *ProviderMutation) (*ProviderRecord, error)
	Update(ctx context.Context, p *ProviderMutation) (*ProviderRecord, error)
	Delete(ctx context.Context, id string) error
	GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) // Returns ciphertext, nonce, key_id
	PutEncryptedSecret(ctx context.Context, id string, ciphertext, nonce []byte, keyID string) error
}

type RoutingRepository interface {
	Get(ctx context.Context) (*RoutingConfig, error)
	Save(ctx context.Context, config *RoutingConfig) error
}

type APIKeyRepository interface {
	GetByHash(ctx context.Context, hash string) (*APIKeyRecord, error)
	List(ctx context.Context) ([]*APIKeyRecord, error)
	Create(ctx context.Context, key *APIKeyRecord) error
	Update(ctx context.Context, key *APIKeyRecord) error
	Delete(ctx context.Context, id string) error
}

type UsageRepository interface {
	Log(ctx context.Context, record *UsageRecord) error
}

type AuditRepository interface {
	Log(ctx context.Context, event *AuditEvent) error
	List(ctx context.Context, targetID string) ([]*AuditEvent, error)
	PurgeOld(ctx context.Context, beforeTimestamp string) (int64, error)
}

type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*IdempotencyRecord, error)
	Save(ctx context.Context, record *IdempotencyRecord) error
}

type Migrator interface {
	Migrate(ctx context.Context) error
}
