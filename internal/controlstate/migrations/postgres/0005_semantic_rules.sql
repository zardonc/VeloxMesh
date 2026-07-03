-- +goose Up

CREATE TABLE IF NOT EXISTS semantic_rule_global_defaults (
    rule TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT false,
    options_json TEXT NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE TABLE IF NOT EXISTS semantic_rule_user_configs (
    user_id TEXT NOT NULL,
    rule TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT false,
    options_json TEXT NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    PRIMARY KEY (user_id, rule)
);

-- +goose Down

DROP TABLE IF EXISTS semantic_rule_user_configs;
DROP TABLE IF EXISTS semantic_rule_global_defaults;
