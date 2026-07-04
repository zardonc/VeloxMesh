package sqlite

import (
	"context"
	"database/sql"
	"time"

	"veloxmesh/internal/controlstate"
)

const defaultSchedulerTrainingSampleLimit = 1000

type schedulerTrainingSampleRepo struct {
	db *sql.DB
}

func (r *schedulerTrainingSampleRepo) Insert(ctx context.Context, sample *controlstate.SchedulerTrainingSample) error {
	if sample.CreatedAt.IsZero() {
		sample.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, insertSchedulerTrainingSampleSQL, sampleValues(sample)...)
	return err
}

func (r *schedulerTrainingSampleRepo) ListByWindow(ctx context.Context, start, end time.Time, limit int) ([]*controlstate.SchedulerTrainingSample, error) {
	if limit <= 0 {
		limit = defaultSchedulerTrainingSampleLimit
	}
	rows, err := r.db.QueryContext(ctx, selectSchedulerTrainingSamplesSQL, start, end, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedulerTrainingSamples(rows)
}

func scanSchedulerTrainingSamples(rows *sql.Rows) ([]*controlstate.SchedulerTrainingSample, error) {
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

func scanSchedulerTrainingSample(rows *sql.Rows, s *controlstate.SchedulerTrainingSample) error {
	return rows.Scan(
		&s.ID, &s.TaskID, &s.ModelClass, &s.EstimatedInputTokens, &s.EstimatedOutputTokens,
		&s.Stream, &s.Priority, &s.TimeoutClass, &s.EnqueueTimeMs, &s.RequestKind,
		&s.RouteHint, &s.HasToolCalls, &s.ToolCallDepth, &s.TurnCount, &s.Multimodal,
		&s.QuestionCount, &s.CodeBlockCount, &s.EnumerationHint, &s.InstructionVerbCount,
		&s.MaxSentenceLengthBucket, &s.VocabularyRichnessBucket, &s.ConfidenceHint,
		&s.UncertaintyHint, &s.ActualLatencyMs, &s.InputTokens, &s.OutputTokens,
		&s.Outcome, &s.ProviderClass, &s.SchedulerVersion, &s.CompletedAt, &s.CreatedAt,
	)
}

func sampleValues(s *controlstate.SchedulerTrainingSample) []any {
	return []any{
		s.ID, s.TaskID, s.ModelClass, s.EstimatedInputTokens, s.EstimatedOutputTokens,
		s.Stream, s.Priority, s.TimeoutClass, s.EnqueueTimeMs, s.RequestKind,
		s.RouteHint, s.HasToolCalls, s.ToolCallDepth, s.TurnCount, s.Multimodal,
		s.QuestionCount, s.CodeBlockCount, s.EnumerationHint, s.InstructionVerbCount,
		s.MaxSentenceLengthBucket, s.VocabularyRichnessBucket, s.ConfidenceHint,
		s.UncertaintyHint, s.ActualLatencyMs, s.InputTokens, s.OutputTokens,
		s.Outcome, s.ProviderClass, s.SchedulerVersion, s.CompletedAt, s.CreatedAt,
	}
}

const schedulerTrainingSampleColumns = `
	id, task_id, model_class, estimated_input_tokens, estimated_output_tokens,
	stream, priority, timeout_class, enqueue_time_ms, request_kind, route_hint,
	has_tool_calls, tool_call_depth, turn_count, multimodal, question_count,
	code_block_count, enumeration_hint, instruction_verb_count,
	max_sentence_length_bucket, vocabulary_richness_bucket, confidence_hint,
	uncertainty_hint, actual_latency_ms, input_tokens, output_tokens, outcome,
	provider_class, scheduler_version, completed_at, created_at`

const insertSchedulerTrainingSampleSQL = `INSERT INTO scheduler_training_samples (` + schedulerTrainingSampleColumns + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

const selectSchedulerTrainingSamplesSQL = `SELECT ` + schedulerTrainingSampleColumns + `
FROM scheduler_training_samples
WHERE completed_at >= ? AND completed_at < ?
ORDER BY completed_at ASC
LIMIT ?`
