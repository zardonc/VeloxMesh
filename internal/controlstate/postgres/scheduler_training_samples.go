package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
)

const defaultSchedulerTrainingSampleLimit = 1000

type schedulerTrainingSampleRepo struct {
	pool *pgxpool.Pool
}

func (r *schedulerTrainingSampleRepo) Insert(ctx context.Context, sample *controlstate.SchedulerTrainingSample) error {
	prepared := schedulerTrainingSampleWithCreatedAt(sample, time.Now().UTC())
	_, err := r.pool.Exec(ctx, insertSchedulerTrainingSampleSQL, sampleValues(prepared)...)
	return err
}

func (r *schedulerTrainingSampleRepo) ListByWindow(ctx context.Context, start, end time.Time, limit int) ([]*controlstate.SchedulerTrainingSample, error) {
	if limit <= 0 {
		limit = defaultSchedulerTrainingSampleLimit
	}
	rows, err := r.pool.Query(ctx, selectSchedulerTrainingSamplesSQL, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedulerTrainingSamples(rows)
}

func (r *schedulerTrainingSampleRepo) ListByIDs(ctx context.Context, ids []string) ([]*controlstate.SchedulerTrainingSample, error) {
	if len(ids) == 0 {
		return []*controlstate.SchedulerTrainingSample{}, nil
	}
	rows, err := r.pool.Query(ctx, selectSchedulerTrainingSamplesByIDSQL, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	samples, err := scanSchedulerTrainingSamples(rows)
	if err != nil {
		return nil, err
	}
	return orderTrainingSamplesByID(ids, samples), nil
}

func scanSchedulerTrainingSamples(rows pgx.Rows) ([]*controlstate.SchedulerTrainingSample, error) {
	var samples []*controlstate.SchedulerTrainingSample
	for rows.Next() {
		sample := &controlstate.SchedulerTrainingSample{}
		if err := scanSchedulerTrainingSample(rows, sample); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func scanSchedulerTrainingSample(rows pgx.Rows, s *controlstate.SchedulerTrainingSample) error {
	return rows.Scan(
		&s.ID, &s.TaskID, &s.ModelClass, &s.EstimatedInputTokens, &s.EstimatedOutputTokens,
		&s.Stream, &s.Priority, &s.TimeoutClass, &s.EnqueueTimeMs, &s.RequestKind,
		&s.RouteHint, &s.HasToolCalls, &s.ToolCallDepth, &s.TurnCount, &s.Multimodal,
		&s.QuestionCount, &s.CodeBlockCount, &s.EnumerationHint, &s.InstructionVerbCount,
		&s.MaxSentenceLengthBucket, &s.VocabularyRichnessBucket, &s.ConfidenceHint,
		&s.UncertaintyHint, &s.NeighborCount, &s.LatencyP50Ms, &s.LatencyP90Ms,
		&s.LatencyStddevMs, &s.OutputTokensP70, &s.SuccessRate, &s.TimeoutRate,
		&s.CoverageLevel, &s.CoverageRatio, &s.ActualLatencyMs, &s.InputTokens,
		&s.OutputTokens, &s.Outcome, &s.ProviderClass, &s.SchedulerVersion,
		&s.CompletedAt, &s.CreatedAt,
	)
}

func sampleValues(s *controlstate.SchedulerTrainingSample) []any {
	return []any{
		s.ID, s.TaskID, s.ModelClass, s.EstimatedInputTokens, s.EstimatedOutputTokens,
		s.Stream, s.Priority, s.TimeoutClass, s.EnqueueTimeMs, s.RequestKind,
		s.RouteHint, s.HasToolCalls, s.ToolCallDepth, s.TurnCount, s.Multimodal,
		s.QuestionCount, s.CodeBlockCount, s.EnumerationHint, s.InstructionVerbCount,
		s.MaxSentenceLengthBucket, s.VocabularyRichnessBucket, s.ConfidenceHint,
		s.UncertaintyHint, s.NeighborCount, s.LatencyP50Ms, s.LatencyP90Ms,
		s.LatencyStddevMs, s.OutputTokensP70, s.SuccessRate, s.TimeoutRate,
		s.CoverageLevel, s.CoverageRatio, s.ActualLatencyMs, s.InputTokens,
		s.OutputTokens, s.Outcome, s.ProviderClass, s.SchedulerVersion,
		s.CompletedAt, s.CreatedAt,
	}
}

func schedulerTrainingSampleWithCreatedAt(s *controlstate.SchedulerTrainingSample, fallback time.Time) *controlstate.SchedulerTrainingSample {
	clone := *s
	if clone.CreatedAt.IsZero() {
		clone.CreatedAt = fallback
	}
	return &clone
}

const schedulerTrainingSampleColumns = `
	id, task_id, model_class, estimated_input_tokens, estimated_output_tokens,
	stream, priority, timeout_class, enqueue_time_ms, request_kind, route_hint,
	has_tool_calls, tool_call_depth, turn_count, multimodal, question_count,
	code_block_count, enumeration_hint, instruction_verb_count,
	max_sentence_length_bucket, vocabulary_richness_bucket, confidence_hint,
	uncertainty_hint, neighbor_count, latency_p50_ms, latency_p90_ms,
	latency_stddev_ms, output_tokens_p70, success_rate, timeout_rate,
	coverage_level, coverage_ratio, actual_latency_ms, input_tokens,
	output_tokens, outcome, provider_class, scheduler_version, completed_at,
	created_at`

const insertSchedulerTrainingSampleSQL = `INSERT INTO scheduler_training_samples (` + schedulerTrainingSampleColumns + `)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
	$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31,
	$32, $33, $34, $35, $36, $37, $38, $39, $40)`

const selectSchedulerTrainingSamplesSQL = `SELECT ` + schedulerTrainingSampleColumns + `
FROM scheduler_training_samples
WHERE completed_at >= $1 AND completed_at < $2
ORDER BY completed_at ASC
LIMIT $3`

const selectSchedulerTrainingSamplesByIDSQL = `SELECT ` + schedulerTrainingSampleColumns + `
FROM scheduler_training_samples
WHERE id = ANY($1)`

func orderTrainingSamplesByID(ids []string, samples []*controlstate.SchedulerTrainingSample) []*controlstate.SchedulerTrainingSample {
	byID := make(map[string]*controlstate.SchedulerTrainingSample, len(samples))
	for _, sample := range samples {
		byID[sample.ID] = sample
	}
	out := make([]*controlstate.SchedulerTrainingSample, 0, len(samples))
	for _, id := range ids {
		if sample := byID[id]; sample != nil {
			out = append(out, sample)
		}
	}
	return out
}
