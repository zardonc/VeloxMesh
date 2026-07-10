-- +goose Up
ALTER TABLE scheduler_quality_rollups RENAME TO scheduler_quality_rollups_old;

CREATE TABLE scheduler_quality_rollups (
  bucket_start TEXT NOT NULL,
  bucket_end TEXT NOT NULL,
  scheduler_type TEXT NOT NULL,
  scheduler_version TEXT NOT NULL,
  task_type TEXT NOT NULL,
  model_class TEXT NOT NULL,
  coverage_level TEXT NOT NULL DEFAULT 'none',
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
  anomaly_count INTEGER NOT NULL DEFAULT 0,
  anomaly_rate REAL NOT NULL DEFAULT 0,
  anomaly_unavailable_count INTEGER NOT NULL DEFAULT 0,
  safe_sample_ids_json TEXT NOT NULL,
  PRIMARY KEY (bucket_start, scheduler_type, scheduler_version, task_type, model_class, coverage_level)
);

INSERT INTO scheduler_quality_rollups (
  bucket_start, bucket_end, scheduler_type, scheduler_version, task_type, model_class,
  coverage_level, sample_count, mape_sum, mape_avg, wait_ms_sum, wait_ms_avg,
  scheduler_call_latency_ms_sum, scheduler_call_latency_ms_avg, error_count,
  confidence_sum, confidence_avg, anomaly_count, anomaly_rate, anomaly_unavailable_count,
  safe_sample_ids_json
)
SELECT
  bucket_start, bucket_end, scheduler_type, scheduler_version, task_type, model_class,
  'none', sample_count, mape_sum, mape_avg, wait_ms_sum, wait_ms_avg,
  scheduler_call_latency_ms_sum, scheduler_call_latency_ms_avg, error_count,
  confidence_sum, confidence_avg, 0, 0, 0, safe_sample_ids_json
FROM scheduler_quality_rollups_old;

DROP TABLE scheduler_quality_rollups_old;

CREATE INDEX IF NOT EXISTS idx_scheduler_quality_rollups_window
  ON scheduler_quality_rollups (bucket_start, bucket_end, scheduler_type, scheduler_version, task_type);

-- +goose Down
DROP TABLE IF EXISTS scheduler_quality_rollups;
