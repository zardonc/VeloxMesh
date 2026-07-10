---
phase: 16-a-b-rollout-and-prediction-quality
status: clean
depth: standard-inline
files_reviewed: 44
findings:
  critical: 0
  warning: 0
  info: 0
  total: 0
resolved_during_review: 1
reviewed: 2026-07-04
---

# Phase 16 Code Review

## Scope

Reviewed the Phase 16 source changes across scheduler rollout routing, prediction quality rollups, admin rollout controls, metrics, config, app wiring, and tests.

## Open Findings

None.

## Resolved During Review

### WR-01: ONNX scorer was unavailable after startup at zero rollout

**Severity:** Warning

**Files:** `internal/scheduler/client.go`, `internal/scheduler/client_test.go`

When `SCHEDULER_ONNX_ROLLOUT_PERCENT=0` was set at startup, `NewScorerWithController` returned a heuristic-only scorer instead of a weighted scorer. That meant a later admin update from `0` to a positive rollout percent would update the controller but still never route traffic to ONNX until restart.

**Resolution:** Commit `585eed1` constructs the weighted scorer whenever an ONNX endpoint is configured, even when the initial rollout percent is zero. Added `TestNewScorerWithControllerKeepsONNXAvailableAtZeroRollout` to prove runtime rollout can increase from 0 to 100 without rebuilding the scorer.

## Verification

- `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/observability ./internal/controlstate/... ./internal/http/handlers ./internal/app`
- `rg "KillSwitch|kill_switch|emergency_switch|SchedulerKill|scheduler_kill" internal README.md .env.example`
- `rg "tenant|api_key|authorization|secret|prompt|message|payload" internal/http/handlers/admin_scheduler.go internal/scheduler/admin_scheduler_service.go`

## Result

Phase 16 source changes pass review with no open critical, warning, or info findings.
