package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
	"veloxmesh/internal/controlstate"
)

type Repository struct {
	db *sql.DB
}

func Open(dsn string) (*Repository, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := configureConnection(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Repository{db: db}, nil
}

func configureConnection(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA synchronous = NORMAL",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) Migrate(ctx context.Context) error {
	return NewMigrator(r.db).Migrate(ctx)
}

func (r *Repository) BeginTx(ctx context.Context) (controlstate.Transaction, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &SqliteTransaction{tx: tx}, nil
}

func (r *Repository) Settle(ctx context.Context, usage *controlstate.UsageRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if usage.Timestamp.IsZero() {
		usage.Timestamp = time.Now().UTC()
	}

	// 1. Get rate
	row := tx.QueryRowContext(ctx, `
		SELECT input_credit_rate, output_credit_rate
		FROM provider_model_rates
		WHERE provider_id = ? AND model = ?`, usage.ProviderID, usage.Model)
	var inputRate, outputRate int64
	err = row.Scan(&inputRate, &outputRate)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			usage.Status = controlstate.SettlementStatusMissingRate
			_, err = tx.ExecContext(ctx, `
				INSERT INTO usage_records (id, api_key_id, provider_id, model, prompt_tokens, response_tokens, total_tokens, duration_ms, timestamp, status)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				usage.ID, usage.APIKeyID, usage.ProviderID, usage.Model, usage.PromptTokens, usage.ResponseTokens, usage.TotalTokens, usage.DurationMs, usage.Timestamp, usage.Status,
			)
			if err != nil {
				return err
			}
			return tx.Commit()
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
		row = tx.QueryRowContext(ctx, `SELECT credit_balance FROM api_keys WHERE id = ?`, *usage.APIKeyID)
		var balance int64
		if err := row.Scan(&balance); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("api key not found")
			}
			return err
		}

		balance -= totalCredits

		_, err = tx.ExecContext(ctx, `UPDATE api_keys SET credit_balance = ?, updated_at = ? WHERE id = ?`, balance, time.Now().UTC(), *usage.APIKeyID)
		if err != nil {
			return err
		}
	}

	// 3. Insert usage record
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_records (id, api_key_id, provider_id, model, prompt_tokens, response_tokens, total_tokens, duration_ms, timestamp, input_rate, output_rate, credits_consumed, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		usage.ID, usage.APIKeyID, usage.ProviderID, usage.Model, usage.PromptTokens, usage.ResponseTokens, usage.TotalTokens, usage.DurationMs, usage.Timestamp, usage.InputRate, usage.OutputRate, usage.CreditsConsumed, usage.Status,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

type SqliteTransaction struct {
	tx *sql.Tx
}

func (t *SqliteTransaction) Commit() error {
	return t.tx.Commit()
}

func (t *SqliteTransaction) Rollback() error {
	return t.tx.Rollback()
}

func (r *Repository) Providers() controlstate.ProviderRepository {
	return &providerRepo{db: r.db}
}

func (r *Repository) Combos() controlstate.ComboRepository {
	return &comboRepo{db: r.db}
}

func (r *Repository) Routing() controlstate.RoutingRepository {
	return &routingRepo{db: r.db}
}

func (r *Repository) APIKeys() controlstate.APIKeyRepository {
	return &apiKeyRepo{db: r.db}
}

func (r *Repository) Rates() controlstate.RateRepository {
	return &rateRepo{db: r.db}
}

func (r *Repository) Usage() controlstate.UsageRepository {
	return &usageRepo{db: r.db}
}

func (r *Repository) Audit() controlstate.AuditRepository {
	return &auditRepo{db: r.db}
}

func (r *Repository) Idempotency() controlstate.IdempotencyRepository {
	return &idempotencyRepo{db: r.db}
}

func (r *Repository) FallbackLog() controlstate.FallbackLogRepository {
	return &fallbackLogRepo{db: r.db}
}

func (r *Repository) LimitRules() controlstate.LimitRuleRepository {
	return &sqliteLimitRuleRepository{db: r.db}
}

func (r *Repository) SessionBlacklist() controlstate.SessionBlacklistRepository {
	return &sqliteSessionBlacklistRepo{db: r.db}
}

func (r *Repository) SchedulerTrainingSamples() controlstate.SchedulerTrainingSampleRepository {
	return &schedulerTrainingSampleRepo{db: r.db}
}

// -- providerRepo --

type providerRepo struct {
	db *sql.DB
}

func (p *providerRepo) Get(ctx context.Context, id string) (*controlstate.ProviderRecord, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT 
			c.id, c.name, c.type, c.base_url, c.enabled, 
			c.models_json, c.default_model, c.timeout, c.weight, c.health_config, 
			c.revision, c.created_at, c.updated_at,
			s.updated_at AS secret_updated_at
		FROM provider_configs c
		LEFT JOIN provider_secrets s ON c.id = s.provider_id
		WHERE c.id = ?`, id)

	rec := &controlstate.ProviderRecord{}
	var modelsJSON, defaultModel, timeout, healthConfig sql.NullString
	var weight sql.NullInt64
	var secretUpdatedAt sql.NullTime

	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Type, &rec.BaseURL, &rec.Enabled,
		&modelsJSON, &defaultModel, &timeout, &weight, &healthConfig,
		&rec.Revision, &rec.CreatedAt, &rec.UpdatedAt,
		&secretUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // or proper error
		}
		return nil, err
	}

	if modelsJSON.Valid {
		_ = json.Unmarshal([]byte(modelsJSON.String), &rec.Models)
	}
	if defaultModel.Valid {
		rec.DefaultModel = defaultModel.String
	}
	if timeout.Valid {
		rec.Timeout = timeout.String
	}
	if weight.Valid {
		rec.Weight = int(weight.Int64)
	}
	if healthConfig.Valid {
		rec.HealthConfig = []byte(healthConfig.String)
	}

	rec.Secret.SecretConfigured = secretUpdatedAt.Valid
	if secretUpdatedAt.Valid {
		rec.Secret.UpdatedAt = &secretUpdatedAt.Time
	}

	return rec, nil
}

