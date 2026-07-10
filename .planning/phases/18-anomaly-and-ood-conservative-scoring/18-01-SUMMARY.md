---
phase: 18-anomaly-and-ood-conservative-scoring
plan: "01"
subsystem: scheduler-training
tags: [onnx, anomaly, training, manifest]
requires:
  - phase: 15-training-feedback-and-onnx-path
    provides: safe scheduler samples and ONNX artifact publishing
provides:
  - anomaly/OOD thresholds in scheduler artifact manifests
  - Go manifest structs for anomaly metadata
affects: [scheduler, onnx, training]
tech-stack:
  added: []
  patterns: [manifest metadata extension, safe sample threshold computation]
key-files:
  created: []
  modified:
    - tools/scheduler_training/scheduler_training/train.py
    - tools/scheduler_training/scheduler_training/artifacts.py
    - tools/scheduler_training/tests/test_train_publish.py
    - internal/scheduler/onnx/artifact.go
    - internal/scheduler/onnx/artifact_test.go
key-decisions:
  - "Anomaly thresholds are computed from successful samples only; failure and timeout rows remain evidence."
patterns-established:
  - "Nested anomaly_thresholds[task_type][coverage_level] manifest metadata"
requirements-completed: ["ANOM-01"]
duration: 45 min
completed: 2026-07-05
---

# Phase 18 Plan 01: Anomaly Artifact Metadata Summary

**Offline scheduler training now publishes conservative anomaly thresholds in normal ONNX artifact manifests.**

## Performance

- **Duration:** 45 min
- **Started:** 2026-07-05T09:00:00Z
- **Completed:** 2026-07-05T09:45:00Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- Added deterministic anomaly distance and threshold computation with mean+3*stddev vs p95 max.
- Published `anomaly_thresholds` and `anomaly_evidence` into `manifest.json`.
- Added Go manifest structs/tests for nested anomaly threshold metadata.

## Task Commits

1. **Tasks 1-3: Publish anomaly metadata** - `42890f4e` (`feat(18-01)`)

## Verification

- `uv run pytest tests/test_train_publish.py tests/test_export_schema.py` - passed from `tools/scheduler_training`
- `go test -timeout 60s ./internal/scheduler/onnx` - passed

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

The first Python test run from repo root failed because `pytest` is a dev dependency of the `tools/scheduler_training` subproject. Re-ran from that project directory with `uv run pytest`; tests then exercised the real project environment.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Runtime ONNX loading can read anomaly metadata; 18-02 can validate and consume it.

---
*Phase: 18-anomaly-and-ood-conservative-scoring*
*Completed: 2026-07-05*
