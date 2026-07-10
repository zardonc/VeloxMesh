-- +goose Up
CREATE TABLE IF NOT EXISTS scheduler_training_samples (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    model_class TEXT NOT NULL,
    estimated_input_tokens INTEGER NOT NULL,
    estimated_output_tokens INTEGER NOT NULL,
    stream BOOLEAN NOT NULL,
    priority TEXT NOT NULL,
    timeout_class TEXT NOT NULL,
    enqueue_time_ms INTEGER NOT NULL,
    request_kind TEXT NOT NULL,
    route_hint TEXT NOT NULL,
    has_tool_calls BOOLEAN NOT NULL,
    tool_call_depth INTEGER NOT NULL,
    turn_count INTEGER NOT NULL,
    multimodal BOOLEAN NOT NULL,
    question_count INTEGER NOT NULL,
    code_block_count INTEGER NOT NULL,
    enumeration_hint BOOLEAN NOT NULL,
    instruction_verb_count INTEGER NOT NULL,
    max_sentence_length_bucket INTEGER NOT NULL,
    vocabulary_richness_bucket INTEGER NOT NULL,
    confidence_hint REAL NOT NULL,
    uncertainty_hint REAL NOT NULL,
    actual_latency_ms INTEGER NOT NULL,
    input_tokens INTEGER NOT NULL,
    output_tokens INTEGER NOT NULL,
    outcome TEXT NOT NULL,
    provider_class TEXT NOT NULL,
    scheduler_version TEXT NOT NULL,
    completed_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_scheduler_training_samples_completed_at
    ON scheduler_training_samples(completed_at);

-- +goose Down
DROP TABLE IF EXISTS scheduler_training_samples;
