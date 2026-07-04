---
status: passed
phase: 15-training-feedback-and-onnx-path
verified_at: 2026-07-04
requirements: [FEED-01, ML-01, ML-02]
plans_verified: [15-01, 15-02, 15-03]
automated_checks:
  passed: 11
  failed: 0
human_verification: []
gaps: []
---

# Phase 15 Verification

## Result

Phase 15 passes verification. The implementation delivers opt-in scheduler training feedback, offline Python training and artifact publishing, and ONNX scheduler runtime support without exposing raw prompts, payloads, provider secrets, API keys, or data-plane response changes.

## Requirement Traceability

| Requirement | Status | Evidence |
| --- | --- | --- |
| FEED-01 | passed | `SchedulerTrainingSample` schema uses explicit safe feature/label columns; SQLite/PostgreSQL repositories persist completed samples; recorder is opt-in via `SCHEDULER_FEEDBACK_ENABLED` and best-effort after success/failure. |
| ML-01 | passed | `tools/scheduler_training` provides `uv`-run export, train, evaluate, and publish tooling; runtime artifacts contain `model.onnx` and `manifest.json` with schema, metrics, parity, checksum, and model parameters. |
| ML-02 | passed | ONNX scheduler mode loads and validates artifacts at startup, reuses the loaded scorer, returns predicted latency/confidence/version through existing `BatchScoreTasks`, and keeps heuristic default mode. |

## Must-Haves

- D-01 through D-06: passed. Training samples contain safe TaskFeature fields plus labels only, write after completion/failure, use durable SQLite/PostgreSQL control state, and require explicit feedback opt-in.
- D-07 through D-11: passed. Python tooling stays outside the Go runtime, uses safe exports, trains the P70 output-token target, and publishes small versioned ONNX runtime artifacts.
- D-12 through D-18: passed. ONNX mode fails startup for invalid artifacts, maps output-token prediction through existing scheduler scoring, preserves proto stability, derives confidence, and leaves gateway-side A/B routing to Phase 16.

## Review Fixes Included

Code review found and fixed three issues before verification:

- ONNX proto mapping now preserves `max_sentence_length_bucket` and `vocabulary_richness_bucket`.
- SQLite/PostgreSQL training sample inserts clone inputs before applying default `CreatedAt`.
- Python export rejects sensitive field-name variants instead of silently dropping them.

## Automated Checks

- `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/config ./cmd/scheduler`
- `go test -timeout 60s ./internal/controlstate/... ./internal/scheduler/onnx ./internal/config ./cmd/scheduler`
- `go test -timeout 60s ./internal/controlstate/... ./internal/config ./internal/scheduler ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/app ./cmd/scheduler`
- `go test -timeout 60s ./...`
- `go build ./...`
- `uv run pytest`
- `node .codex/gsd-core/bin/gsd-tools.cjs query verify.schema-drift 15`
- `node .codex/gsd-core/bin/gsd-tools.cjs verify codebase-drift`
- Scoped scheduler training forbidden-field grep returned no matches.
- Python test forbidden-field grep returned no matches.
- Proto/config deferred-scope grep returned no matches.

## Gate Notes

- Schema drift: passed, no drift detected.
- Codebase drift: non-blocking skip, reason `no-structure-md`.
- Human verification: none required for Phase 15; all acceptance criteria are covered by automated tests and artifact inspection.

## Conclusion

Phase 15 is complete and ready to be marked done in the roadmap.
