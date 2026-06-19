-- +goose Up

CREATE TABLE schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);

CREATE TABLE provider_configs (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(255) NOT NULL,
    base_url VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    models_json JSONB,
    default_model VARCHAR(255),
    timeout VARCHAR(50),
    weight INT,
    health_config JSONB,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE provider_secrets (
    provider_id VARCHAR(255) PRIMARY KEY REFERENCES provider_configs(id) ON DELETE CASCADE,
    ciphertext BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    key_id VARCHAR(255) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE routing_configs (
    id VARCHAR(255) PRIMARY KEY,
    strategy VARCHAR(255) NOT NULL,
    default_provider VARCHAR(255),
    fallback_enabled BOOLEAN NOT NULL DEFAULT false,
    max_attempts INT NOT NULL DEFAULT 1,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE api_keys (
    id VARCHAR(255) PRIMARY KEY,
    prefix VARCHAR(255) NOT NULL,
    hash VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    role VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE usage_records (
    id VARCHAR(255) PRIMARY KEY,
    provider_id VARCHAR(255) NOT NULL,
    model VARCHAR(255) NOT NULL,
    prompt_tokens INT NOT NULL DEFAULT 0,
    response_tokens INT NOT NULL DEFAULT 0,
    total_tokens INT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE audit_events (
    id VARCHAR(255) PRIMARY KEY,
    actor VARCHAR(255) NOT NULL,
    action VARCHAR(255) NOT NULL,
    target_id VARCHAR(255) NOT NULL,
    outcome VARCHAR(255) NOT NULL,
    metadata JSONB,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE idempotency_keys (
    key VARCHAR(255) PRIMARY KEY,
    status VARCHAR(50) NOT NULL,
    response TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL
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
