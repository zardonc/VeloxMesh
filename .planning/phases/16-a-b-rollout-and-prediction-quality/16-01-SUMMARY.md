---
phase: 16-a-b-rollout-and-prediction-quality
plan: 16-01
subsystem: scheduler
tags: [scheduler, rollout, onnx, heuristic, config]
requires:
  - phase: 15-training-feedback-and-onnx-path
    provides: ONNX scheduler output and scheduler version metadata
provides:
  - Gateway-side weighted heuristic/ONNX scheduler scoring
  - Scheduler rollout configuration with legacy endpoint compatibility
  - Internal scheduler type metadata on score results
affects: [scheduler, config, app, observability]
tech-stack:
  added: []
  patterns: [weighted scorer wrapper, task-id rollout bucket]
key-files:
  created: []
  modified:
    - internal/config/config.go
    - internal/config/config_validation.go
    - internal/scheduler/client.go
    - internal/scheduler/types.go
    - internal/app/app_test.go
    - .env.example
key-decisions:
  - "Reused scheduler.NewScorer as the single gateway wiring point for heuristic-only and weighted rollout modes."
  - "Kept SCHEDULER_ENDPOINT as a backward-compatible heuristic endpoint alias."
patterns-established:
  - "WeightedScorer batches task-level assignment by backend and falls back ONNX to heuristic before FIFO."
requirements-completed: [ML-03]
duration: 23 min
completed: 2026-07-04
---

# Phase 16 Plan 01: Weighted Scheduler Backend Selection Summary

**Gateway-side weighted heuristic/ONNX scheduler routing with task-level assignment and FIFO fail-open fallback**

## Performance

- **Duration:** 23 min
- **Started:** 2026-07-04T20:40:00Z
- **Completed:** 2026-07-04T21:03:00Z
- **Tasks:** 3
- **Files modified:** 9

## Accomplishments

- Added heuristic and ONNX scheduler endpoint config plus bounded ONNX rollout percent validation.
- Added `WeightedScorer` using deterministic task ID buckets and ONNX -> heuristic -> FIFO fallback.
- Proved app startup supports disabled, heuristic-only, and weighted rollout scheduler configuration without public data-plane changes.

## Task Commits

1. **Task 1: Add scheduler rollout config fields and validation** - `7b96d759` (feat)
2. **Task 2: Implement task-level weighted backend scoring** - `cc51f27f` (feat)
3. **Task 3: Wire weighted scorer into app construction** - `3129afb5` (test)

## Files Created/Modified

- `internal/config/config.go` - Added scheduler backend endpoint and rollout fields with env loading.
- `internal/config/config_validation.go` - Added rollout percent and ONNX endpoint validation.
- `internal/config/config_test.go` - Covered defaults, boundaries, legacy endpoint alias, and ONNX endpoint requirement.
- `internal/scheduler/types.go` - Added internal scheduler type metadata.
- `internal/scheduler/client.go` - Added weighted scorer routing and fallback behavior.
- `internal/scheduler/client_test.go` - Covered rollout 0, rollout 100, deterministic assignment, and ONNX fallback.
- `internal/scheduler/onnx/scorer.go` - Tagged local ONNX scores with scheduler type metadata.
- `internal/app/app_test.go` - Covered heuristic-only and weighted rollout startup.
- `.env.example` - Added placeholder scheduler rollout endpoint settings.

## Decisions Made

- Reused `scheduler.NewScorer` as the only construction point instead of adding a new app-level scorer factory.
- Treated `SCHEDULER_ENDPOINT` as a legacy heuristic alias to preserve existing deployments.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

Run separate heuristic and ONNX Scheduler services, configure placeholder endpoint values, and keep `SCHEDULER_ONNX_ROLLOUT_PERCENT=0` until quality evidence is available.

## Verification

- `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/app`
- `rg "kill.?switch|emergency.*switch" internal/config internal/scheduler internal/app`
- `rg "onnx_rollout|scheduler_type|scheduler_version" internal/http internal/gateway`

## Self-Check: PASSED

All task acceptance criteria passed.

## Next Phase Readiness

Weighted scoring metadata is available for Plan 16-02 quality metrics and durable rollups.

---
*Phase: 16-a-b-rollout-and-prediction-quality*
*Completed: 2026-07-04*
