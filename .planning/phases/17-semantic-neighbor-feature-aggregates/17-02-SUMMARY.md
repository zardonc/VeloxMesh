---
phase: 17-semantic-neighbor-feature-aggregates
plan: 17-02
subsystem: scheduler
tags: [scheduler, qdrant, semantic-neighbors, observability]
requires:
  - phase: 17-semantic-neighbor-feature-aggregates
    provides: Semantic-neighbor TaskFeature fields and disabled-by-default config
provides:
  - Gateway semantic-neighbor aggregation from completed scheduler training samples
  - Fail-open intake enrichment between safe feature extraction and scheduler scoring
  - Completed-sample vector indexing with sanitized semantic-neighbor metrics
affects: [scheduler, gateway, observability, app]
tech-stack:
  added: []
  patterns: [gateway-side optional enrichment, safe vector metadata, fail-open scheduler dependency injection]
key-files:
  created:
    - internal/scheduler/semantic_neighbors.go
    - internal/scheduler/semantic_neighbors_test.go
    - internal/app/semantic_neighbors.go
  modified:
    - internal/scheduler/intake.go
    - internal/scheduler/intake_test.go
    - internal/scheduler/executor.go
    - internal/scheduler/training.go
    - internal/scheduler/quality_test.go
    - internal/app/app.go
    - internal/app/app_test.go
    - internal/observability/metrics.go
    - internal/observability/prometheus.go
key-decisions:
  - "Reused the existing semantic-cache embedding provider and vector adapter path for scheduler semantic-neighbor lookup."
  - "Hydrated vector sample IDs through recent SchedulerTrainingSample windows because the repository has no GetByID method."
  - "Kept enrichment fail-open at TaskIntake so Scheduler scoring still receives neutral aggregate fields on dependency errors."
patterns-established:
  - "Optional Gateway enrichers are injected into TaskIntake and run after ExtractSafeFeatures but before Scorer.Score."
  - "Completed training samples are indexed only after TrainingRecorder.Insert succeeds."
requirements-completed: [QDR-01, QDR-02, QDR-03]
duration: 40 min
completed: 2026-07-04
---

# Phase 17 Plan 02: Semantic Neighbor Collection and Runtime Enrichment Summary

**Gateway intake now enriches safe scheduler features from completed training-sample neighbors and fails open with sanitized metrics**

## Performance

- **Duration:** 40 min
- **Started:** 2026-07-04T20:00:00-07:00
- **Completed:** 2026-07-04T20:40:30-07:00
- **Tasks:** 3
- **Files modified:** 12

## Accomplishments

- Added a Gateway-owned semantic-neighbor service that embeds the in-memory request, searches safe sample metadata, hydrates completed scheduler training samples, and computes bounded aggregate fields.
- Wired TaskIntake ordering as `ExtractSafeFeatures -> SemanticNeighbors.Enrich -> Scorer.Score` with timeout/error fail-open behavior.
- Indexed completed training samples only after durable training insertion succeeds, and added closed-label Prometheus metrics for attempts, errors, timeouts, fallbacks, and coverage.

## Task Commits

1. **Task 1 and Task 3: Semantic-neighbor aggregation, indexing, and metrics** - `e2bec43` (feat)
2. **Task 2: Gateway intake and app wiring** - `739ad77` (feat)

## Files Created/Modified

- `internal/scheduler/semantic_neighbors.go` - Implements semantic-neighbor enrichment, safe metadata indexing, completed-sample hydration, aggregate calculation, and metrics recording.
- `internal/scheduler/semantic_neighbors_test.go` - Covers tenant scope, fallback scope, min-count defaults, safe metadata, and index-after-record ordering.
- `internal/scheduler/intake.go` - Injects optional semantic enrichment between safe feature extraction and scheduler scoring.
- `internal/scheduler/intake_test.go` - Proves enrichment ordering and error/timeout fail-open behavior.
- `internal/scheduler/executor.go` - Records timeout completion evidence and indexes samples after training records are stored.
- `internal/scheduler/training.go` - Adds the timeout training outcome.
- `internal/scheduler/quality_test.go` - Updates completion-evidence test setup for request-aware indexing.
- `internal/app/semantic_neighbors.go` - Builds the concrete semantic-neighbor service from existing provider, vector, and training repository dependencies.
- `internal/app/app.go` - Wires the optional enricher/indexer into the scheduler runner and exposes startup state.
- `internal/app/app_test.go` - Covers startup when semantic-neighbor dependencies are missing.
- `internal/observability/metrics.go` - Adds semantic-neighbor metrics methods to the metrics interface and stub.
- `internal/observability/prometheus.go` - Registers closed-label semantic-neighbor Prometheus counters.

## Decisions Made

- Reused the existing semantic-cache provider/vector configuration path instead of introducing a scheduler-only provider registry.
- Used recent `ListByWindow` hydration by `sample_id` because the training sample repository does not expose direct ID lookup.
- Split commits into two compile-safe slices: the core aggregation/indexing/metrics foundation, then the intake/app wiring.

## Deviations from Plan

None - plan executed exactly as written.

---

**Total deviations:** 0 auto-fixed.
**Impact on plan:** None.

## Issues Encountered

- `git add` initially failed in the sandbox with `Unable to create .git/index.lock: Permission denied`; reran the same staging command with approved escalation and continued without changing the file set.

## User Setup Required

None - no external service configuration required.

## Verification

- `go test -count=1 -timeout 60s ./internal/scheduler ./internal/app ./internal/observability`
- `go build ./...`

## Self-Check: PASSED

All task acceptance criteria passed.

## Next Phase Readiness

17-03 can now consume semantic-neighbor aggregate fields in downstream scheduler training/export behavior without raw request content crossing into Scheduler scoring.

---
*Phase: 17-semantic-neighbor-feature-aggregates*
*Completed: 2026-07-04*
