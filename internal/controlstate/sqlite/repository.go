package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

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

func (a *auditRepo) Log(ctx context.Context, event *controlstate.AuditEvent) error { return nil }
func (a *auditRepo) List(ctx context.Context, targetID string) ([]*controlstate.AuditEvent, error) {
	return nil, nil
}
func (a *auditRepo) PurgeOld(ctx context.Context, beforeTimestamp string) (int64, error) {
	return 0, nil
}

type idempotencyRepo struct{ db *sql.DB }

func (i *idempotencyRepo) Get(ctx context.Context, key string) (*controlstate.IdempotencyRecord, error) {
	return nil, nil
}
func (i *idempotencyRepo) Save(ctx context.Context, record *controlstate.IdempotencyRecord) error {
	return nil
}

func (r *Repository) DBForTest() *sql.DB {
	return r.db
}
