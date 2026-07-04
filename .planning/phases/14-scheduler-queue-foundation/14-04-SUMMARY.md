---
phase: 14-scheduler-queue-foundation
plan: 14-04
subsystem: scheduler
tags: [priority, queue, gateway, prometheus]
requires:
  - phase: 14-01
    provides: scheduler.v1 contract, config, and FIFO-safe gRPC scorer
  - phase: 14-02
    provides: QueueBackend, ResultRegistry, Redis queue, memory fallback, and guard
  - phase: 14-03
    provides: safe feature extraction and heuristic Scheduler scoring service
provides:
  - trusted high/normal/low priority resolver and downgrade policy
  - internal TaskIntake that scores, guards, and enqueues safe task metadata
  - synchronous Gateway runner over the queue/result registry core
  - Gateway app wiring for Redis, memory, gRPC scorer, and FIFO fallback modes
  - low-cardinality scheduler, queue, breaker, and priority Prometheus metrics
affects: [phase-14, gateway-data-plane, scheduler-observability]
tech-stack:
  added: []
  patterns: [trusted priority seam, synchronous facade over queue core, sanitized metric labels]
key-files:
  created:
    - internal/scheduler/priority.go
    - internal/scheduler/priority_test.go
    - internal/scheduler/intake.go
    - internal/scheduler/intake_test.go
    - internal/scheduler/executor.go
  modified:
    - internal/admission/controller.go
    - internal/admission/controller_test.go
    - internal/app/app.go
    - internal/app/app_test.go
    - internal/gateway/service.go
    - internal/gateway/service_test.go
    - internal/http/handlers/chat.go
    - internal/llm/types.go
    - internal/observability/metrics.go
    - internal/observability/prometheus.go
    - internal/observability/prometheus_test.go
    - internal/scheduler/result_registry.go
key-decisions:
  - "Priority is resolved at one trusted seam and remains limited to high, normal, or low."
  - "Gateway exposes the queue internally through a synchronous runner so OpenAI-compatible clients keep the same blocking API."
  - "app.New always constructs a safe runner: Redis when healthy, memory fallback when Redis is unavailable, and FIFO scoring when Scheduler is disabled."
  - "Metrics labels are allowlisted and avoid request IDs, task IDs, tenant IDs, prompt text, auth material, and scheduler topology."
patterns-established:
  - "SynchronousRunner submits only safe task metadata and waits for one in-memory TaskResult."
  - "Priority downgrade details are visible through sanitized metrics, not ordinary data-plane responses."
requirements-completed: [SCH-01, SCH-02, SCH-03, SCH-04, PRIO-01, PRIO-02, OBS-01]
duration: 1h
completed: 2026-07-04
---

# Phase 14 Plan 04: Priority and Observability Summary

**Trusted priority resolution, synchronous queued Gateway execution, and sanitized scheduler observability**

## Performance

- **Duration:** 1h
- **Started:** 2026-07-04T08:35:00-07:00
- **Completed:** 2026-07-04T09:36:29-07:00
- **Tasks:** 3
- **Files modified:** 17

## Accomplishments

- Added a trusted priority resolver with max-priority policy, high-priority quota downgrade, and compatibility normalization for legacy admission terms.
- Added internal task intake and a synchronous queue runner that preserve the existing OpenAI-compatible non-stream and stream response paths.
- Wired app startup to use Redis queue when configured and healthy, memory fallback when Redis is unavailable, and FIFO scoring when Scheduler is disabled.
- Added sanitized queue, scheduler, breaker, classification, wait-time, and priority downgrade Prometheus metrics.
- Verified the restored real PostgreSQL-backed test environment and required package gates without mock clients or skipped component tests.

## Task Commits

1. **Task 1: Implement trusted priority resolver and downgrade policy** - `e2bbe4df`
2. **Task 2: Wire internal queue/executor through Gateway sync and stream paths** - `e2bbe4df`
3. **Task 3: Add sanitized queue, scheduler, and priority observability** - `e2bbe4df`

**Plan metadata:** pending in docs commit

## Files Created/Modified

