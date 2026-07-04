---
status: complete
phase: 15-training-feedback-and-onnx-path
source:
  - 15-01-SUMMARY.md
  - 15-02-SUMMARY.md
  - 15-03-SUMMARY.md
started: 2026-07-04T00:00:00-07:00
updated: 2026-07-04T00:00:00-07:00
verification_mode: real-components
mock_policy: "Mocks and skipped tests are not accepted as evidence for this UAT."
---

## Current Test

[testing complete]

## Tests

### 1. Safe Scheduler Training Feedback
expected: |
  SQLite/PostgreSQL durable training sample repositories, opt-in feedback config, and scheduler completion/failure recording work through real package tests. Feedback remains disabled by default and no sample is written at enqueue.
result: pass
evidence:
  - "go test -count=1 -timeout 60s ./internal/controlstate/sqlite ./internal/scheduler ./internal/app"
  - "go test -count=1 -v -timeout 60s -run TestPostgresSchedulerTrainingSamplesInsertAndListByWindow ./internal/controlstate/postgres"

### 2. Offline Training Tooling
expected: |
  The uv-based scheduler training package exports safe rows, trains/evaluates the P70 output-token predictor, validates ONNX parity, and publishes only model.onnx plus manifest.json.
result: pass
evidence:
  - "uv run pytest"

### 3. ONNX Scheduler Runtime
expected: |
  ONNX artifact loading, checksum/schema validation, scoring, scheduler service mode selection, and existing BatchScoreTasks response fields work through real package tests.
result: pass
evidence:
  - "go test -count=1 -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/config ./cmd/scheduler"

### 4. Sensitive Field and Deferred Scope Guard
expected: |
  Training sample schema/tooling does not contain raw prompts, messages, auth headers, API keys, secrets, payload hashes, or raw payload fields. Scheduler proto does not expose predicted_output_tokens, and Phase 16 gateway backend selector config is not present.
result: pass
evidence:
  - "rg -n \"raw_prompt|messages|authorization|api_key|secret|payload_hash|payload\" internal/controlstate/migrations/sqlite/0007_scheduler_training_samples.sql internal/controlstate/migrations/postgres/0006_scheduler_training_samples.sql internal/scheduler/training.go"
  - "rg -n \"prompt|messages|authorization|api_key|secret|payload_hash|payload\" tools/scheduler_training/tests"
  - "rg -n \"predicted_output_tokens\" proto/scheduler/v1/scheduler.proto"
  - "rg -n \"SCHEDULER_AB|SCHEDULER_BACKEND|scheduler_backend\" internal/config cmd internal/app"
notes: "All four rg checks returned no matches."

### 5. Build and Schema Drift
expected: |
  The repository builds and GSD schema drift verification reports no blocking drift for Phase 15.
result: pass
evidence:
  - "go build ./..."
  - "node .codex\\gsd-core\\bin\\gsd-tools.cjs query verify.schema-drift 15"

## Summary

total: 5
passed: 5
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

None.
