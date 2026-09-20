---
phase: 21-observability-admin-apis-tooling
plan: "02"
subsystem: api
tags: [scheduler, training-data, semantic-neighbors, admin-api]
requires: []
provides:
  - safe scheduler training sample export
  - precise scheduler training sample lookup by ID
  - semantic neighbor hydration by exact vector result IDs
affects: [scheduler, semantic-neighbors, controlstate]
tech-stack:
  added: []
  patterns: [safe export projection, ordered repository hydration]
key-files:
  created: []
  modified:
    - internal/controlstate/repository.go
    - internal/controlstate/sqlite/scheduler_training_samples.go
    - internal/controlstate/postgres/scheduler_training_samples.go
    - internal/http/handlers/admin_scheduler.go
    - internal/scheduler/admin_scheduler_service.go
    - internal/scheduler/semantic_neighbors.go
key-decisions:
  - "Training export uses an explicit features/labels whitelist and omits task IDs and raw payload-like fields."
  - "Semantic neighbor hydration asks repositories for exact vector result IDs and preserves vector result order."
patterns-established:
  - "SQLite and PostgreSQL repository APIs stay parity-tested for scheduler training samples."
requirements-completed: ["OBS-04", "QDR-07"]
duration: 1h
completed: 2026-07-06
---

# Phase 21 Plan 02: Training Export and ID Hydration Summary

**Safe scheduler training export plus precise semantic-neighbor sample hydration**

## Performance

- **Duration:** 1h
- **Started:** 2026-07-06T18:20:00Z
- **Completed:** 2026-07-06T19:20:00Z
- **Tasks:** 3
- **Files modified:** 12

## Accomplishments

- Added `SchedulerTrainingSampleRepository.ListByIDs` for SQLite and PostgreSQL with requested-order preservation.
- Updated semantic-neighbor hydration to use exact sample IDs from vector search results instead of broad window scans.
- Added `GET /admin/v1/scheduler/training-samples/export` with JSON default, NDJSON support, bounded limits, time/task filters, and safe features/labels projection.

## Task Commits

1. **Safe export and precise hydration** - `3d45e961` (feat)

## Files Created/Modified

- `internal/controlstate/repository.go` - repository contract.
- `internal/controlstate/sqlite/scheduler_training_samples.go` - SQLite `ListByIDs`.
- `internal/controlstate/postgres/scheduler_training_samples.go` - PostgreSQL `ListByIDs`.
- `internal/scheduler/semantic_neighbors.go` - exact ID hydration.
- `internal/http/handlers/admin_scheduler.go` - export handler.
- `internal/scheduler/admin_scheduler_service.go` - export request/window/projection logic.
- `internal/http/router.go` - export route.
- Repository, semantic-neighbor, training, and admin handler tests.

## Decisions Made

Used the existing training sample shape and explicit export DTOs rather than exposing repository records directly.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Verification

- `go test -timeout 60s ./internal/controlstate/sqlite ./internal/controlstate/postgres ./internal/scheduler ./internal/http/handlers` passed.
- `go test -timeout 60s ./internal/http ./internal/http/handlers ./internal/scheduler ./internal/controlstate/sqlite ./internal/controlstate/postgres` passed.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Export and hydration foundations are ready for Phase 22 operator docs and UAT.

---
*Phase: 21-observability-admin-apis-tooling*
*Completed: 2026-07-06*
