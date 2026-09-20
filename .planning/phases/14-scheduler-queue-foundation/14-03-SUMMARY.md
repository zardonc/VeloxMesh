---
phase: 14-scheduler-queue-foundation
plan: 14-03
subsystem: scheduler
tags: [heuristic, grpc, prometheus, scoring]
requires:
  - phase: 14-01
    provides: scheduler.v1 gRPC contract and Gateway scorer
  - phase: 14-02
    provides: queue backend score consumer
provides:
  - bounded Gateway-local feature extraction
  - bounded request_kind classifier
  - static virtual deadline score calculator
  - stateless heuristic Scheduler gRPC service
  - scheduler binary with HTTP /health and /metrics
affects: [phase-14, scheduler-service, scoring]
tech-stack:
  added: []
  patterns: [bounded feature extraction, static virtual deadline, low-cardinality scheduler metrics]
key-files:
  created:
    - internal/scheduler/features.go
    - internal/scheduler/features_test.go
    - internal/scheduler/request_kind.go
    - internal/scheduler/request_kind_test.go
    - internal/scheduler/heuristic/config.go
    - internal/scheduler/heuristic/classifier.go
    - internal/scheduler/heuristic/score.go
    - internal/scheduler/heuristic/score_test.go
    - internal/scheduler/heuristic/server.go
    - internal/scheduler/heuristic/server_test.go
    - internal/scheduler/heuristic/metrics.go
    - cmd/scheduler/main.go
    - cmd/scheduler/main_test.go
  modified: []
key-decisions:
  - "Prompt-derived features remain bounded counters/enums and cannot change trusted priority."
  - "Scheduler scoring stays stateless and table/rule based for Phase 14."
patterns-established:
  - "Heuristic BatchScoreTasks can be verified through a real localhost TCP gRPC server."
  - "Scheduler HTTP health and Prometheus metrics are served by the standalone scheduler binary."
requirements-completed: [SCH-04, SCORE-01, SCORE-02]
duration: 45m
completed: 2026-07-04
---

# Phase 14 Plan 03: Heuristic Scheduler Summary

**Bounded feature extraction, static virtual deadline scoring, and standalone heuristic Scheduler service**

## Performance

- **Duration:** 45 min
- **Started:** 2026-07-04T07:50:00Z
- **Completed:** 2026-07-04T08:35:00Z
- **Tasks:** 3
- **Files modified:** 13

## Accomplishments

- Added deterministic local feature extraction for bounded numeric/categorical scheduling fields.
- Added request kind classification for the documented Phase 14 enum values.
- Added heuristic score calculation using enqueue time, predicted latency, priority multiplier, and uncertainty penalty.
- Added stateless `scheduler.v1` gRPC service and `cmd/scheduler` HTTP `/health` and `/metrics` endpoints.
- Verified gRPC service behavior through real localhost TCP gRPC calls.

## Task Commits

1. **Task 1: Implement bounded local feature extraction and request_kind** - `b423442`
2. **Task 2: Implement heuristic classifier and static virtual deadline scoring** - `b423442`
3. **Task 3: Expose heuristic Scheduler gRPC, health, and metrics** - `b423442`

**Plan metadata:** pending in docs commit

## Files Created/Modified

- `internal/scheduler/features.go` - Safe feature extraction from in-memory LLM requests.
- `internal/scheduler/features_test.go` - Bounded counter and priority-boundary tests.
- `internal/scheduler/request_kind.go` - Bounded request kind classifier.
- `internal/scheduler/request_kind_test.go` - Coverage for all documented request kinds.
- `internal/scheduler/heuristic/config.go` - Heuristic latency and multiplier defaults.
- `internal/scheduler/heuristic/classifier.go` - Structured/rule/fallback classifier.
- `internal/scheduler/heuristic/score.go` - Static virtual deadline score calculator.
- `internal/scheduler/heuristic/score_test.go` - Aging, priority, and uncertainty tests.
- `internal/scheduler/heuristic/server.go` - BatchScoreTasks service.
- `internal/scheduler/heuristic/server_test.go` - Real TCP gRPC service tests.
- `internal/scheduler/heuristic/metrics.go` - Low-cardinality Prometheus metrics.
- `cmd/scheduler/main.go` - Standalone scheduler entrypoint.
- `cmd/scheduler/main_test.go` - HTTP health and metrics tests.

## Decisions Made

- Kept the heuristic service free of Redis, SQLite, Qdrant, provider adapters, and model loading.
- Used existing Prometheus dependency and stdlib HTTP mux for the scheduler binary.
- Put classification source into the response reason field and metrics label; task IDs and request content are not metric labels.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `cmd.exe` passed quoted `-run` regexes as literals in this shell, so package-level `go test` commands were used for reliable verification.

## Verification

- `go test -timeout 60s ./internal/scheduler` - passed
- `go test -timeout 60s ./internal/scheduler/heuristic` - passed
- `go test -timeout 60s ./cmd/scheduler` - passed
- `go test -timeout 60s ./internal/scheduler ./internal/scheduler/heuristic ./cmd/scheduler` - passed
- `rg "urgent|onnx|qdrant|training|redis stream|score rewrite" internal/scheduler/heuristic` - no matches
- `rg "First|Snippet|Raw|PromptText|MessageText" internal/scheduler/features.go internal/scheduler/request_kind.go` - no matches
- `rg "task_id|prompt|message|tenant|secret|api_key" internal/scheduler/heuristic/metrics.go cmd/scheduler/main.go` - no matches

## User Setup Required

Operators may run the scheduler binary separately and set `SCHEDULER_ENDPOINT` to its gRPC address. Gateway still works without it.

## Next Phase Readiness

Ready for 14-04 Priority and observability. The scheduler can now score batches and expose basic health/metrics.

---
*Phase: 14-scheduler-queue-foundation*
*Completed: 2026-07-04*
