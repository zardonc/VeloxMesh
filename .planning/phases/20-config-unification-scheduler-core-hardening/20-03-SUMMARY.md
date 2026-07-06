---
phase: 20-config-unification-scheduler-core-hardening
plan: "03"
subsystem: scheduler
tags: [semantic-neighbors, qdrant, embeddings, config, fail-open]
requires:
  - phase: 20-01
    provides: cache vector dimension and nested scheduler/cache config
provides:
  - capped semantic-neighbor embedding input
  - explicit Qdrant collection ensure path
  - semantic-neighbor startup fail-open on Qdrant ensure failure
affects: [scheduler, semantic-neighbors, qdrant, observability]
tech-stack:
  added: []
  patterns: [optional vector collection ensure, sanitized startup fallback metric]
key-files:
  created:
    - internal/storage/qdrant_test.go
  modified:
    - internal/scheduler/semantic_neighbors.go
    - internal/storage/qdrant.go
    - internal/app/semantic_neighbors.go
    - internal/config/config.go
    - internal/config/config_types.go
    - internal/config/config_validation.go
    - internal/observability/prometheus.go
key-decisions:
  - "Semantic-neighbor embedding input defaults to a 16000-character cap and can be overridden by scheduler config."
  - "Qdrant collection creation is explicit at startup when the adapter supports collection ensure."
  - "Qdrant ensure failures disable semantic neighbors without blocking gateway startup."
patterns-established:
  - "Record fixed low-cardinality semantic-neighbor reasons for input truncation and startup ensure failures."
  - "Keep collection ensure optional on the vector boundary instead of widening every vector adapter."
requirements-completed: ["QDR-05", "QDR-06"]
duration: 35 min
completed: 2026-07-06
---

# Phase 20 Plan 03: Semantic Neighbor Safeguards Summary

**Semantic-neighbor embedding input caps with explicit Qdrant collection startup checks and fail-open app wiring**

## Performance

- **Duration:** 35 min
- **Started:** 2026-07-06T16:28:00Z
- **Completed:** 2026-07-06T17:03:35Z
- **Tasks:** 3
- **Files modified:** 13

## Accomplishments

- Added `defaultSemanticNeighborInputMaxChars = 16000`, scheduler config override, and truncation before embedding requests are built.
- Added `QdrantVectorAdapter.EnsureCollection` and made `Insert` reuse the same ensure path.
- Wired semantic-neighbor startup to ensure `scheduler_training_samples` using `cache.vector_dimension`, disabling semantic neighbors fail-open on ensure failure.
- Added real-component coverage for OpenAI-compatible embedding HTTP calls, Qdrant collection creation, app startup success, and app startup fail-open.

## Task Commits

1. **Tasks 1-3: semantic-neighbor safeguards** - `1558ca0` (`feat(20-03): harden semantic neighbors`)

## Files Created/Modified

- `internal/scheduler/semantic_neighbors.go` - Input cap, sanitized truncation evidence, exported collection name.
- `internal/storage/qdrant.go` - Explicit ensure collection method reused by insert.
- `internal/storage/interfaces.go` - Narrow optional `VectorCollectionEnsurer` capability.
- `internal/app/semantic_neighbors.go` - Startup ensure and fail-open disabling.
- `internal/config/*` - Scheduler input cap config, defaults, validation, and tests.
- `internal/observability/prometheus.go` - Fixed allowlist labels for truncation and startup ensure evidence.
- `internal/storage/qdrant_test.go` - Real Qdrant ensure/insert coverage.

## Decisions Made

- Kept collection ensure as an optional interface so non-Qdrant adapters do not inherit unused API surface.
- Used `cache.vector_dimension` as the startup collection dimension source.
- Recorded truncation and startup ensure evidence only as fixed metric reasons, never prompt text or provider payloads.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## Verification

- `go test -timeout 60s ./internal/scheduler ./internal/storage ./internal/app`
- `go test -timeout 60s ./internal/scheduler ./internal/storage ./internal/app ./internal/config ./internal/observability`
- `go test -timeout 60s ./...`
- `go build ./...`

## User Setup Required

None - no external service configuration required beyond the existing real Qdrant test environment.

## Next Phase Readiness

Phase 20 plan execution is complete and ready for phase-level review/verification gates.

---
*Phase: 20-config-unification-scheduler-core-hardening*
*Completed: 2026-07-06*
