---
phase: 16-a-b-rollout-and-prediction-quality
plan: 16-03
subsystem: scheduler
tags: [scheduler, rollout, admin, alerts, observability]
requires:
  - phase: 16-a-b-rollout-and-prediction-quality
    provides: Weighted scheduler routing and prediction quality rollups
provides:
  - Runtime scheduler rollout controller
  - Authenticated admin rollout status and update API
  - Manual rollback alerts for ONNX MAPE degradation and scheduler error spikes
affects: [scheduler, config, observability, http, app, docs]
tech-stack:
  added: []
  patterns: [runtime controller snapshot, sanitized admin audit metadata, manual rollback alerting]
key-files:
  created:
    - internal/scheduler/rollout_control.go
    - internal/scheduler/admin_scheduler_service.go
    - internal/http/handlers/admin_scheduler.go
  modified:
    - internal/config/config.go
    - internal/config/config_validation.go
    - internal/scheduler/client.go
    - internal/scheduler/quality.go
    - internal/observability/metrics.go
    - internal/observability/prometheus.go
    - internal/app/app.go
    - internal/http/router.go
    - README.md
    - .env.example
key-decisions:
  - "Kept rollback manual: alerts never change ONNX rollout percent automatically."
  - "Placed AdminSchedulerService in internal/scheduler to avoid a scheduler/controlstate import cycle."
  - "Kept emergency FIFO bypass as SCHEDULER_ENABLED=false instead of adding a dedicated kill-switch field."
patterns-established:
  - "Admin rollout mutation audit metadata records old_percent and new_percent only."
requirements-completed: [OBS-02, ML-03]
duration: 39 min
completed: 2026-07-04
---

# Phase 16 Plan 03: Scheduler Rollout Controls Summary

**Runtime ONNX rollout control, authenticated admin rollback API, and operator alert signals**

## Performance

- **Duration:** 39 min
- **Started:** 2026-07-04T21:37:00Z
- **Completed:** 2026-07-04T22:16:00Z
- **Tasks:** 3
- **Files modified:** 19

## Accomplishments

- Added `SchedulerRolloutController` with immutable snapshots and runtime ONNX percent updates consumed by `WeightedScorer`.
- Added admin rollout status/update endpoints behind existing admin auth and writable middleware.
- Added rollout alerts for ONNX MAPE degradation and scheduler error spikes without automatic rollback.
- Documented manual rollback as setting `onnx_rollout_percent` to `0`; emergency FIFO bypass remains `SCHEDULER_ENABLED=false`.

## Task Commits

1. **Task 1: Add runtime rollout controller and alert thresholds** - `38e8be19` (feat)
2. **Task 2: Expose authenticated admin rollout status and update endpoints** - `fa040fa2` (feat)
3. **Task 3: Surface rollback alerts without automatic rollout changes** - `8d2c83d8` (feat)

## Files Created/Modified

- `internal/scheduler/rollout_control.go` - Added runtime rollout status, percent updates, and alert snapshots.
- `internal/scheduler/admin_scheduler_service.go` - Added rollout status/update service and sanitized audit events.
- `internal/http/handlers/admin_scheduler.go` - Added GET/PATCH rollout handlers with exact JSON body validation.
- `internal/scheduler/client.go` - Made weighted scoring read current rollout percent at score time.
- `internal/scheduler/quality.go` - Added MAPE and error-spike alert recording without changing rollout percent.
- `internal/observability/metrics.go` - Added rollout alert metric interface.
- `internal/observability/prometheus.go` - Added `gateway_scheduler_rollout_alerts_total`.
- `internal/app/app.go` - Wired shared rollout controller into scheduler scoring, quality recording, and admin handlers.
- `internal/http/router.go` - Registered admin rollout routes behind existing admin middleware.
- `README.md` and `.env.example` - Added placeholder-only rollout and rollback guidance.

## Decisions Made

- Kept rollout status aggregate-only: no tenant/API-key identity, raw task details, prompts, payloads, secrets, or authorization material.
- Used existing admin auth and writable middleware rather than adding a new control plane.
- Alerting records status and metrics only; operators still perform rollback manually.

## Deviations from Plan

- Moved the admin scheduler service from `internal/controlstate` to `internal/scheduler` to avoid a Go import cycle with scheduler rollout types and quality rollups.

## Issues Encountered

- Initial service placement created `scheduler -> controlstate -> scheduler`; moving the service to `internal/scheduler` resolved the cycle.

## User Setup Required

Use `PATCH /admin/scheduler/rollout` with `{"onnx_rollout_percent":0}` for runtime rollback. Use `SCHEDULER_ENABLED=false` only for the existing emergency FIFO bypass.

## Verification

- `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/observability ./internal/controlstate ./internal/http/handlers ./internal/app`
- `rg "KillSwitch|kill_switch|emergency_switch|SchedulerKill|scheduler_kill" internal README.md .env.example`
- `rg "tenant|api_key|authorization|secret|prompt|message|payload" internal/http/handlers/admin_scheduler.go internal/scheduler/admin_scheduler_service.go`

## Self-Check: PASSED

All task acceptance criteria passed.

## Next Phase Readiness

Phase 16 now has weighted rollout, quality evidence, runtime rollback controls, and alert visibility for final phase verification.

---
*Phase: 16-a-b-rollout-and-prediction-quality*
*Completed: 2026-07-04*
