---
phase: 26-scheduler-scoring-backpressure-hardening
plan: "02"
subsystem: scheduler
tags: [scheduler, quality, rollout, admin]
key-files:
  - internal/scheduler/quality.go
  - internal/scheduler/rollout_control.go
  - internal/scheduler/executor.go
  - internal/scheduler/admin_scheduler_service.go
  - internal/http/handlers/admin_scheduler_test.go
metrics:
  tests: passed
---

# Plan 26-02 Summary

## What Changed

- Replaced ONNX single-sample MAPE/error-spike alerts with an in-memory sample window.
- Added `scheduler.quality_sample_window` / `SCHEDULER_QUALITY_SAMPLE_WINDOW`, default `100`.
- Added runtime admin PATCH support for `quality_sample_window`.
- Added regression coverage for alert retention and scheduler empty-queue waiting.
- Fixed the executor PopMin-to-MarkRunning gap, then preserved lost-task/queue-unavailable failure behavior.

## Commits

| Commit | Description |
| --- | --- |
| `a04babb` | Harden scheduler quality alerts, admin config, docs, and executor wait behavior. |

## Verification

- `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/http/handlers -count=1` passed.
- `go test -timeout 60s ./internal/scheduler/predictor ./cmd/scheduler -count=1` passed.
- `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/predictive -count=1` passed.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.
- `git diff --check` passed.

## Real Components

- Full test run includes `cmd/scheduler` Python ONNX worker smoke coverage.
- ONNX package tests load the model artifact path.
- Scheduler/predictor tests exercise real local TCP gRPC servers.

## Deviations from Plan

- Auto-fixed a related executor edge case exposed by full verification: `ErrQueueEmpty` now returns queue unavailable when the submitted task is not running, while the PopMin-to-MarkRunning race is closed by marking running immediately after pop.

## Self-Check: PASSED
