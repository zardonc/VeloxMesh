-- +goose Up
ALTER TABLE scheduler_training_samples ADD COLUMN neighbor_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN latency_p50_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN latency_p90_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN latency_stddev_ms REAL NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN output_tokens_p70 INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN success_rate REAL NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN timeout_rate REAL NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN coverage_level TEXT NOT NULL DEFAULT 'none';
ALTER TABLE scheduler_training_samples ADD COLUMN coverage_ratio REAL NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot drop columns safely without rebuilding the table; leave existing data intact.
