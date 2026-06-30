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

func (r *Repository) SemanticRules() controlstate.SemanticRuleStore {
	return nil // TODO: implement for postgres if needed
}

func (r *Repository) LimitRules() controlstate.LimitRuleRepository {
	return nil // Deferred for Postgres
}

func (r *Repository) SessionBlacklist() controlstate.SessionBlacklistRepository {
	return nil // Deferred for Postgres
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

func (r *Repository) Settle(ctx context.Context, usage *controlstate.UsageRecord) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if usage.Timestamp.IsZero() {
		usage.Timestamp = time.Now().UTC()
	}

	// 1. Get rate
	row := tx.QueryRow(ctx, `
		SELECT input_credit_rate, output_credit_rate
		FROM provider_model_rates
		WHERE provider_id = $1 AND model = $2`, usage.ProviderID, usage.Model)
	var inputRate, outputRate int64
	err = row.Scan(&inputRate, &outputRate)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			usage.Status = controlstate.SettlementStatusMissingRate
			_, err = tx.Exec(ctx, `
				INSERT INTO usage_records (id, api_key_id, provider_id, model, prompt_tokens, response_tokens, total_tokens, duration_ms, timestamp, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				usage.ID, usage.APIKeyID, usage.ProviderID, usage.Model, usage.PromptTokens, usage.ResponseTokens, usage.TotalTokens, usage.DurationMs, usage.Timestamp, usage.Status,
			)
			if err != nil {
				return err
			}
			return tx.Commit(ctx)
		}
		return err
	}

	usage.InputRate = &inputRate
	usage.OutputRate = &outputRate

	credits := (int64(usage.PromptTokens)*inputRate + 999) / 1000
	outCredits := (int64(usage.ResponseTokens)*outputRate + 999) / 1000
	totalCredits := credits + outCredits
	usage.CreditsConsumed = &totalCredits
	usage.Status = controlstate.SettlementStatusSettled

	// 2. Lock and update API key
	if usage.APIKeyID != nil {
		row = tx.QueryRow(ctx, `SELECT credit_balance FROM api_keys WHERE id = $1 FOR UPDATE`, *usage.APIKeyID)
		var balance int64
		if err := row.Scan(&balance); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errors.New("api key not found")
			}
			return err
		}

		balance -= totalCredits

		_, err = tx.Exec(ctx, `UPDATE api_keys SET credit_balance = $1, updated_at = $2 WHERE id = $3`, balance, time.Now().UTC(), *usage.APIKeyID)
		if err != nil {
			return err
		}
	}

	// 3. Insert usage record
	_, err = tx.Exec(ctx, `
		INSERT INTO usage_records (id, api_key_id, provider_id, model, prompt_tokens, response_tokens, total_tokens, duration_ms, timestamp, input_rate, output_rate, credits_consumed, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		usage.ID, usage.APIKeyID, usage.ProviderID, usage.Model, usage.PromptTokens, usage.ResponseTokens, usage.TotalTokens, usage.DurationMs, usage.Timestamp, usage.InputRate, usage.OutputRate, usage.CreditsConsumed, usage.Status,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
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

func (r *Repository) Combos() controlstate.ComboRepository {
	return &comboRepo{pool: r.pool}
}

func (r *Repository) Routing() controlstate.RoutingRepository {
	return &routingRepo{pool: r.pool}
}

func (r *Repository) APIKeys() controlstate.APIKeyRepository {
	return &apiKeyRepo{pool: r.pool}
}

func (r *Repository) Rates() controlstate.RateRepository {
	return &rateRepo{pool: r.pool}
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

func (r *Repository) FallbackLog() controlstate.FallbackLogRepository {
	return &fallbackLogRepo{pool: r.pool}
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

// -- comboRepo --

type comboRepo struct{ pool *pgxpool.Pool }

func (c *comboRepo) Get(ctx context.Context, id string) (*controlstate.ComboRecord, error) {
	row := c.pool.QueryRow(ctx, `
		SELECT id, name, enabled, strategy, members, judge, revision, created_at, updated_at
		FROM combos
		WHERE id = $1`, id)

	rec := &controlstate.ComboRecord{}
	var membersJSON *string
	var judge *string

	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Enabled, &rec.Strategy,
		&membersJSON, &judge,
		&rec.Revision, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if membersJSON != nil {
		_ = json.Unmarshal([]byte(*membersJSON), &rec.Members)
	}
	if judge != nil {
		rec.Judge = *judge
	}

	return rec, nil
}

func (c *comboRepo) List(ctx context.Context, filter controlstate.ComboFilter) ([]*controlstate.ComboRecord, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, name, enabled, strategy, members, judge, revision, created_at, updated_at
		FROM combos
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*controlstate.ComboRecord
	for rows.Next() {
		rec := &controlstate.ComboRecord{}
		var membersJSON *string
		var judge *string

		if err := rows.Scan(
			&rec.ID, &rec.Name, &rec.Enabled, &rec.Strategy,
			&membersJSON, &judge,
			&rec.Revision, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, err
		}

		if filter.Enabled != nil && rec.Enabled != *filter.Enabled {
			continue
		}
		if filter.Search != "" {
			if rec.Name != filter.Search && rec.ID != filter.Search {
				continue
			}
		}

		if membersJSON != nil {
			_ = json.Unmarshal([]byte(*membersJSON), &rec.Members)
		}
		if judge != nil {
			rec.Judge = *judge
		}

		result = append(result, rec)
	}

	return result, nil
}

func (c *comboRepo) Create(ctx context.Context, m *controlstate.ComboMutation) (*controlstate.ComboRecord, error) {
	b, _ := json.Marshal(m.Members)
	membersJSON := string(b)

	var judge *string
	if m.Judge != nil {
		str := *m.Judge
		judge = &str
	}

	_, err := c.pool.Exec(ctx, `
		INSERT INTO combos (id, name, enabled, strategy, members, judge, revision, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 1, NOW(), NOW())`,
		m.ID, m.Name, m.Enabled, m.Strategy, membersJSON, judge,
	)
	if err != nil {
		return nil, err
	}
	return c.Get(ctx, m.ID)
}

func (c *comboRepo) Update(ctx context.Context, m *controlstate.ComboMutation) (*controlstate.ComboRecord, error) {
	if m.Revision == nil {
		return nil, errors.New("optimistic concurrency: missing revision")
	}

	b, _ := json.Marshal(m.Members)
	membersJSON := string(b)

	var judge *string
	if m.Judge != nil {
		str := *m.Judge
		judge = &str
	}

	res, err := c.pool.Exec(ctx, `
		UPDATE combos 
		SET name = $1, enabled = $2, strategy = $3, members = $4, judge = $5, revision = revision + 1, updated_at = NOW()
		WHERE id = $6 AND revision = $7`,
		m.Name, m.Enabled, m.Strategy, membersJSON, judge,
		m.ID, *m.Revision,
	)
	if err != nil {
		return nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, errors.New("optimistic concurrency conflict: record modified or not found")
	}

	return c.Get(ctx, m.ID)
}

func (c *comboRepo) Delete(ctx context.Context, id string) error {
	_, err := c.pool.Exec(ctx, `DELETE FROM combos WHERE id = $1`, id)
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

func (u *usageRepo) Log(ctx context.Context, record *controlstate.UsageRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	if record.Status == "" {
		record.Status = controlstate.SettlementStatusUnsettled
	}
	_, err := u.pool.Exec(ctx, `
		INSERT INTO usage_records (id, api_key_id, provider_id, model, prompt_tokens, response_tokens, total_tokens, duration_ms, timestamp, input_rate, output_rate, credits_consumed, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		record.ID, record.APIKeyID, record.ProviderID, record.Model, record.PromptTokens, record.ResponseTokens, record.TotalTokens, record.DurationMs, record.Timestamp, record.InputRate, record.OutputRate, record.CreditsConsumed, record.Status,
	)
	return err
}

type rateRepo struct{ pool *pgxpool.Pool }

func (r *rateRepo) Save(ctx context.Context, rate *controlstate.ProviderModelRate) error {
	if rate.CreatedAt.IsZero() {
		rate.CreatedAt = time.Now().UTC()
	}
	rate.UpdatedAt = time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		INSERT INTO provider_model_rates (provider_id, model, input_credit_rate, output_credit_rate, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(provider_id, model) DO UPDATE SET
			input_credit_rate=excluded.input_credit_rate,
			output_credit_rate=excluded.output_credit_rate,
			updated_at=excluded.updated_at`,
		rate.ProviderID, rate.Model, rate.InputCreditRate, rate.OutputCreditRate, rate.CreatedAt, rate.UpdatedAt,
	)
	return err
}

func (r *rateRepo) Get(ctx context.Context, providerID, model string) (*controlstate.ProviderModelRate, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT provider_id, model, input_credit_rate, output_credit_rate, created_at, updated_at
		FROM provider_model_rates
		WHERE provider_id = $1 AND model = $2`, providerID, model)
	rate := &controlstate.ProviderModelRate{}
	if err := row.Scan(&rate.ProviderID, &rate.Model, &rate.InputCreditRate, &rate.OutputCreditRate, &rate.CreatedAt, &rate.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rate, nil
}

func (r *rateRepo) Delete(ctx context.Context, providerID, model string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM provider_model_rates WHERE provider_id = $1 AND model = $2`, providerID, model)
	return err
}

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

func (r *Repository) SemanticCache() controlstate.SemanticCacheRepository {
	return &semanticCacheRepo{pool: r.pool}
}

type semanticCacheRepo struct{ pool *pgxpool.Pool }

func (s *semanticCacheRepo) Store(ctx context.Context, entry *controlstate.SemanticCacheEntry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	var usageID *string
	if entry.UsageID != nil {
		str := *entry.UsageID
		usageID = &str
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO semantic_cache_entries (id, scope, model, vector, response, usage_id, hit_count, enabled, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			model=excluded.model,
			vector=excluded.vector,
			response=excluded.response,
			usage_id=excluded.usage_id,
			hit_count=excluded.hit_count,
			enabled=excluded.enabled,
			expires_at=excluded.expires_at`,
		entry.ID, entry.Scope, entry.Model, entry.Vector, entry.Response, usageID, entry.HitCount, entry.Enabled, entry.CreatedAt, entry.ExpiresAt,
	)
	return err
}

func (s *semanticCacheRepo) ListCandidates(ctx context.Context, scope, model string) ([]*controlstate.SemanticCacheEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, scope, model, vector, response, usage_id, hit_count, enabled, created_at, expires_at
		FROM semantic_cache_entries
		WHERE scope = $1 AND model = $2 AND enabled = true AND expires_at > $3
		ORDER BY created_at DESC`, scope, model, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*controlstate.SemanticCacheEntry
	for rows.Next() {
		entry := &controlstate.SemanticCacheEntry{}
		var usageID *string
		if err := rows.Scan(&entry.ID, &entry.Scope, &entry.Model, &entry.Vector, &entry.Response, &usageID, &entry.HitCount, &entry.Enabled, &entry.CreatedAt, &entry.ExpiresAt); err != nil {
			return nil, err
		}
		if usageID != nil {
			entry.UsageID = usageID
		}
		results = append(results, entry)
	}
	return results, rows.Err()
}

func (s *semanticCacheRepo) RecordHit(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE semantic_cache_entries SET hit_count = hit_count + 1 WHERE id = $1`, id)
	return err
}

func (s *semanticCacheRepo) Disable(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE semantic_cache_entries SET enabled = false WHERE id = $1`, id)
	return err
}

type fallbackLogRepo struct{ pool *pgxpool.Pool }

func (f *fallbackLogRepo) Insert(ctx context.Context, record *controlstate.FallbackLogRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	_, err := f.pool.Exec(ctx, `
		INSERT INTO fallback_log (id, type, payload, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		record.ID, record.Type, record.Payload, record.Status, record.CreatedAt, record.UpdatedAt,
	)
	return err
}

func (f *fallbackLogRepo) ListPending(ctx context.Context, limit int) ([]*controlstate.FallbackLogRecord, error) {
	rows, err := f.pool.Query(ctx, `
		SELECT id, type, payload, status, created_at, updated_at
		FROM fallback_log
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*controlstate.FallbackLogRecord
	for rows.Next() {
		rec := &controlstate.FallbackLogRecord{}
		if err := rows.Scan(&rec.ID, &rec.Type, &rec.Payload, &rec.Status, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, rows.Err()
}

func (f *fallbackLogRepo) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := f.pool.Exec(ctx, `
		UPDATE fallback_log 
		SET status = $1, updated_at = $2 
		WHERE id = $3`, status, time.Now().UTC(), id)
	return err
}
