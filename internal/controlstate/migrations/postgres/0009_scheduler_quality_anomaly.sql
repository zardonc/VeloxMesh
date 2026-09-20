-- +goose Up
ALTER TABLE scheduler_quality_rollups
  ADD COLUMN IF NOT EXISTS coverage_level TEXT NOT NULL DEFAULT 'none',
  ADD COLUMN IF NOT EXISTS anomaly_count BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS anomaly_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS anomaly_unavailable_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE scheduler_quality_rollups
  DROP CONSTRAINT IF EXISTS scheduler_quality_rollups_pkey;

ALTER TABLE scheduler_quality_rollups
  ADD PRIMARY KEY (bucket_start, scheduler_type, scheduler_version, task_type, model_class, coverage_level);

-- +goose Down
ALTER TABLE scheduler_quality_rollups
  DROP CONSTRAINT IF EXISTS scheduler_quality_rollups_pkey;

ALTER TABLE scheduler_quality_rollups
  ADD PRIMARY KEY (bucket_start, scheduler_type, scheduler_version, task_type, model_class);

ALTER TABLE scheduler_quality_rollups
  DROP COLUMN IF EXISTS anomaly_unavailable_count,
  DROP COLUMN IF EXISTS anomaly_rate,
  DROP COLUMN IF EXISTS anomaly_count,
  DROP COLUMN IF EXISTS coverage_level;
