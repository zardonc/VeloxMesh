---
phase: 21-observability-admin-apis-tooling
plan: "03"
subsystem: config
tags: [scheduler, config, heuristic, quality]
requires:
  - phase: 21-observability-admin-apis-tooling
    provides: training export and precise hydration
provides:
  - configurable semantic-neighbor embedding model
  - narrow heuristic override file
  - non-empty SchedulerType attribution guard
affects: [scheduler, config, observability]
tech-stack:
  added: []
  patterns: [narrow DisallowUnknownFields config loader, shared SchedulerType default guard]
key-files:
  created:
    - config.heuristic.example.json
  modified:
    - internal/config/config.go
    - internal/config/config_types.go
    - internal/app/semantic_neighbors.go
    - internal/scheduler/semantic_neighbors.go
    - internal/scheduler/heuristic/config.go
    - internal/scheduler/heuristic/score.go
    - internal/scheduler/intake.go
    - cmd/scheduler/main.go
key-decisions:
  - "Heuristic override files accept only base_latency and model_multipliers."
  - "Empty semantic-neighbor embedding model falls back to text-embedding-3-small."
patterns-established:
  - "Score metadata is normalized before quality evidence is recorded."
requirements-completed: ["QDR-08", "OBS-05", "OBS-06"]
duration: 1h
completed: 2026-07-06
---

# Phase 21 Plan 03: Scheduler Tuning Controls Summary

**Configurable semantic-neighbor embedding model, narrow heuristic overrides, and SchedulerType quality attribution**

## Performance

- **Duration:** 1h
- **Started:** 2026-07-06T19:20:00Z
- **Completed:** 2026-07-06T20:20:00Z
- **Tasks:** 3
- **Files modified:** 16

## Accomplishments

- Added `scheduler.semantic_neighbors_embedding_model` and `SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL`, defaulting to `text-embedding-3-small`.
- Added a narrow heuristic config loader for `base_latency` and `model_multipliers`, plus `config.heuristic.example.json`.
- Ensured heuristic, FIFO, gRPC, weighted, and metadata fallback paths do not record empty `SchedulerType`.

## Task Commits

1. **Scheduler tuning controls** - `221b386d` (feat)

## Files Created/Modified

- `config.heuristic.example.json` - safe template for the two supported override tables.
- `internal/config/*` - scheduler config field, env, JSON merge, defaults, and tests.
- `internal/app/semantic_neighbors.go` and `internal/scheduler/semantic_neighbors.go` - embedding model wiring.
- `internal/scheduler/heuristic/config.go` - narrow override loader.
- `internal/scheduler/heuristic/score.go` and `internal/scheduler/intake.go` - SchedulerType attribution and default guard.
- `cmd/scheduler/main.go` - heuristic config file loading for the scheduler service.

## Decisions Made

Kept heuristic tuning intentionally narrow and failed unknown top-level fields rather than exposing the whole scorer config.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Verification

- `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/scheduler/heuristic ./internal/app ./cmd/scheduler` passed.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Scheduler tuning controls and quality attribution are ready for documentation and UAT in Phase 22.

---
*Phase: 21-observability-admin-apis-tooling*
*Completed: 2026-07-06*
