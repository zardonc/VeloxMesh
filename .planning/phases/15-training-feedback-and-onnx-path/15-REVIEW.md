---
status: clean
phase: 15-training-feedback-and-onnx-path
reviewed_at: 2026-07-04
depth: standard
files_reviewed: 21
findings:
  critical: 0
  warning: 3
  info: 0
  total: 3
resolution: fixed
---

# Phase 15 Code Review

## Scope

Reviewed Phase 15 scheduler feedback, offline training, and ONNX runtime changes from the plan summaries, focusing on:

- Scheduler training sample schema and repository parity.
- Feedback recorder safety and data-plane isolation.
- Python export, training, artifact publishing, and manifest contracts.
- ONNX artifact loading, scorer behavior, proto boundary mapping, and scheduler mode wiring.
- Phase requirements FEED-01, ML-01, and ML-02.

## Findings

### WR-01: ONNX proto mapping dropped two safe feature buckets

`internal/scheduler/onnx/server.go` mapped most `TaskFeature` proto fields but omitted `max_sentence_length_bucket` and `vocabulary_richness_bucket`. This lost safe request-shape signals at the ONNX service boundary.

Resolution: fixed by preserving both fields in `featureFromProto` and adding `TestFeatureFromProtoPreservesSafeFeatureBuckets`.

### WR-02: Training sample repositories mutated caller-owned input

SQLite and PostgreSQL `Insert` methods populated `CreatedAt` directly on the provided `SchedulerTrainingSample`. That violates the project immutability rule and could leak side effects back to callers.

Resolution: fixed by cloning the sample before applying default `CreatedAt`, with SQLite and PostgreSQL regression tests.

### WR-03: Python export sanitizer silently dropped sensitive field variants

`sanitize_row` rejected exact forbidden names, but variants such as `raw_payload` were only omitted by the allowlist. The exported CSV stayed safe, but the input error was hidden.

Resolution: fixed by rejecting any input field name containing a forbidden token, with a regression test that avoids storing sensitive fixture literals.

## Verification

- `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/config ./cmd/scheduler`
- `go test -timeout 60s ./internal/controlstate/... ./internal/scheduler/onnx ./internal/config ./cmd/scheduler`
- `go test -timeout 60s ./internal/controlstate/... ./internal/config ./internal/scheduler ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/app ./cmd/scheduler`
- `go test -timeout 60s ./...`
- `go build ./...`
- `uv run pytest`
- Scoped forbidden-field grep checks for scheduler training schema/recorder and Python tests returned no matches.
- Proto/config drift greps for deferred A/B or proto output-token fields returned no matches.

## Result

Phase 15 code review is clean after fixes. No remaining critical or warning findings.
