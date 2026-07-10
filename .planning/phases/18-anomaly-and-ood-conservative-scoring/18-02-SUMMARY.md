---
phase: 18-anomaly-and-ood-conservative-scoring
plan: "02"
subsystem: scheduler-runtime
tags: [onnx, anomaly, scoring, metrics]
requires:
  - phase: 18-anomaly-and-ood-conservative-scoring
    provides: anomaly threshold manifest metadata
provides:
  - anomaly metadata availability/degraded state
  - OOD confidence and uncertainty adjustment
  - low-cardinality anomaly status metrics
affects: [scheduler, observability, onnx]
tech-stack:
  added: []
  patterns: [optional artifact degradation, local score metadata]
key-files:
  created: []
  modified:
    - internal/scheduler/onnx/artifact.go
    - internal/scheduler/onnx/scorer.go
    - internal/scheduler/types.go
    - internal/scheduler/intake.go
    - internal/observability/metrics.go
    - internal/observability/prometheus.go
key-decisions:
  - "Scheduler RPC stays unchanged; anomaly status is local metadata/metrics only."
patterns-established:
  - "Invalid anomaly metadata degrades anomaly behavior without disabling ONNX scoring"
requirements-completed: ["ANOM-02", "ANOM-03"]
duration: 55 min
completed: 2026-07-05
---

# Phase 18 Plan 02: Runtime Conservative Scoring Summary

**ONNX scoring now treats out-of-distribution tasks conservatively by lowering confidence and raising uncertainty.**

## Performance

- **Duration:** 55 min
- **Started:** 2026-07-05T09:45:00Z
- **Completed:** 2026-07-05T10:40:00Z
- **Tasks:** 3
- **Files modified:** 9

## Accomplishments

- Added optional anomaly metadata validation with `available`, `unavailable`, and `degraded` states.
- Applied relative threshold exceedance to confidence and uncertainty before heuristic scoring.
- Added sanitized anomaly status metrics and task metadata for later quality evidence.

## Task Commits

1. **Tasks 1-3: Apply anomaly status to ONNX scoring** - `ab2dfa29` (`feat(18-02)`)
2. **Review fix: Expose scheduler anomaly status** - `1c9d555` (`fix(18-02)`)

## Verification

- `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/observability ./cmd/scheduler` - passed

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Exposed anomaly state in scheduler status**
- **Found during:** Code review gate
- **Issue:** Runtime artifact/scorer emitted anomaly state, but `cmd/scheduler` did not expose status output required by D-09/D-10.
- **Fix:** Added low-cardinality startup log and `/status` JSON fields for `anomaly_status` and `anomaly_reason`.
- **Files modified:** cmd/scheduler/main.go, cmd/scheduler/main_test.go, internal/scheduler/onnx/scorer.go
- **Verification:** `go test -timeout 60s ./cmd/scheduler ./internal/scheduler/onnx`
- **Committed in:** `1c9d555`

**Total deviations:** 1 auto-fixed missing critical item.
**Impact on plan:** Completes the planned visibility requirement without changing Scheduler RPC.

## Issues Encountered

One scorer test initially compared artifacts with different p70 model parameters; fixed the test fixture so only anomaly metadata differed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Score metadata includes anomaly status for in-process quality recording, and metrics expose sanitized anomaly status labels.

---
*Phase: 18-anomaly-and-ood-conservative-scoring*
*Completed: 2026-07-05*
