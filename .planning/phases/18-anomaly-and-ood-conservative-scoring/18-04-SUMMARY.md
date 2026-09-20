---
phase: 18-anomaly-and-ood-conservative-scoring
plan: "04"
subsystem: scheduler
tags: [scheduler, predictor, onnxruntime, grpc, anomaly]
requires:
  - phase: 18-anomaly-and-ood-conservative-scoring
    provides: anomaly metadata, quality evidence, semantic scheduler features
provides:
  - Model-neutral OutputTokenPredictor contract with predictor-v1 manifest validation
  - Python ONNX Runtime worker using onnxruntime.InferenceSession
  - Go predictor gRPC client with health, timeout, breaker, and process supervisor
  - PredictiveScorer policy layer over quantiles and model-native signals
affects: [scheduler, scheduler-training, anomaly-quality]
tech-stack:
  added: [grpcio, grpcio-tools, onnxruntime]
  patterns: [predictor-boundary, predictive-scheduler-policy, real-onnx-worker]
key-files:
  created:
    - internal/scheduler/predictor/types.go
    - internal/scheduler/predictor/python_client.go
    - internal/scheduler/predictive/scorer.go
    - tools/scheduler_training/scheduler_training/onnx_worker.py
    - proto/predictor/v1/predictor.proto
  modified:
    - cmd/scheduler/main.go
    - tools/scheduler_training/scheduler_training/artifacts.py
    - tools/scheduler_training/scheduler_training/train.py
    - internal/scheduler/onnx/server.go
key-decisions:
  - "Scheduler mode onnx now routes through the model-neutral predictive scorer boundary."
  - "Python worker is the default real ONNX Runtime path; Go constant parsing remains compatibility/test support only."
  - "Predictor success clears fallback reason; Scheduler policy owns quantile selection and OOD interpretation."
patterns-established:
  - "Predictor returns quantiles/signals/per-task errors; Scheduler turns them into confidence, uncertainty, and scores."
  - "Python worker artifacts use released ONNX opset 26 for onnxruntime compatibility."
requirements-completed: ["ANOM-01", "ANOM-02", "ANOM-03", "ANOM-04"]
duration: 35 min
completed: 2026-07-05
---

# Phase 18 Plan 04: Predictor Boundary and Real ONNX Runtime Summary

**Model-neutral scheduler prediction with a real Python ONNX Runtime worker and predictive scoring policy**

## Performance

- **Duration:** 35 min
- **Started:** 2026-07-05T18:16:57Z
- **Completed:** 2026-07-05T18:51:47Z
- **Tasks:** 3
- **Files modified:** 34

## Accomplishments

- Added the predictor contract, NoopPredictor, predictor-v1 manifest schema, and fail-fast feature schema validation.
- Added a real Python gRPC worker that loads `manifest.json`, creates one `onnxruntime.InferenceSession`, and returns quantiles/signals with per-task errors.
- Added Go predictor client lifecycle handling: startup health, call timeout, breaker, recovery probe behavior, and worker process restart backoff.
- Added `PredictiveScorer` so Scheduler policy selects P50/P70/P90, interprets OOD/spread signals, records anomaly status, and falls back per task.
- Rewired `cmd/scheduler` ONNX mode to the predictive boundary and added an end-to-end smoke test with a real Python ONNX worker.

## Task Commits

1. **Task 1: Define predictor contract and manifest validation** - `4d7c3469` (feat)
2. **Task 2: Add Python ONNX Runtime worker and Go client lifecycle** - `e01a2807` (feat)
3. **Task 3: Replace ONNXScorer with PredictiveScorer policy and routing** - `e252f456` (feat)

## Files Created/Modified

- `internal/scheduler/predictor/` - Predictor contract, manifest gate, NoopPredictor, gRPC client, process supervisor, router, and tests.
- `internal/scheduler/predictive/` - Scheduler-owned predictive policy and BatchScoreTasks service mapping.
- `proto/predictor/v1/predictor.proto` and generated bindings - Predictor worker protocol.
- `tools/scheduler_training/scheduler_training/onnx_worker.py` - Long-lived Python ONNX Runtime worker.
- `cmd/scheduler/main.go` - ONNX mode now builds predictive scorer over the predictor boundary.
- `tools/scheduler_training/scheduler_training/artifacts.py` - Predictor-v1 manifest fields and released-opset quantile ONNX output.

## Decisions Made

- Kept the Gateway/Scheduler RPC unchanged; predictor communication is a local Scheduler-owned boundary.
- Used Python ONNX Runtime as the real runtime path to keep the default Go build pure.
- Kept legacy `internal/scheduler/onnx` parser tests, but production Scheduler startup no longer treats that parser as runtime evidence.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] ONNX Runtime rejected development opset 27**
- **Found during:** Task 2 worker verification
- **Issue:** `onnxruntime.InferenceSession` rejected models emitted with ONNX opset 27 because current runtime support only guarantees released opset 26.
- **Fix:** Updated artifact publishing to stamp generated ONNX models with opset 26.
- **Files modified:** `tools/scheduler_training/scheduler_training/artifacts.py`
- **Verification:** `uv run pytest tests/test_onnx_worker.py tests/test_train_publish.py`
- **Committed in:** `e01a2807`

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Required for real ONNX Runtime acceptance; no scope expansion.

## Issues Encountered

- `uv run pytest ...` must be run from `tools/scheduler_training` so `pyproject.toml` dev dependencies resolve.

## User Setup Required

None - no external service configuration required for the implemented tests.

## Verification

- `uv run pytest tests/test_onnx_worker.py tests/test_train_publish.py` - 8 passed
- `go test -timeout 60s ./internal/scheduler/predictive ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler ./cmd/scheduler` - passed
- `go test -timeout 60s ./...` - passed
- `go build ./...` - passed

## Self-Check: PASSED

- Predictor contract is quantile-aware and model-neutral.
- Scheduler policy owns quantile selection, conservative scoring, OOD status, fallback, and routing.
- ONNX Runtime is invoked through `onnxruntime.InferenceSession` in a long-lived Python worker.
- Scheduler startup and request scoring degrade to NoopPredictor/heuristic when manifest, health, timeout, or breaker gates fail.
- Manifest schema drift fails before prediction calls.
- Partial batch failures preserve sibling predictions.
- Existing Gateway data-plane and Scheduler `BatchScoreTasks` contracts remain unchanged.
- ONNX is called through the final runtime path; the Go constant parser no longer satisfies production Scheduler startup.

## Next Phase Readiness

Phase 18 corrective execution is complete and ready for phase-level verification.

---
*Phase: 18-anomaly-and-ood-conservative-scoring*
*Completed: 2026-07-05*
