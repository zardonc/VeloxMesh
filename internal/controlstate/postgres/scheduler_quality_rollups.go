package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
)

const defaultSchedulerQualityRollupLimit = 1000

type schedulerQualityRollupRepo struct {
	pool *pgxpool.Pool
}

func (r *schedulerQualityRollupRepo) Upsert(ctx context.Context, rollup *controlstate.SchedulerQualityRollup) error {
	prepared, err := r.prepare(ctx, rollup)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, upsertSchedulerQualityRollupSQL, qualityRollupValues(prepared)...)
	return err
}

func (r *schedulerQualityRollupRepo) ListByWindow(ctx context.Context, start, end time.Time, schedulerType, schedulerVersion, taskType string, limit int) ([]*controlstate.SchedulerQualityRollup, error) {
	if limit <= 0 {
		limit = defaultSchedulerQualityRollupLimit
	}
	rows, err := r.pool.Query(ctx, selectSchedulerQualityRollupsSQL, start, end, schedulerType, schedulerVersion, taskType, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedulerQualityRollups(rows)
}

func (r *schedulerQualityRollupRepo) prepare(ctx context.Context, incoming *controlstate.SchedulerQualityRollup) (*controlstate.SchedulerQualityRollup, error) {
	current, err := r.get(ctx, incoming)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if current == nil {
		return controlstate.NormalizeSchedulerQualityRollup(incoming), nil
	}
	return controlstate.MergeSchedulerQualityRollups(current, incoming), nil
}

func (r *schedulerQualityRollupRepo) get(ctx context.Context, key *controlstate.SchedulerQualityRollup) (*controlstate.SchedulerQualityRollup, error) {
	row := r.pool.QueryRow(ctx, selectSchedulerQualityRollupSQL, key.BucketStart, key.SchedulerType, key.SchedulerVersion, key.TaskType, key.ModelClass)
	return scanSchedulerQualityRollup(row)
}

func scanSchedulerQualityRollups(rows pgx.Rows) ([]*controlstate.SchedulerQualityRollup, error) {
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
	var sampleIDs []byte
	err := row.Scan(
		&rollup.BucketStart, &rollup.BucketEnd, &rollup.SchedulerType, &rollup.SchedulerVersion,
		&rollup.TaskType, &rollup.ModelClass, &rollup.SampleCount, &rollup.MAPESum,
		&rollup.MAPEAvg, &rollup.WaitMSSum, &rollup.WaitMSAvg,
		&rollup.SchedulerCallLatencyMSSum, &rollup.SchedulerCallLatencyMSAvg,
		&rollup.ErrorCount, &rollup.ConfidenceSum, &rollup.ConfidenceAvg, &sampleIDs,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(sampleIDs, &rollup.SafeSampleIDs)
	return rollup, nil
}

func qualityRollupValues(r *controlstate.SchedulerQualityRollup) []any {
	sampleIDs, _ := json.Marshal(r.SafeSampleIDs)
	return []any{
		r.BucketStart, r.BucketEnd, r.SchedulerType, r.SchedulerVersion, r.TaskType,
		r.ModelClass, r.SampleCount, r.MAPESum, r.MAPEAvg, r.WaitMSSum, r.WaitMSAvg,
		r.SchedulerCallLatencyMSSum, r.SchedulerCallLatencyMSAvg, r.ErrorCount,
		r.ConfidenceSum, r.ConfidenceAvg, sampleIDs,
	}
}

const schedulerQualityRollupColumns = `
	bucket_start, bucket_end, scheduler_type, scheduler_version, task_type, model_class,
	sample_count, mape_sum, mape_avg, wait_ms_sum, wait_ms_avg,
	scheduler_call_latency_ms_sum, scheduler_call_latency_ms_avg,
	error_count, confidence_sum, confidence_avg, safe_sample_ids_json`

const upsertSchedulerQualityRollupSQL = `INSERT INTO scheduler_quality_rollups (` + schedulerQualityRollupColumns + `)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (bucket_start, scheduler_type, scheduler_version, task_type, model_class)
DO UPDATE SET
	bucket_end = EXCLUDED.bucket_end,
	sample_count = EXCLUDED.sample_count,
	mape_sum = EXCLUDED.mape_sum,
	mape_avg = EXCLUDED.mape_avg,
	wait_ms_sum = EXCLUDED.wait_ms_sum,
	wait_ms_avg = EXCLUDED.wait_ms_avg,
	scheduler_call_latency_ms_sum = EXCLUDED.scheduler_call_latency_ms_sum,
	scheduler_call_latency_ms_avg = EXCLUDED.scheduler_call_latency_ms_avg,
	error_count = EXCLUDED.error_count,
	confidence_sum = EXCLUDED.confidence_sum,
	confidence_avg = EXCLUDED.confidence_avg,
	safe_sample_ids_json = EXCLUDED.safe_sample_ids_json`

const selectSchedulerQualityRollupSQL = `SELECT ` + schedulerQualityRollupColumns + `
FROM scheduler_quality_rollups
WHERE bucket_start = $1 AND scheduler_type = $2 AND scheduler_version = $3 AND task_type = $4 AND model_class = $5`

const selectSchedulerQualityRollupsSQL = `SELECT ` + schedulerQualityRollupColumns + `
FROM scheduler_quality_rollups
WHERE bucket_start >= $1 AND bucket_start < $2
  AND ($3 = '' OR scheduler_type = $3)
  AND ($4 = '' OR scheduler_version = $4)
  AND ($5 = '' OR task_type = $5)
ORDER BY bucket_start ASC
LIMIT $6`
