CREATE TABLE IF NOT EXISTS limit_rules (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    target_id TEXT NOT NULL,
    dimension TEXT NOT NULL,
    "window" TEXT NOT NULL,
    limit_val BIGINT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_limit_rules_scope_target ON limit_rules (scope, target_id);

CREATE TABLE IF NOT EXISTS session_blacklist (
    session_hash TEXT PRIMARY KEY,
    reason TEXT NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);
