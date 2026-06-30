-- +migrate Up
CREATE TABLE limit_rules (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    target_id TEXT NOT NULL,
    dimension TEXT NOT NULL,
    window TEXT NOT NULL,
    limit_val INTEGER NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_limit_rules_scope_target ON limit_rules (scope, target_id);

-- +migrate Down
DROP TABLE IF EXISTS limit_rules;
