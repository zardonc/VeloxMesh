package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"veloxmesh/internal/controlstate"
)

const defaultSchedulerQualityRollupLimit = 1000

type schedulerQualityRollupRepo struct {
	db *sql.DB
}

func (r *schedulerQualityRollupRepo) Upsert(ctx context.Context, rollup *controlstate.SchedulerQualityRollup) error {
	prepared, err := r.prepare(ctx, rollup)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, replaceSchedulerQualityRollupSQL, qualityRollupValues(prepared)...)
	return err
}

func (r *schedulerQualityRollupRepo) ListByWindow(ctx context.Context, start, end time.Time, schedulerType, schedulerVersion, taskType string, limit int) ([]*controlstate.SchedulerQualityRollup, error) {
	if limit <= 0 {
		limit = defaultSchedulerQualityRollupLimit
	}
	rows, err := r.db.QueryContext(ctx, selectSchedulerQualityRollupsSQL, qualityTime(start), qualityTime(end), schedulerType, schedulerType, schedulerVersion, schedulerVersion, taskType, taskType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedulerQualityRollups(rows)
}

func (r *schedulerQualityRollupRepo) prepare(ctx context.Context, incoming *controlstate.SchedulerQualityRollup) (*controlstate.SchedulerQualityRollup, error) {
	current, err := r.get(ctx, incoming)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if current == nil {
		return controlstate.NormalizeSchedulerQualityRollup(incoming), nil
	}
	return controlstate.MergeSchedulerQualityRollups(current, incoming), nil
}

func (r *schedulerQualityRollupRepo) get(ctx context.Context, key *controlstate.SchedulerQualityRollup) (*controlstate.SchedulerQualityRollup, error) {
	row := r.db.QueryRowContext(ctx, selectSchedulerQualityRollupSQL, qualityTime(key.BucketStart), key.SchedulerType, key.SchedulerVersion, key.TaskType, key.ModelClass, key.CoverageLevel)
	return scanSchedulerQualityRollup(row)
}

func scanSchedulerQualityRollups(rows *sql.Rows) ([]*controlstate.SchedulerQualityRollup, error) {
	var rollups []*controlstate.SchedulerQualityRollup
	for rows.Next() {
		rollup, err := scanSchedulerQualityRollup(rows)
		if err != nil {
			return nil, err
		}
		rollups = append(rollups, rollup)
	}
	return rollups, rows.Err()
}

type qualityRollupScanner interface {
	Scan(dest ...any) error
}

func scanSchedulerQualityRollup(row qualityRollupScanner) (*controlstate.SchedulerQualityRollup, error) {
	rollup := &controlstate.SchedulerQualityRollup{}
	var bucketStart, bucketEnd, sampleIDs string
	err := row.Scan(
		&bucketStart, &bucketEnd, &rollup.SchedulerType, &rollup.SchedulerVersion,
		&rollup.TaskType, &rollup.ModelClass, &rollup.CoverageLevel, &rollup.SampleCount,
		&rollup.MAPESum, &rollup.MAPEAvg, &rollup.WaitMSSum, &rollup.WaitMSAvg,
		&rollup.SchedulerCallLatencyMSSum, &rollup.SchedulerCallLatencyMSAvg,
		&rollup.ErrorCount, &rollup.ConfidenceSum, &rollup.ConfidenceAvg,
		&rollup.AnomalyCount, &rollup.AnomalyRate, &rollup.AnomalyUnavailableCount, &sampleIDs,
	)
	if err != nil {
		return nil, err
	}
	rollup.BucketStart = parseQualityTime(bucketStart)
	rollup.BucketEnd = parseQualityTime(bucketEnd)
	_ = json.Unmarshal([]byte(sampleIDs), &rollup.SafeSampleIDs)
	return rollup, nil
}

func qualityRollupValues(r *controlstate.SchedulerQualityRollup) []any {
	sampleIDs, _ := json.Marshal(r.SafeSampleIDs)
	return []any{
		qualityTime(r.BucketStart), qualityTime(r.BucketEnd), r.SchedulerType, r.SchedulerVersion, r.TaskType,
		r.ModelClass, r.CoverageLevel, r.SampleCount, r.MAPESum, r.MAPEAvg, r.WaitMSSum, r.WaitMSAvg,
		r.SchedulerCallLatencyMSSum, r.SchedulerCallLatencyMSAvg, r.ErrorCount,
		r.ConfidenceSum, r.ConfidenceAvg, r.AnomalyCount, r.AnomalyRate, r.AnomalyUnavailableCount, string(sampleIDs),
	}
}

func qualityTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseQualityTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

const schedulerQualityRollupColumns = `
	bucket_start, bucket_end, scheduler_type, scheduler_version, task_type, model_class, coverage_level,
	sample_count, mape_sum, mape_avg, wait_ms_sum, wait_ms_avg,
	scheduler_call_latency_ms_sum, scheduler_call_latency_ms_avg,
	error_count, confidence_sum, confidence_avg,
	anomaly_count, anomaly_rate, anomaly_unavailable_count, safe_sample_ids_json`

const replaceSchedulerQualityRollupSQL = `INSERT OR REPLACE INTO scheduler_quality_rollups (` + schedulerQualityRollupColumns + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const selectSchedulerQualityRollupSQL = `SELECT ` + schedulerQualityRollupColumns + `
FROM scheduler_quality_rollups
WHERE bucket_start = ? AND scheduler_type = ? AND scheduler_version = ? AND task_type = ? AND model_class = ? AND coverage_level = ?`

const selectSchedulerQualityRollupsSQL = `SELECT ` + schedulerQualityRollupColumns + `
FROM scheduler_quality_rollups
WHERE bucket_start >= ? AND bucket_start < ?
  AND (? = '' OR scheduler_type = ?)
  AND (? = '' OR scheduler_version = ?)
  AND (? = '' OR task_type = ?)
ORDER BY bucket_start ASC
LIMIT ?`
