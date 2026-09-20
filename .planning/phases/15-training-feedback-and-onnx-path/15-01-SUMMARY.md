---
phase: 15-training-feedback-and-onnx-path
plan: 15-01
subsystem: scheduler
tags: [scheduler, training, sqlite, postgres, config]
requires:
  - phase: 14-scheduler-queue-foundation
    provides: gateway-owned scheduler queue, safe task features, synchronous runner
provides:
  - durable scheduler training sample repository for SQLite and PostgreSQL
  - explicit scheduler feedback recording opt-in
  - best-effort completed-sample recorder on scheduler success and failure
affects: [scheduler, controlstate, config, app]
tech-stack:
  added: []
  patterns: [controlstate sub-repository parity, best-effort scheduler feedback recorder]
key-files:
  created:
    - internal/controlstate/migrations/sqlite/0007_scheduler_training_samples.sql
    - internal/controlstate/migrations/postgres/0006_scheduler_training_samples.sql
    - internal/controlstate/sqlite/scheduler_training_samples.go
    - internal/controlstate/postgres/scheduler_training_samples.go
    - internal/scheduler/training.go
  modified:
    - internal/controlstate/repository.go
    - internal/controlstate/types.go
    - internal/config/config.go
    - internal/app/app.go
    - internal/scheduler/executor.go
    - internal/scheduler/intake.go
key-decisions:
  - "Feedback recording is a separate opt-in flag: SCHEDULER_FEEDBACK_ENABLED."
  - "Recording is best-effort and disabled when durable control state is unavailable."
  - "Samples store explicit allowlisted feature and label columns rather than raw request material."
patterns-established:
  - "Scheduler training samples are written only after completion or terminal handler failure."
  - "Recorder errors use low-cardinality scheduler error metrics and do not alter data-plane responses."
requirements-completed: [FEED-01]
duration: 52min
completed: 2026-07-04
---

# Phase 15 Plan 01: Scheduler Training Feedback Summary

**Opt-in durable scheduler training feedback with safe feature snapshots and completion labels**

## Performance

- **Duration:** 52 min
- **Started:** 2026-07-04T10:55:00-07:00
- **Completed:** 2026-07-04T11:47:24-07:00
- **Tasks:** 3
- **Files modified:** 21

## Accomplishments

- Added SQLite and PostgreSQL scheduler training sample tables plus repository APIs for insert and time-window export.
- Added `SCHEDULER_FEEDBACK_ENABLED`, defaulting off and independent from `SCHEDULER_ENABLED`.
- Added a best-effort scheduler training recorder that writes one safe sample after success or terminal handler failure.

## Task Commits

1. **Task 1: Add durable sample schema and repository parity** - `e33d6b7` (feat)
2. **Task 2: Add opt-in feedback config and admin/control visibility** - `7a1f307` (feat)
3. **Task 3: Record completed samples from scheduler execution** - `2765b0e` (feat)

## Files Created/Modified

- `internal/controlstate/migrations/sqlite/0007_scheduler_training_samples.sql` - SQLite safe sample table.
- `internal/controlstate/migrations/postgres/0006_scheduler_training_samples.sql` - PostgreSQL parity table.
- `internal/controlstate/sqlite/scheduler_training_samples.go` - SQLite insert and time-window list implementation.
- `internal/controlstate/postgres/scheduler_training_samples.go` - PostgreSQL insert and time-window list implementation.
- `internal/scheduler/training.go` - Completed sample recorder and label conversion.
- `internal/scheduler/executor.go` - Success/failure recording hooks in the synchronous runner.
- `internal/config/config.go` - Scheduler feedback opt-in config.
- `internal/app/app.go` - Recorder installation only when durable control state is present.

## Decisions Made

- Used explicit typed columns for the sample allowlist instead of a feature blob, so forbidden fields are easy to audit.
- Disabled feedback recording when control state is disabled, while preserving the operator's config value for visibility.
- Kept recorder failures non-blocking to preserve the OpenAI-compatible response path.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- The existing SQLite migrator only applied a fixed migration list on new databases. It now includes the new migration and ensures the training table exists for older migrated databases.
- The plan's broad forbidden-field grep would match existing Phase 1 `provider_secrets`, `api_keys`, and fallback `payload` tables. Verification used a scoped grep against the new training migrations and recorder, which returned no matches.

## User Setup Required

Operators must set `SCHEDULER_FEEDBACK_ENABLED=true` and use a durable control-state backend to record samples. Scheduler enablement alone does not store training samples.

## Verification

- `go test -timeout 60s ./internal/controlstate/... ./internal/config ./internal/scheduler ./internal/app`
- `rg "raw_prompt|messages|authorization|api_key|secret|payload_hash|payload" internal/controlstate/migrations/sqlite/0007_scheduler_training_samples.sql internal/controlstate/migrations/postgres/0006_scheduler_training_samples.sql internal/scheduler/training.go` returned no matches.
- `rg "scheduler_feedback|training" internal/http internal/gateway` returned no matches.

## Next Phase Readiness

Plan 15-02 can export safe completed samples through the new repository boundary and build offline training tooling without adding Python to the gateway runtime.

---
*Phase: 15-training-feedback-and-onnx-path*
*Completed: 2026-07-04*
