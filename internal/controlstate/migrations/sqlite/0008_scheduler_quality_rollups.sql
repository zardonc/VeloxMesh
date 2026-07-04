-- +goose Up
CREATE TABLE IF NOT EXISTS scheduler_quality_rollups (
  bucket_start TEXT NOT NULL,
  bucket_end TEXT NOT NULL,
  scheduler_type TEXT NOT NULL,
  scheduler_version TEXT NOT NULL,
  task_type TEXT NOT NULL,
  model_class TEXT NOT NULL,
  sample_count INTEGER NOT NULL,
  mape_sum REAL NOT NULL,
  mape_avg REAL NOT NULL,
  wait_ms_sum REAL NOT NULL,
  wait_ms_avg REAL NOT NULL,
  scheduler_call_latency_ms_sum REAL NOT NULL,
  scheduler_call_latency_ms_avg REAL NOT NULL,
  error_count INTEGER NOT NULL,
  confidence_sum REAL NOT NULL,
  confidence_avg REAL NOT NULL,
  safe_sample_ids_json TEXT NOT NULL,
  PRIMARY KEY (bucket_start, scheduler_type, scheduler_version, task_type, model_class)
);

CREATE INDEX IF NOT EXISTS idx_scheduler_quality_rollups_window
  ON scheduler_quality_rollups (bucket_start, bucket_end, scheduler_type, scheduler_version, task_type);

-- +goose Down
DROP TABLE IF EXISTS scheduler_quality_rollups;