- `internal/scheduler/priority.go` - Trusted priority resolution, max priority enforcement, quota downgrade, and legacy compatibility seam.
- `internal/scheduler/priority_test.go` - Priority class, prompt-safety, policy downgrade, and quota tests.
- `internal/scheduler/intake.go` - Safe task intake, scoring fallback, queue guard, and enqueue path.
- `internal/scheduler/intake_test.go` - FIFO fallback, guard, metrics, and intake behavior tests.
- `internal/scheduler/executor.go` - Executor and synchronous runner over QueueBackend and ResultRegistry.
- `internal/scheduler/result_registry.go` - Handler lookup and cleanup for queued task execution.
- `internal/admission/controller.go` - Scheduler-facing priority compatibility at the admission boundary.
- `internal/admission/controller_test.go` - Regression coverage that high priority does not bypass admission rejection.
- `internal/gateway/service.go` - Optional scheduler runner integration for non-stream and stream chat paths.
- `internal/gateway/service_test.go` - Response-shape, streaming, queue wait, and header/topology safety tests.
- `internal/http/handlers/chat.go` - Preserves the existing handler path while allowing queue wait response metadata.
- `internal/app/app.go` - Runner construction for Redis, memory fallback, gRPC scorer, and FIFO fallback.
- `internal/app/app_test.go` - Startup fallback coverage with degraded Redis/Scheduler paths.
- `internal/llm/types.go` - Queue wait timing field used by the synchronous response path.
- `internal/observability/metrics.go` - Scheduler and queue metric methods.
- `internal/observability/prometheus.go` - Low-cardinality Prometheus metric implementations.
- `internal/observability/prometheus_test.go` - Metric gather and label allowlist tests.

## Decisions Made

- Kept the Gateway constructor nil-safe: existing direct execution remains unchanged unless a scheduler runner is supplied.
- Kept priority classes exactly `high`, `normal`, and `low`; legacy `interactive`, `batch`, and `background` terms are normalized only at admission.
- Kept queued task storage to task IDs, scores, safe features, and in-memory callbacks; raw prompts and auth material stay out of Redis, logs, and metrics.
- Chose silent priority downgrade with sanitized metric reasons instead of exposing policy details in response headers or bodies.

## Deviations from Plan

### Auto-fixed Issues

**1. Added queue wait timing to LLM response metadata**
- **Found during:** Task 2 (Gateway synchronous queued execution)
- **Issue:** The plan allowed `X-Queue-Wait-Ms`, but `LLMResponse` had no field to carry queue wait duration back to the handler path.
- **Fix:** Added `QueueWaitMs` to `internal/llm/types.go`.
- **Files modified:** `internal/llm/types.go`
- **Verification:** Gateway tests and combined package tests passed.
- **Committed in:** `e2bbe4df`

**2. Added handler lookup to ResultRegistry**
- **Found during:** Task 2 (Executor/synchronous runner integration)
- **Issue:** The queued executor needed a process-local callback lookup so Redis and memory queues only carry task IDs and scores.
- **Fix:** Added `TaskHandler`, handler registration, lookup, and unregister cleanup.
- **Files modified:** `internal/scheduler/result_registry.go`
- **Verification:** Scheduler and gateway tests passed, including cancellation cleanup.
- **Committed in:** `e2bbe4df`

---

**Total deviations:** 2 auto-fixed integration gaps.
**Impact on plan:** Both changes were required to preserve the planned synchronous facade without storing raw request payloads in the queue.

## Issues Encountered

- PostgreSQL was unavailable during the first verification attempt. After the user restored PostgreSQL, the real component test gates were rerun successfully.

## Verification

- `cmd.exe /c "set GOCACHE=%TEMP%\\codex-go-build-veloxmesh&& go test -count=1 -timeout 60s ./internal/app"` - passed after PostgreSQL was restored.
- `cmd.exe /c "set GOCACHE=%TEMP%\\codex-go-build-veloxmesh&& go test -count=1 -timeout 60s ./internal/scheduler ./internal/admission ./internal/gateway ./internal/http/handlers ./internal/app ./internal/observability"` - passed after PostgreSQL was restored.
- `rg "urgent" internal/scheduler internal/admission` - no matches.
- `rg "queue_depth|scheduler_id|scheduler_type|scheduler_version" internal/http/handlers internal/gateway` - no matches.
- `rg "request_id|task_id|tenant_id|prompt|message|provider_secret|api_key|authorization" internal/observability/prometheus.go` - no matches.

## User Setup Required

Operators enabling Scheduler must run the Scheduler service and configure `SCHEDULER_ENDPOINT`. Gateway remains FIFO-safe when Scheduler is omitted or unavailable.

Developer setup for protobuf generation remains documented in `14-CONTEXT.md` and `14-01-SUMMARY.md`: use `C:\Soft\1A-Coding\protoc-35.1-win64\bin\protoc.exe` with `protoc-gen-go` and `protoc-gen-go-grpc` on `PATH`.

## Next Phase Readiness

Phase 14 now has the optional Scheduler queue foundation, heuristic scorer, trusted priority safety, and sanitized observability needed for Phase 15 training feedback and ONNX path work.

---
*Phase: 14-scheduler-queue-foundation*
*Completed: 2026-07-04*
