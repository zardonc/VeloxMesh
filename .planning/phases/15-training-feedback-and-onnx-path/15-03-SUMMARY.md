---
phase: 15-training-feedback-and-onnx-path
plan: 15-03
subsystem: scheduler
tags: [scheduler, onnx, grpc, config]
requires:
  - phase: 15-02
    provides: published scheduler artifact directory with model.onnx and manifest.json
provides:
  - ONNX artifact loader with checksum and schema validation
  - ONNX scheduler scorer using startup-loaded artifact parameters
  - ONNX BatchScoreTasks service without proto changes
  - scheduler binary heuristic/ONNX mode selection
affects: [scheduler, cmd, config]
tech-stack:
  added: []
  patterns: [startup artifact validation, endpoint-selected scheduler backend]
key-files:
  created:
    - internal/scheduler/onnx/artifact.go
    - internal/scheduler/onnx/scorer.go
    - internal/scheduler/onnx/server.go
  modified:
    - cmd/scheduler/main.go
    - internal/config/config.go
    - internal/config/config_validation.go
    - .env.example
key-decisions:
  - "Heuristic remains the default scheduler service mode."
  - "ONNX mode fails startup for missing, invalid, or checksum-mismatched artifacts."
  - "Gateway-side A/B routing remains deferred; operators select backend by running the desired scheduler service endpoint."
patterns-established:
  - "ONNX scorer maps P70 output-token prediction through existing heuristic score calculation."
  - "Low feature coverage lowers confidence without changing the scheduler proto."
requirements-completed: [ML-02]
duration: 11min
completed: 2026-07-04
---

# Phase 15 Plan 03: ONNX Scheduler Runtime Summary

**Startup-validated ONNX scheduler mode serving existing BatchScoreTasks responses with predicted latency, confidence, and version**

## Performance

- **Duration:** 11 min
- **Started:** 2026-07-04T11:58:00-07:00
- **Completed:** 2026-07-04T12:08:40-07:00
- **Tasks:** 3
- **Files modified:** 12

## Accomplishments

- Added `internal/scheduler/onnx` with manifest parsing, checksum validation, schema validation, scorer, and gRPC service.
- Added scheduler config for `SCHEDULER_MODE=heuristic|onnx` and `SCHEDULER_ONNX_ARTIFACT_DIR`.
- Updated `cmd/scheduler` so heuristic remains default and ONNX mode fails startup clearly when artifacts are invalid.

## Task Commits

1. **Task 1: Load and validate ONNX artifact directories at startup** - `602ebbf` (feat)
2. **Task 2: Implement ONNX scorer using existing scheduler contract** - `e9217fe` (feat)
3. **Task 3: Wire scheduler service mode without gateway-side A/B routing** - `33e80d7` (feat)

## Files Created/Modified

- `internal/scheduler/onnx/artifact.go` - manifest, checksum, parity, and schema validation.
- `internal/scheduler/onnx/scorer.go` - startup-loaded scorer using P70 output-token artifact parameters.
- `internal/scheduler/onnx/server.go` - BatchScoreTasks implementation for ONNX mode.
- `cmd/scheduler/main.go` - scheduler service mode selection.
- `internal/config/config.go` - scheduler mode and ONNX artifact directory config.
- `.env.example` - documented heuristic default and ONNX artifact path knob.

## Decisions Made

- Reused the existing heuristic score formula after replacing estimated output tokens with the artifact's P70 prediction.
- Preserved the scheduler proto; no output-token metadata is exposed in Phase 15.
- Kept backend selection outside the gateway, matching the Phase 16 boundary.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

Run the scheduler with `SCHEDULER_MODE=onnx` and `SCHEDULER_ONNX_ARTIFACT_DIR` pointing at a valid artifact directory, then point the gateway `SCHEDULER_ENDPOINT` at that scheduler service.

## Verification

- `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/config ./cmd/scheduler`
- `rg "predicted_output_tokens" proto/scheduler/v1/scheduler.proto` returned no matches.
- `rg "SCHEDULER_AB|SCHEDULER_BACKEND|scheduler_backend" internal/config cmd internal/app` returned no matches.

## Next Phase Readiness

Phase 16 can add gateway-side heuristic/ONNX routing, A/B rollout, rollback controls, and prediction-quality comparison on top of this endpoint-selected scheduler mode.

---
*Phase: 15-training-feedback-and-onnx-path*
*Completed: 2026-07-04*
