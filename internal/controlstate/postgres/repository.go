package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
)

type Repository struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Repository, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Close() error {
	r.pool.Close()
	return nil
}

func (r *Repository) BeginTx(ctx context.Context) (controlstate.Transaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &PgxTransaction{tx: tx, ctx: ctx}, nil
}

type PgxTransaction struct {
	tx  pgx.Tx
	ctx context.Context
}

func (t *PgxTransaction) Commit() error {
	return t.tx.Commit(t.ctx)
}

func (t *PgxTransaction) Rollback() error {
	return t.tx.Rollback(t.ctx)
}

func (r *Repository) Providers() controlstate.ProviderRepository {
	return &providerRepo{pool: r.pool}
}

func (r *Repository) Routing() controlstate.RoutingRepository {
	return &routingRepo{pool: r.pool}
}

func (r *Repository) APIKeys() controlstate.APIKeyRepository {
	return &apiKeyRepo{pool: r.pool}
}

func (r *Repository) Usage() controlstate.UsageRepository {
	return &usageRepo{pool: r.pool}
}

func (r *Repository) Audit() controlstate.AuditRepository {
	return &auditRepo{pool: r.pool}
}

func (r *Repository) Idempotency() controlstate.IdempotencyRepository {
	return &idempotencyRepo{pool: r.pool}
}

// -- providerRepo --

type providerRepo struct {
	pool *pgxpool.Pool
}

func (p *providerRepo) Get(ctx context.Context, id string) (*controlstate.ProviderRecord, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT 
			c.id, c.name, c.type, c.base_url, c.enabled, 
			c.models_json, c.default_model, c.timeout, c.weight, c.health_config, 
			c.revision, c.created_at, c.updated_at,
			s.updated_at AS secret_updated_at
		FROM provider_configs c
		LEFT JOIN provider_secrets s ON c.id = s.provider_id
		WHERE c.id = $1`, id)

	rec := &controlstate.ProviderRecord{}
	var modelsJSON, defaultModel, timeout, healthConfig *string
	var weight *int
	var secretUpdatedAt *time.Time

	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.Enabled,
		&modelsJSON, &defaultModel, &timeout, &weight, &healthConfig,
		&rec.Revision, &rec.CreatedAt, &rec.UpdatedAt,
		&secretUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // or return proper not found error
		}
		return nil, err
	}

	if modelsJSON != nil {
		_ = json.Unmarshal([]byte(*modelsJSON), &rec.Models)
	}
	if defaultModel != nil {
		rec.DefaultModel = *defaultModel
	}
	if timeout != nil {
		rec.Timeout = *timeout
	}
	if weight != nil {
		rec.Weight = *weight
	}
	if healthConfig != nil {
		rec.HealthConfig = []byte(*healthConfig)
	}

	if secretUpdatedAt != nil {
		rec.Secret.SecretConfigured = true
		rec.Secret.UpdatedAt = secretUpdatedAt
	}

	return rec, nil
}

func (p *providerRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT 
			c.id, c.name, c.type, c.base_url, c.enabled, 
			c.models_json, c.default_model, c.timeout, c.weight, c.health_config, 
			c.revision, c.created_at, c.updated_at,
			s.updated_at AS secret_updated_at
		FROM provider_configs c
		LEFT JOIN provider_secrets s ON c.id = s.provider_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*controlstate.ProviderRecord
	for rows.Next() {
		rec := &controlstate.ProviderRecord{}
		var modelsJSON, defaultModel, timeout, healthConfig *string
		var weight *int
		var secretUpdatedAt *time.Time

		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.Enabled,
			&modelsJSON, &defaultModel, &timeout, &weight, &healthConfig,
			&rec.Revision, &rec.CreatedAt, &rec.UpdatedAt,
			&secretUpdatedAt,
		); err != nil {
			return nil, err
		}

		if filter.Enabled != nil && rec.Enabled != *filter.Enabled {
			continue
		}
		if filter.Type != "" && rec.Type != filter.Type {
			continue
		}

		if modelsJSON != nil {
			_ = json.Unmarshal([]byte(*modelsJSON), &rec.Models)
		}
		if defaultModel != nil {
			rec.DefaultModel = *defaultModel
		}
		if timeout != nil {
			rec.Timeout = *timeout
		}
		if weight != nil {
			rec.Weight = *weight
		}
		if healthConfig != nil {
			rec.HealthConfig = []byte(*healthConfig)
		}

		if secretUpdatedAt != nil {
			rec.Secret.SecretConfigured = true
			rec.Secret.UpdatedAt = secretUpdatedAt
		}

		result = append(result, rec)
	}

	return result, nil
}

func (p *providerRepo) Create(ctx context.Context, m *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	var modelsJSON *string
	if len(m.Models) > 0 {
		b, _ := json.Marshal(m.Models)
		str := string(b)
		modelsJSON = &str
	}

	var healthConfig *string
	if m.HealthConfig != nil {
		str := string(m.HealthConfig)
		healthConfig = &str
	}

	_, err := p.pool.Exec(ctx, `
		INSERT INTO provider_configs (id, name, type, base_url, enabled, models_json, default_model, timeout, weight, health_config, revision)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1)`,
		m.ID, m.Name, m.Type, m.BaseURL, m.Enabled,
		modelsJSON, m.DefaultModel, m.Timeout, m.Weight, healthConfig,
	)
	if err != nil {
		return nil, err
	}
	return p.Get(ctx, m.ID)
}

func (p *providerRepo) Update(ctx context.Context, m *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	if m.Revision == nil {
		return nil, errors.New("optimistic concurrency: missing revision")
	}

	var modelsJSON *string
	if len(m.Models) > 0 {
		b, _ := json.Marshal(m.Models)
		str := string(b)
		modelsJSON = &str
	}

	var healthConfig *string
	if m.HealthConfig != nil {
		str := string(m.HealthConfig)
		healthConfig = &str
	}

	res, err := p.pool.Exec(ctx, `
		UPDATE provider_configs 
		SET name = $1, type = $2, base_url = $3, enabled = $4, models_json = $5, default_model = $6, timeout = $7, weight = $8, health_config = $9, revision = revision + 1, updated_at = NOW()
		WHERE id = $10 AND revision = $11`,
		m.Name, m.Type, m.BaseURL, m.Enabled, modelsJSON, m.DefaultModel, m.Timeout, m.Weight, healthConfig,
		m.ID, *m.Revision,
	)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, errors.New("optimistic concurrency conflict: record modified or not found")
	}

	return p.Get(ctx, m.ID)
}

func (p *providerRepo) Delete(ctx context.Context, id string) error {
	_, err := p.pool.Exec(ctx, `DELETE FROM provider_configs WHERE id = $1`, id)
	return err
}

func (p *providerRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	row := p.pool.QueryRow(ctx, `SELECT ciphertext, nonce, key_id FROM provider_secrets WHERE provider_id = $1`, id)
	var ciphertext, nonce []byte
	var keyID string
	if err := row.Scan(&ciphertext, &nonce, &keyID); err != nil {
		return nil, nil, "", err
	}
	return ciphertext, nonce, keyID, nil
}

func (p *providerRepo) PutEncryptedSecret(ctx context.Context, id string, ciphertext, nonce []byte, keyID string) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO provider_secrets (provider_id, ciphertext, nonce, key_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(provider_id) DO UPDATE SET 
			ciphertext=excluded.ciphertext, 
			nonce=excluded.nonce, 
			key_id=excluded.key_id, 
			updated_at=NOW()`,
		id, ciphertext, nonce, keyID,
	)
	return err
}

