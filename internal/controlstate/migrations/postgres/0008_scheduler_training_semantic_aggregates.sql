-- +goose Up
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS neighbor_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS latency_p50_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS latency_p90_ms BIGINT NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS latency_stddev_ms DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS output_tokens_p70 BIGINT NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS success_rate DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS timeout_rate DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS coverage_level TEXT NOT NULL DEFAULT 'none';
ALTER TABLE scheduler_training_samples ADD COLUMN IF NOT EXISTS coverage_ratio DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS coverage_ratio;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS coverage_level;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS timeout_rate;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS success_rate;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS output_tokens_p70;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS latency_stddev_ms;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS latency_p90_ms;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS latency_p50_ms;
ALTER TABLE scheduler_training_samples DROP COLUMN IF EXISTS neighbor_count;
