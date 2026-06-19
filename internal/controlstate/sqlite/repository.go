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
	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) BeginTx(ctx context.Context) (controlstate.Transaction, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &SqliteTransaction{tx: tx}, nil
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

func (r *Repository) Routing() controlstate.RoutingRepository {
	return &routingRepo{db: r.db}
}

func (r *Repository) APIKeys() controlstate.APIKeyRepository {
	return &apiKeyRepo{db: r.db}
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

// -- other repos simplified for now --

type routingRepo struct{ db *sql.DB }

func (r *routingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error)       { return nil, nil }
func (r *routingRepo) Save(ctx context.Context, config *controlstate.RoutingConfig) error { return nil }

type apiKeyRepo struct{ db *sql.DB }

func (a *apiKeyRepo) GetByHash(ctx context.Context, hash string) (*controlstate.APIKeyRecord, error) {
	return nil, nil
}
func (a *apiKeyRepo) List(ctx context.Context) ([]*controlstate.APIKeyRecord, error)   { return nil, nil }
func (a *apiKeyRepo) Create(ctx context.Context, key *controlstate.APIKeyRecord) error { return nil }
func (a *apiKeyRepo) Update(ctx context.Context, key *controlstate.APIKeyRecord) error { return nil }
func (a *apiKeyRepo) Delete(ctx context.Context, id string) error                      { return nil }

type usageRepo struct{ db *sql.DB }

func (u *usageRepo) Log(ctx context.Context, record *controlstate.UsageRecord) error { return nil }

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
