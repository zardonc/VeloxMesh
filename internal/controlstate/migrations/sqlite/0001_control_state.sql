-- +goose Up

CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

CREATE TABLE provider_configs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    base_url TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 0,
    models_json TEXT,
    default_model TEXT,
    timeout TEXT,
    weight INTEGER,
    health_config TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE provider_secrets (
    provider_id TEXT PRIMARY KEY REFERENCES provider_configs(id) ON DELETE CASCADE,
    ciphertext BLOB NOT NULL,
    nonce BLOB NOT NULL,
    key_id TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE routing_configs (
    id TEXT PRIMARY KEY,
    strategy TEXT NOT NULL,
    default_provider TEXT,
    fallback_enabled BOOLEAN NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    prefix TEXT NOT NULL,
    hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    credit_balance INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE usage_records (
    id TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    model TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    response_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    target_id TEXT NOT NULL,
    outcome TEXT NOT NULL,
    metadata TEXT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE idempotency_keys (
    key TEXT PRIMARY KEY,
    action_name TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    status TEXT NOT NULL,
    response TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

-- +goose Down

DROP TABLE idempotency_keys;
DROP TABLE audit_events;
DROP TABLE usage_records;
DROP TABLE api_keys;
DROP TABLE routing_configs;
DROP TABLE provider_secrets;
DROP TABLE provider_configs;
DROP TABLE schema_migrations;