// -- other repos simplified --
type routingRepo struct{ pool *pgxpool.Pool }

func (r *routingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, strategy, default_provider, fallback_enabled, max_attempts, revision, created_at, updated_at
		FROM routing_configs
		WHERE id = 'global'`)

	rec := &controlstate.RoutingConfig{}
	var defaultProvider *string
	if err := row.Scan(&rec.ID, &rec.Strategy, &defaultProvider, &rec.FallbackEnabled, &rec.MaxAttempts, &rec.Revision, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, controlstate.ErrRoutingConfigNotFound
		}
		return nil, err
	}
	if defaultProvider != nil {
		rec.DefaultProvider = *defaultProvider
	}
	return rec, nil
}

func (r *routingRepo) Save(ctx context.Context, config *controlstate.RoutingConfig) error {
	if config.ID == "" {
		config.ID = "global"
	}
	if config.CreatedAt.IsZero() {
		config.CreatedAt = time.Now().UTC()
	}

	var defaultProvider *string
	if config.DefaultProvider != "" {
		str := config.DefaultProvider
		defaultProvider = &str
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO routing_configs (id, strategy, default_provider, fallback_enabled, max_attempts, revision, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT(id) DO UPDATE SET
			strategy=excluded.strategy,
			default_provider=excluded.default_provider,
			fallback_enabled=excluded.fallback_enabled,
			max_attempts=excluded.max_attempts,
			revision=routing_configs.revision + 1,
			updated_at=NOW()`,
		config.ID, config.Strategy, defaultProvider, config.FallbackEnabled, config.MaxAttempts, config.Revision, config.CreatedAt,
	)
	return err
}

type apiKeyRepo struct{ pool *pgxpool.Pool }