func (p *providerRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	// Minimal implementation for listing
	rows, err := p.db.QueryContext(ctx, `
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
		var modelsJSON, defaultModel, timeout, healthConfig sql.NullString
		var weight sql.NullInt64
		var secretUpdatedAt sql.NullTime

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

		if modelsJSON.Valid {
			_ = json.Unmarshal([]byte(modelsJSON.String), &rec.Models)
		}
		if defaultModel.Valid {
			rec.DefaultModel = defaultModel.String
		}
		if timeout.Valid {
			rec.Timeout = timeout.String
		}
		if weight.Valid {
			rec.Weight = int(weight.Int64)
		}
		if healthConfig.Valid {
			rec.HealthConfig = []byte(healthConfig.String)
		}

		rec.Secret.SecretConfigured = secretUpdatedAt.Valid
		if secretUpdatedAt.Valid {
			rec.Secret.UpdatedAt = &secretUpdatedAt.Time
		}

		result = append(result, rec)
	}

	return result, nil
}

func (p *providerRepo) Create(ctx context.Context, m *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	var modelsJSON sql.NullString
	if len(m.Models) > 0 {
		b, _ := json.Marshal(m.Models)
		modelsJSON.String = string(b)
		modelsJSON.Valid = true
	}

	var healthConfig sql.NullString
	if m.HealthConfig != nil {
		healthConfig.String = string(m.HealthConfig)
		healthConfig.Valid = true
	}

	_, err := p.db.ExecContext(ctx, `
		INSERT INTO provider_configs (id, name, type, base_url, enabled, models_json, default_model, timeout, weight, health_config, revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
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

	var modelsJSON sql.NullString
	if len(m.Models) > 0 {
		b, _ := json.Marshal(m.Models)
		modelsJSON.String = string(b)
		modelsJSON.Valid = true
	}

	var healthConfig sql.NullString
	if m.HealthConfig != nil {
		healthConfig.String = string(m.HealthConfig)
		healthConfig.Valid = true
	}

	res, err := p.db.ExecContext(ctx, `
		UPDATE provider_configs 
		SET name = ?, type = ?, base_url = ?, enabled = ?, models_json = ?, default_model = ?, timeout = ?, weight = ?, health_config = ?, revision = revision + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND revision = ?`,
		m.Name, m.Type, m.BaseURL, m.Enabled, modelsJSON, m.DefaultModel, m.Timeout, m.Weight, healthConfig,
		m.ID, *m.Revision,
	)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, errors.New("optimistic concurrency conflict: record modified or not found")
	}

	return p.Get(ctx, m.ID)
}

func (p *providerRepo) Delete(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM provider_configs WHERE id = ?`, id)
	return err
}

func (p *providerRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	row := p.db.QueryRowContext(ctx, `SELECT ciphertext, nonce, key_id FROM provider_secrets WHERE provider_id = ?`, id)
	var ciphertext, nonce []byte
	var keyID string
	if err := row.Scan(&ciphertext, &nonce, &keyID); err != nil {
		return nil, nil, "", err
	}
	return ciphertext, nonce, keyID, nil
}

func (p *providerRepo) PutEncryptedSecret(ctx context.Context, id string, ciphertext, nonce []byte, keyID string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO provider_secrets (provider_id, ciphertext, nonce, key_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider_id) DO UPDATE SET 
			ciphertext=excluded.ciphertext, 
			nonce=excluded.nonce, 
			key_id=excluded.key_id, 
			updated_at=CURRENT_TIMESTAMP`,
		id, ciphertext, nonce, keyID,
	)
	return err
}

