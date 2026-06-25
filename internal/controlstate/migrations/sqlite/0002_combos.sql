CREATE TABLE combos (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    strategy TEXT NOT NULL,
    members TEXT NOT NULL,
    judge TEXT,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_combos_name ON combos(name);
CREATE INDEX idx_combos_enabled ON combos(enabled);