func (a *apiKeyRepo) GetByHash(ctx context.Context, hash string) (*controlstate.APIKeyRecord, error) {
	row := a.pool.QueryRow(ctx, `
		SELECT id, prefix, hash, name, role, enabled, credit_balance, created_at, updated_at
		FROM api_keys
		WHERE hash = $1`, hash)
	rec := &controlstate.APIKeyRecord{}
	if err := row.Scan(&rec.ID, &rec.Prefix, &rec.Hash, &rec.Name, &rec.Role, &rec.Enabled, &rec.CreditBalance, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

func (a *apiKeyRepo) List(ctx context.Context) ([]*controlstate.APIKeyRecord, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, prefix, hash, name, role, enabled, credit_balance, created_at, updated_at
		FROM api_keys
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*controlstate.APIKeyRecord
	for rows.Next() {
		rec := &controlstate.APIKeyRecord{}
		if err := rows.Scan(&rec.ID, &rec.Prefix, &rec.Hash, &rec.Name, &rec.Role, &rec.Enabled, &rec.CreditBalance, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, rec)
	}
	return result, rows.Err()
}

func (a *apiKeyRepo) Create(ctx context.Context, key *controlstate.APIKeyRecord) error {
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	if key.UpdatedAt.IsZero() {
		key.UpdatedAt = time.Now().UTC()
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO api_keys (id, prefix, hash, name, role, enabled, credit_balance, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		key.ID, key.Prefix, key.Hash, key.Name, key.Role, key.Enabled, key.CreditBalance, key.CreatedAt, key.UpdatedAt,
	)
	return err
}

func (a *apiKeyRepo) Update(ctx context.Context, key *controlstate.APIKeyRecord) error {
	key.UpdatedAt = time.Now().UTC()
	res, err := a.pool.Exec(ctx, `
		UPDATE api_keys
		SET name = $1, role = $2, enabled = $3, credit_balance = $4, updated_at = $5
		WHERE id = $6`,
		key.Name, key.Role, key.Enabled, key.CreditBalance, key.UpdatedAt, key.ID,
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("api key not found")
	}
	return nil
}

func (a *apiKeyRepo) Delete(ctx context.Context, id string) error {
	_, err := a.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	return err
}

type usageRepo struct{ pool *pgxpool.Pool }

func (u *usageRepo) Log(ctx context.Context, record *controlstate.UsageRecord) error { return nil }

type auditRepo struct{ pool *pgxpool.Pool }

func (a *auditRepo) Log(ctx context.Context, event *controlstate.AuditEvent) error {
	if event.ID == "" {
		event.ID = time.Now().UTC().Format("20060102150405.000000000") + "-" + event.Action
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO audit_events (id, actor, action, target_id, outcome, metadata, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		event.ID, event.Actor, event.Action, event.TargetID, event.Outcome, event.Metadata, event.Timestamp,
	)
	return err
}
func (a *auditRepo) List(ctx context.Context, targetID string) ([]*controlstate.AuditEvent, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, actor, action, target_id, outcome, metadata, timestamp
		FROM audit_events
		WHERE target_id = $1
		ORDER BY timestamp ASC`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*controlstate.AuditEvent
	for rows.Next() {
		event := &controlstate.AuditEvent{}
		if err := rows.Scan(&event.ID, &event.Actor, &event.Action, &event.TargetID, &event.Outcome, &event.Metadata, &event.Timestamp); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
func (a *auditRepo) PurgeOld(ctx context.Context, beforeTimestamp string) (int64, error) {
	res, err := a.pool.Exec(ctx, `DELETE FROM audit_events WHERE timestamp < $1`, beforeTimestamp)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

type idempotencyRepo struct{ pool *pgxpool.Pool }

func (i *idempotencyRepo) Get(ctx context.Context, key string) (*controlstate.IdempotencyRecord, error) {
	row := i.pool.QueryRow(ctx, `
		SELECT key, action_name, fingerprint, status, response, created_at, expires_at
		FROM idempotency_keys
		WHERE key = $1`, key)
	record := &controlstate.IdempotencyRecord{}
	var response *string
	if err := row.Scan(&record.Key, &record.ActionName, &record.Fingerprint, &record.Status, &response, &record.CreatedAt, &record.ExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if response != nil {
		record.Response = *response
	}
	return record, nil
}
func (i *idempotencyRepo) Save(ctx context.Context, record *controlstate.IdempotencyRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	_, err := i.pool.Exec(ctx, `
		INSERT INTO idempotency_keys (key, action_name, fingerprint, status, response, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(key) DO UPDATE SET
			action_name=excluded.action_name,
			fingerprint=excluded.fingerprint,
			status=excluded.status,
			response=excluded.response,
			expires_at=excluded.expires_at`,
		record.Key, record.ActionName, record.Fingerprint, record.Status, record.Response, record.CreatedAt, record.ExpiresAt,
	)
	return err
}