// -- comboRepo --

type comboRepo struct{ db *sql.DB }

func (c *comboRepo) Get(ctx context.Context, id string) (*controlstate.ComboRecord, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT id, name, enabled, strategy, members, judge, revision, created_at, updated_at
		FROM combos
		WHERE id = ?`, id)

	rec := &controlstate.ComboRecord{}
	var membersJSON sql.NullString
	var judge sql.NullString

	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Enabled, &rec.Strategy,
		&membersJSON, &judge,
		&rec.Revision, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if membersJSON.Valid {
		_ = json.Unmarshal([]byte(membersJSON.String), &rec.Members)
	}
	if judge.Valid {
		rec.Judge = judge.String
	}

	return rec, nil
}

func (c *comboRepo) List(ctx context.Context, filter controlstate.ComboFilter) ([]*controlstate.ComboRecord, error) {
	rows, err := c.db.QueryContext(ctx, `
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
		var membersJSON sql.NullString
		var judge sql.NullString

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
			// very naive substring search
			if rec.Name != filter.Search && rec.ID != filter.Search {
				// not a perfect search, but follows basic logic
				continue
			}
		}

		if membersJSON.Valid {
			_ = json.Unmarshal([]byte(membersJSON.String), &rec.Members)
		}
		if judge.Valid {
			rec.Judge = judge.String
		}

		result = append(result, rec)
	}

	return result, nil
}

