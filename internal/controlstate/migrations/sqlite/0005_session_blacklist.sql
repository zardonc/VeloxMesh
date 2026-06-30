-- +goose Up
CREATE TABLE session_blacklist (
    session_hash TEXT PRIMARY KEY,
    reason TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS session_blacklist;
