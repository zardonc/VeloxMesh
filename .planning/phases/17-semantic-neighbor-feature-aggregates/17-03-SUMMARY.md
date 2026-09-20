---
phase: 17-semantic-neighbor-feature-aggregates
plan: 17-03
subsystem: scheduler
tags: [scheduler, training, onnx, sqlite, postgres]
requires:
  - phase: 17-semantic-neighbor-feature-aggregates
    provides: Gateway semantic-neighbor aggregate enrichment and indexing
provides:
  - Durable scheduler training sample semantic aggregate fields
  - Offline export/training feature schema support for semantic aggregates
  - ONNX manifest compatibility metadata with legacy neutral defaults
affects: [scheduler, controlstate, scheduler-training, onnx]
tech-stack:
  added: []
  patterns: [first-class safe sample columns, semantic aggregate manifest capability flag, legacy ONNX neutral defaults]
key-files:
  created:
    - internal/controlstate/migrations/sqlite/0009_scheduler_training_semantic_aggregates.sql
    - internal/controlstate/migrations/postgres/0008_scheduler_training_semantic_aggregates.sql
  modified:
    - internal/controlstate/types.go
    - internal/scheduler/training.go
    - internal/scheduler/training_test.go
    - internal/controlstate/sqlite/migrations.go
    - internal/controlstate/sqlite/scheduler_training_samples.go
    - internal/controlstate/sqlite/scheduler_training_samples_test.go
    - internal/controlstate/postgres/migrations.go
    - internal/controlstate/postgres/scheduler_training_samples.go
    - internal/controlstate/postgres/scheduler_training_samples_test.go
    - tools/scheduler_training/scheduler_training/export.py
    - tools/scheduler_training/scheduler_training/train.py
    - tools/scheduler_training/scheduler_training/artifacts.py
    - tools/scheduler_training/tests/test_export_schema.py
    - tools/scheduler_training/tests/test_train_publish.py
    - tools/scheduler_training/tests/test_artifacts.py
    - internal/scheduler/onnx/artifact.go
    - internal/scheduler/onnx/scorer.go
    - internal/scheduler/onnx/artifact_test.go
    - internal/scheduler/onnx/scorer_test.go
    - internal/scheduler/heuristic/score_test.go
key-decisions:
  - "Stored semantic aggregates as first-class sample columns with non-null D-08 defaults in both SQLite and PostgreSQL."
  - "Kept offline training's constant P70 model shape while adding deterministic semantic feature preparation and manifest metadata."
  - "Made ONNX semantic aggregate support opt-in through manifest metadata so legacy artifacts keep neutral runtime behavior."
patterns-established:
  - "Training/export/runtime schemas carry semantic aggregate fields only as bounded numeric or enum values."
  - "Heuristic score behavior is protected by aggregate-only invariance tests."
requirements-completed: [QDR-02, QDR-04]
duration: 55 min
completed: 2026-07-04
---

# Phase 17 Plan 03: Training, Export, and ONNX Compatibility Summary

**Semantic-neighbor aggregates now persist through training samples, offline feature preparation, and ONNX artifact compatibility without changing heuristic scoring**

## Performance

- **Duration:** 55 min
- **Started:** 2026-07-04T20:40:30-07:00
- **Completed:** 2026-07-04T21:35:00-07:00
- **Tasks:** 3
- **Files modified:** 22

## Accomplishments

- Added nine semantic aggregate fields to `SchedulerTrainingSample`, SQLite/PostgreSQL insert/select paths, migrations, and recorder copy behavior.
- Extended scheduler-training export and training feature preparation with default-filled semantic aggregate columns and manifest capability metadata.
- Added ONNX runtime support for semantic aggregate-aware artifacts while preserving neutral defaults for legacy artifacts.
- Proved heuristic scoring is invariant when only semantic aggregate fields change.

## Task Commits

1. **Task 1: Persist aggregate fields in scheduler training samples** - `f9cb792` (feat)
2. **Task 2: Extend offline export and training feature schema** - `4c419eb` (feat)
3. **Task 3: Add ONNX compatibility defaults and heuristic score invariance** - `6dcad14` (feat)

## Files Created/Modified

- `internal/controlstate/types.go` - Adds first-class semantic aggregate fields to durable training samples.
- `internal/scheduler/training.go` - Copies aggregate snapshots from `Task.Feature` into completed samples.
- `internal/controlstate/migrations/sqlite/0009_scheduler_training_semantic_aggregates.sql` - Adds bounded SQLite columns with D-08 defaults.
- `internal/controlstate/migrations/postgres/0008_scheduler_training_semantic_aggregates.sql` - Adds bounded PostgreSQL columns with D-08 defaults.
- `internal/controlstate/sqlite/*` and `internal/controlstate/postgres/*` - Maintain repository parity and migration application.
- `tools/scheduler_training/scheduler_training/export.py` - Adds semantic aggregate export columns and safe default filling.
- `tools/scheduler_training/scheduler_training/train.py` - Adds deterministic semantic feature preparation and support metadata.
- `tools/scheduler_training/scheduler_training/artifacts.py` - Publishes semantic aggregate support into runtime manifests only.
- `internal/scheduler/onnx/artifact.go` and `internal/scheduler/onnx/scorer.go` - Parse support metadata, validate supported fields, and keep unsupported artifacts neutral.
- `internal/scheduler/heuristic/score_test.go` - Protects D-15 heuristic invariance.

## Decisions Made

- Updated the hand-maintained migrator file lists alongside the SQL migrations so new columns are applied in tests and runtime startup.
- Left the offline model algorithm as the existing constant P70 predictor; Phase 17’s contract is feature availability and compatibility, not a new model architecture.
- Counted semantic coverage in ONNX confidence only when the manifest declares semantic aggregate support.

## Deviations from Plan

None - plan executed exactly as written.

---

**Total deviations:** 0 auto-fixed.
**Impact on plan:** None.

## Issues Encountered

- The first Go verification hit sandbox Go-cache access denial; reran with approved cache access.
- The first root-level `uv run pytest ...` did not pick up the scheduler-training project environment; reran with `uv run --project tools/scheduler_training ...`.

## User Setup Required

None - no external service configuration required.

## Verification

- `go test -count=1 -timeout 60s ./internal/scheduler ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/controlstate/sqlite ./internal/controlstate/postgres`
- `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests -q`
- `go build ./...`

## Self-Check: PASSED

All task acceptance criteria passed.

## Next Phase Readiness

All Phase 17 implementation plans are complete. Phase-level verification can now validate QDR-01 through QDR-04 and close the semantic-neighbor feature aggregate phase.

---
*Phase: 17-semantic-neighbor-feature-aggregates*
*Completed: 2026-07-04*