func (c *comboRepo) Create(ctx context.Context, m *controlstate.ComboMutation) (*controlstate.ComboRecord, error) {
	b, _ := json.Marshal(m.Members)
	membersJSON := string(b)

	var judge sql.NullString
	if m.Judge != nil {
		judge.String = *m.Judge
		judge.Valid = true
	}

	_, err := c.db.ExecContext(ctx, `
		INSERT INTO combos (id, name, enabled, strategy, members, judge, revision, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
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

	var judge sql.NullString
	if m.Judge != nil {
		judge.String = *m.Judge
		judge.Valid = true
	}

	res, err := c.db.ExecContext(ctx, `
		UPDATE combos 
		SET name = ?, enabled = ?, strategy = ?, members = ?, judge = ?, revision = revision + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND revision = ?`,
		m.Name, m.Enabled, m.Strategy, membersJSON, judge,
		m.ID, *m.Revision,
	)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, errors.New("optimistic concurrency conflict: record modified or not found")
	}

	return c.Get(ctx, m.ID)
}

func (c *comboRepo) Delete(ctx context.Context, id string) error {
	_, err := c.db.ExecContext(ctx, `DELETE FROM combos WHERE id = ?`, id)
	return err
}

// -- other repos simplified for now --

type routingRepo struct{ db *sql.DB }

func (r *routingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, strategy, default_provider, fallback_enabled, max_attempts, revision, created_at, updated_at, composite_json
		FROM routing_configs
		WHERE id = 'global'`)

	rec := &controlstate.RoutingConfig{}
	var defaultProvider sql.NullString
	var compositeJSON sql.NullString
	if err := row.Scan(&rec.ID, &rec.Strategy, &defaultProvider, &rec.FallbackEnabled, &rec.MaxAttempts, &rec.Revision, &rec.CreatedAt, &rec.UpdatedAt, &compositeJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, controlstate.ErrRoutingConfigNotFound
		}
		return nil, err
	}
	if defaultProvider.Valid {
		rec.DefaultProvider = defaultProvider.String
	}
	if compositeJSON.Valid && compositeJSON.String != "" {
		var comp controlstate.CompositeRoutingConfig
		if err := json.Unmarshal([]byte(compositeJSON.String), &comp); err != nil {
			return nil, err
		}
		rec.Composite = &comp
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

	var defaultProvider sql.NullString
	if config.DefaultProvider != "" {
		defaultProvider.String = config.DefaultProvider
		defaultProvider.Valid = true
	}

	var compositeJSON sql.NullString
	if config.Composite != nil {
		b, err := json.Marshal(config.Composite)
		if err != nil {
			return err
		}
		compositeJSON.String = string(b)
		compositeJSON.Valid = true
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO routing_configs (id, strategy, default_provider, fallback_enabled, max_attempts, revision, created_at, updated_at, composite_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(id) DO UPDATE SET
			strategy=excluded.strategy,
			default_provider=excluded.default_provider,
			fallback_enabled=excluded.fallback_enabled,
			max_attempts=excluded.max_attempts,
			composite_json=excluded.composite_json,
			revision=routing_configs.revision + 1,
			updated_at=CURRENT_TIMESTAMP`,
		config.ID, config.Strategy, defaultProvider, config.FallbackEnabled, config.MaxAttempts, config.Revision, config.CreatedAt, compositeJSON,
	)
	return err
}

type apiKeyRepo struct{ db *sql.DB }

func (a *apiKeyRepo) GetByHash(ctx context.Context, hash string) (*controlstate.APIKeyRecord, error) {
	row := a.db.QueryRowContext(ctx, `
		SELECT id, prefix, hash, name, role, enabled, credit_balance, created_at, updated_at
		FROM api_keys
		WHERE hash = ?`, hash)
	rec := &controlstate.APIKeyRecord{}
	if err := row.Scan(&rec.ID, &rec.Prefix, &rec.Hash, &rec.Name, &rec.Role, &rec.Enabled, &rec.CreditBalance, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rec, nil
}

func (a *apiKeyRepo) List(ctx context.Context) ([]*controlstate.APIKeyRecord, error) {
	rows, err := a.db.QueryContext(ctx, `
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
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, prefix, hash, name, role, enabled, credit_balance, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.ID, key.Prefix, key.Hash, key.Name, key.Role, key.Enabled, key.CreditBalance, key.CreatedAt, key.UpdatedAt,
	)
	return err
}

func (a *apiKeyRepo) Update(ctx context.Context, key *controlstate.APIKeyRecord) error {
	key.UpdatedAt = time.Now().UTC()
	res, err := a.db.ExecContext(ctx, `
		UPDATE api_keys
		SET name = ?, role = ?, enabled = ?, credit_balance = ?, updated_at = ?
		WHERE id = ?`,
		key.Name, key.Role, key.Enabled, key.CreditBalance, key.UpdatedAt, key.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("api key not found")
	}
	return nil
}

func (a *apiKeyRepo) Delete(ctx context.Context, id string) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	return err
}

