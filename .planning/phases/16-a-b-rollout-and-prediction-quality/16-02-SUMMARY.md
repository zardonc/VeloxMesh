---
phase: 16-a-b-rollout-and-prediction-quality
plan: 16-02
subsystem: observability
tags: [scheduler, quality, mape, prometheus, sqlite, postgres]
requires:
  - phase: 16-a-b-rollout-and-prediction-quality
    provides: Weighted heuristic/ONNX scheduler score metadata
provides:
  - Live scheduler prediction-quality metrics
  - Durable scheduler quality rollups for SQLite and PostgreSQL
  - PredictionQualityRecorder wired to scheduler completion evidence
affects: [scheduler, observability, controlstate, app]
tech-stack:
  added: []
  patterns: [aggregate rollup repository, best-effort quality recording]
key-files:
  created:
    - internal/scheduler/quality.go
    - internal/controlstate/scheduler_quality_rollup.go
    - internal/controlstate/sqlite/scheduler_quality_rollups.go
    - internal/controlstate/postgres/scheduler_quality_rollups.go
    - internal/controlstate/migrations/sqlite/0008_scheduler_quality_rollups.sql
    - internal/controlstate/migrations/postgres/0007_scheduler_quality_rollups.sql
  modified:
    - internal/observability/metrics.go
    - internal/observability/prometheus.go
    - internal/scheduler/executor.go
    - internal/scheduler/intake.go
    - internal/app/app.go
key-decisions:
  - "Used aggregated rollups with safe sample IDs instead of a per-task quality event stream."
  - "Quality recording is best-effort and never changes successful data-plane responses."
patterns-established:
  - "Prediction quality evidence uses scheduler_type, scheduler_version, and task_type live labels only."
requirements-completed: [OBS-02]
duration: 34 min
completed: 2026-07-04
---

# Phase 16 Plan 02: Scheduler Prediction Quality Summary

**Live MAPE metrics and durable scheduler quality rollups linked to safe training samples**

## Performance

- **Duration:** 34 min
- **Started:** 2026-07-04T21:03:00Z
- **Completed:** 2026-07-04T21:37:00Z
- **Tasks:** 3
- **Files modified:** 25

## Accomplishments

- Added low-cardinality Prometheus metrics for scheduler MAPE, wait, call latency, and comparison errors.
- Added SQLite/PostgreSQL scheduler quality rollup tables and repository parity.
- Wired prediction quality recording after scheduled task completion with MAPE calculation and safe sample links.

## Task Commits

1. **Task 1: Add low-cardinality live comparison metrics** - `5647a44e` (feat)
2. **Task 2: Add durable quality rollup repository parity** - `ef346937` (feat)
3. **Task 3: Record MAPE and rollups from completed tasks** - `d9c57f0f` (feat)

## Files Created/Modified

- `internal/observability/metrics.go` - Extended metrics interface and stub.
- `internal/observability/prometheus.go` - Added scheduler quality collectors with safe labels.
- `internal/controlstate/scheduler_quality_rollup.go` - Added aggregate merge helpers.
- `internal/controlstate/sqlite/scheduler_quality_rollups.go` - Added SQLite rollup repository.
- `internal/controlstate/postgres/scheduler_quality_rollups.go` - Added PostgreSQL rollup repository.
- `internal/scheduler/quality.go` - Added MAPE calculation and prediction quality recorder.
- `internal/scheduler/executor.go` - Wired best-effort quality recording after completion.
- `internal/scheduler/intake.go` - Preserved internal scheduler score metadata for quality recording.
- `internal/app/app.go` - Passed durable rollup repository into the scheduler runner.

## Decisions Made

- Stored only aggregate quality records and safe sample IDs, not per-task quality event bodies.
- Used `abs(predicted_ms - actual_ms) / actual_ms * 100` as the sole MAPE calculation.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- SQLite quality rollup timestamps needed explicit RFC3339 string handling because the table stores bucket times as `TEXT`.

## User Setup Required

Enable scheduler feedback and durable control state before expecting safe sample links in quality rollups.

## Verification

- `go test -timeout 60s ./internal/observability ./internal/controlstate/... ./internal/scheduler ./internal/app`
- `rg "tenant|api_key|authorization|secret|prompt|message|payload_hash|payload" internal/controlstate/migrations/*/*scheduler_quality_rollups.sql`
- `rg "model_class|confidence|tenant|api_key|task_id|prompt|payload" internal/observability`

## Self-Check: PASSED

All task acceptance criteria passed.

## Next Phase Readiness

Runtime rollout controls can read current rollout state and expose quality aggregates without leaking tenant/API-key identity or raw request data.

---
*Phase: 16-a-b-rollout-and-prediction-quality*
*Completed: 2026-07-04*