type usageRepo struct{ db *sql.DB }

func (u *usageRepo) Log(ctx context.Context, record *controlstate.UsageRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	if record.Status == "" {
		record.Status = controlstate.SettlementStatusUnsettled
	}
	_, err := u.db.ExecContext(ctx, `
		INSERT INTO usage_records (id, api_key_id, provider_id, model, prompt_tokens, response_tokens, total_tokens, duration_ms, timestamp, input_rate, output_rate, credits_consumed, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.APIKeyID, record.ProviderID, record.Model, record.PromptTokens, record.ResponseTokens, record.TotalTokens, record.DurationMs, record.Timestamp, record.InputRate, record.OutputRate, record.CreditsConsumed, record.Status,
	)
	return err
}

type rateRepo struct{ db *sql.DB }

func (r *rateRepo) Save(ctx context.Context, rate *controlstate.ProviderModelRate) error {
	if rate.CreatedAt.IsZero() {
		rate.CreatedAt = time.Now().UTC()
	}
	rate.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO provider_model_rates (provider_id, model, input_credit_rate, output_credit_rate, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_id, model) DO UPDATE SET
			input_credit_rate=excluded.input_credit_rate,
			output_credit_rate=excluded.output_credit_rate,
			updated_at=excluded.updated_at`,
		rate.ProviderID, rate.Model, rate.InputCreditRate, rate.OutputCreditRate, rate.CreatedAt, rate.UpdatedAt,
	)
	return err
}

func (r *rateRepo) Get(ctx context.Context, providerID, model string) (*controlstate.ProviderModelRate, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT provider_id, model, input_credit_rate, output_credit_rate, created_at, updated_at
		FROM provider_model_rates
		WHERE provider_id = ? AND model = ?`, providerID, model)
	rate := &controlstate.ProviderModelRate{}
	if err := row.Scan(&rate.ProviderID, &rate.Model, &rate.InputCreditRate, &rate.OutputCreditRate, &rate.CreatedAt, &rate.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rate, nil
}

func (r *rateRepo) Delete(ctx context.Context, providerID, model string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM provider_model_rates WHERE provider_id = ? AND model = ?`, providerID, model)
	return err
}

type auditRepo struct{ db *sql.DB }

func (a *auditRepo) Log(ctx context.Context, event *controlstate.AuditEvent) error {
	if event.ID == "" {
		event.ID = time.Now().UTC().Format("20060102150405.000000000") + "-" + event.Action
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO audit_events (id, actor, action, target_id, outcome, metadata, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.ID, event.Actor, event.Action, event.TargetID, event.Outcome, string(event.Metadata), event.Timestamp,
	)
	return err
}
func (a *auditRepo) List(ctx context.Context, targetID string) ([]*controlstate.AuditEvent, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, actor, action, target_id, outcome, metadata, timestamp
		FROM audit_events
		WHERE target_id = ?
		ORDER BY timestamp ASC`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*controlstate.AuditEvent
	for rows.Next() {
		event := &controlstate.AuditEvent{}
		var metadata sql.NullString
		if err := rows.Scan(&event.ID, &event.Actor, &event.Action, &event.TargetID, &event.Outcome, &metadata, &event.Timestamp); err != nil {
			return nil, err
		}
		if metadata.Valid {
			event.Metadata = json.RawMessage(metadata.String)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
func (a *auditRepo) PurgeOld(ctx context.Context, beforeTimestamp string) (int64, error) {
	res, err := a.db.ExecContext(ctx, `DELETE FROM audit_events WHERE timestamp < ?`, beforeTimestamp)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type idempotencyRepo struct{ db *sql.DB }

func (i *idempotencyRepo) Get(ctx context.Context, key string) (*controlstate.IdempotencyRecord, error) {
	row := i.db.QueryRowContext(ctx, `
		SELECT key, action_name, fingerprint, status, response, created_at, expires_at
		FROM idempotency_keys
		WHERE key = ?`, key)
	record := &controlstate.IdempotencyRecord{}
	var response sql.NullString
	if err := row.Scan(&record.Key, &record.ActionName, &record.Fingerprint, &record.Status, &response, &record.CreatedAt, &record.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if response.Valid {
		record.Response = response.String
	}
	return record, nil
}
func (i *idempotencyRepo) Save(ctx context.Context, record *controlstate.IdempotencyRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	_, err := i.db.ExecContext(ctx, `
		INSERT INTO idempotency_keys (key, action_name, fingerprint, status, response, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
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

func (r *Repository) DBForTest() *sql.DB {
	return r.db
}

func (r *Repository) SemanticCache() controlstate.SemanticCacheRepository {
	return &semanticCacheRepo{db: r.db}
}

type semanticCacheRepo struct{ db *sql.DB }

func (s *semanticCacheRepo) Store(ctx context.Context, entry *controlstate.SemanticCacheEntry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	var usageID sql.NullString
	if entry.UsageID != nil {
		usageID.String = *entry.UsageID
		usageID.Valid = true
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO semantic_cache_entries (id, scope, model, vector, response, usage_id, hit_count, enabled, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, scope, model, vector, response, usage_id, hit_count, enabled, created_at, expires_at
		FROM semantic_cache_entries
		WHERE scope = ? AND model = ? AND enabled = 1 AND expires_at > ?
		ORDER BY created_at DESC`, scope, model, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*controlstate.SemanticCacheEntry
	for rows.Next() {
		entry := &controlstate.SemanticCacheEntry{}
		var usageID sql.NullString
		if err := rows.Scan(&entry.ID, &entry.Scope, &entry.Model, &entry.Vector, &entry.Response, &usageID, &entry.HitCount, &entry.Enabled, &entry.CreatedAt, &entry.ExpiresAt); err != nil {
			return nil, err
		}
		if usageID.Valid {
			entry.UsageID = &usageID.String
		}
		results = append(results, entry)
	}
	return results, rows.Err()
}

func (s *semanticCacheRepo) RecordHit(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE semantic_cache_entries SET hit_count = hit_count + 1 WHERE id = ?`, id)
	return err
}

func (s *semanticCacheRepo) Disable(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE semantic_cache_entries SET enabled = 0 WHERE id = ?`, id)
	return err
}

type fallbackLogRepo struct{ db *sql.DB }

func (f *fallbackLogRepo) Insert(ctx context.Context, record *controlstate.FallbackLogRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = record.CreatedAt
	}
	_, err := f.db.ExecContext(ctx, `
		INSERT INTO fallback_log (id, type, payload, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		record.ID, record.Type, record.Payload, record.Status, record.CreatedAt, record.UpdatedAt,
	)
	return err
}

func (f *fallbackLogRepo) ListPending(ctx context.Context, limit int) ([]*controlstate.FallbackLogRecord, error) {
	rows, err := f.db.QueryContext(ctx, `
		SELECT id, type, payload, status, created_at, updated_at
		FROM fallback_log
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT ?`, limit)
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
	_, err := f.db.ExecContext(ctx, `
		UPDATE fallback_log 
		SET status = ?, updated_at = ? 
		WHERE id = ?`, status, time.Now().UTC(), id)
	return err
}
