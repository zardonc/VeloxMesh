---
phase: 14-scheduler-queue-foundation
plan: 14-01
subsystem: scheduler
tags: [grpc, protobuf, config, fallback, scheduler]
requires:
  - phase: 13-postgresql-compatibility
    provides: shipped gateway baseline for v7.4 scheduler work
provides:
  - scheduler.v1 BatchScoreTasks protobuf contract
  - disabled-by-default Scheduler configuration
  - Gateway Scheduler gRPC scorer with FIFO fallback
  - real TCP gRPC client verification for timeout, breaker, and partial responses
affects: [phase-14, scheduler, gateway-config]
tech-stack:
  added: [protoc-gen-go, protoc-gen-go-grpc]
  patterns: [disabled optional service, FIFO fallback, real TCP gRPC tests]
key-files:
  created:
    - proto/scheduler/v1/scheduler.proto
    - internal/scheduler/schedulerv1/scheduler.pb.go
    - internal/scheduler/schedulerv1/scheduler_grpc.pb.go
    - internal/scheduler/types.go
    - internal/scheduler/client.go
    - internal/scheduler/client_test.go
  modified:
    - internal/config/config.go
    - internal/config/config_validation.go
    - internal/config/env.go
    - internal/config/config_test.go
    - .env.example
    - .planning/phases/14-scheduler-queue-foundation/14-CONTEXT.md
    - .planning/phases/14-scheduler-queue-foundation/14-01-PLAN.md
key-decisions:
  - "Scheduler protobuf generation uses C:\\Soft\\1A-Coding\\protoc-35.1-win64\\bin\\protoc.exe with Go generator plugins on PATH."
  - "Scheduler service verification must use real components and real network calls; mock clients/data and skipped necessary tests are disallowed."
patterns-established:
  - "Optional Scheduler services return FIFO scores instead of blocking startup or request scoring."
  - "Scheduler integration tests start a real localhost TCP gRPC service and call it through the generated client."
requirements-completed: [SCH-01, SCH-02]
duration: 1h 20m
completed: 2026-07-04
---

# Phase 14 Plan 01: Proto, Config, and Client Fallback Summary

**Scheduler gRPC contract, disabled-by-default config, and real TCP-tested FIFO fallback client**

## Performance

- **Duration:** 1h 20m
- **Started:** 2026-07-04T05:55:00Z
- **Completed:** 2026-07-04T07:15:00Z
- **Tasks:** 3
- **Files modified:** 14

## Accomplishments

- Added `scheduler.v1.TaskScheduler` with `BatchScoreTasks` and safe bounded feature fields.
- Generated Go protobuf and gRPC bindings using the configured local `protoc.exe`.
- Added Scheduler config defaults and validation with disabled-by-default, 15ms timeout, three priority classes, queue knobs, breaker knobs, and `.env.example` placeholders.
- Added a Gateway-side gRPC scorer that falls back to FIFO when disabled, timing out, returning errors, breaker-open, or missing per-task scores.
- Verified client behavior through real localhost TCP gRPC calls, not injected mock clients.

## Task Commits

1. **Task 1: Define scheduler.v1 and safe Gateway scheduling DTOs** - `e019c9ad`
2. **Task 2: Add disabled-by-default Scheduler configuration** - `e019c9ad`
3. **Task 3: Implement Scheduler scorer client with FIFO fallback** - `e019c9ad`

**Plan metadata:** pending in docs commit

## Files Created/Modified

- `proto/scheduler/v1/scheduler.proto` - Scheduler service and safe feature/result wire contract.
- `internal/scheduler/schedulerv1/scheduler.pb.go` - Generated protobuf messages.
- `internal/scheduler/schedulerv1/scheduler_grpc.pb.go` - Generated gRPC client/server bindings.
- `internal/scheduler/types.go` - Go-native scheduler feature/result DTOs and enums.
- `internal/scheduler/client.go` - FIFO scorer, gRPC scorer, timeout handling, breaker fallback, and per-task missing-score fallback.
- `internal/scheduler/client_test.go` - Real TCP gRPC tests for success, timeout, breaker-open, disabled mode, and partial response fallback.
- `internal/config/config.go` - Scheduler config struct and env/file loading.
- `internal/config/config_validation.go` - Scheduler duration, priority, queue, and concurrency validation.
- `internal/config/env.go` - Float env parsing for score penalty config.
- `internal/config/config_test.go` - Scheduler config defaults, JSON override, validation, and `.env.example` safety checks.
- `.env.example` - Disabled Scheduler placeholder config.
- `.planning/phases/14-scheduler-queue-foundation/14-CONTEXT.md` - Tooling and real-component verification constraints.
- `.planning/phases/14-scheduler-queue-foundation/14-01-PLAN.md` - Protoc path, real network verification constraint, and corrected sensitive-field grep.

## Decisions Made

- Used generated protobuf/gRPC code rather than hand-written service descriptors once `protoc-gen-go` and `protoc-gen-go-grpc` were installed.
- Kept Scheduler disabled when `SCHEDULER_ENABLED=false` or endpoint is empty, returning FIFO without dialing.
- Used localhost TCP gRPC test servers for service verification so timeout and breaker behavior exercise the real generated client.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Corrected sensitive-field grep false positive**
- **Found during:** Task 1 acceptance verification
- **Issue:** The planned grep included singular `message`, which always matches protobuf `message` declarations and makes the acceptance check impossible.
- **Fix:** Tightened the grep to `raw_prompt|messages|authorization|api_key|secret|payload|tool_arguments`, preserving the raw/sensitive field boundary without matching protobuf syntax.
- **Files modified:** `.planning/phases/14-scheduler-queue-foundation/14-01-PLAN.md`
- **Verification:** Corrected grep exits with no matches.
- **Committed in:** `e019c9ad`

---

**Total deviations:** 1 auto-fixed blocking verification issue.
**Impact on plan:** No scope expansion; the safety check now tests transmitted sensitive fields instead of protobuf syntax.

## Issues Encountered

- Default Go build cache was not writable under the sandbox. Tests were run with `GOCACHE=%TEMP%\codex-go-build-veloxmesh`.
- `protoc-gen-go` and `protoc-gen-go-grpc` were not initially on PATH; user installed them under `C:\Users\inthe\go\bin`, then generation succeeded with a temporary PATH update.

## Verification

- `go test -timeout 60s ./internal/config` - passed
- `go test -timeout 60s ./internal/scheduler/...` - passed
- `rg "urgent|interactive|batch|background" proto/scheduler/v1 internal/scheduler/types.go` - no matches
- `rg "raw_prompt|messages|authorization|api_key|secret|payload|tool_arguments" proto/scheduler/v1 internal/scheduler/types.go` - no matches
- `rg 'service TaskScheduler' proto/scheduler/v1/scheduler.proto` - matched
- `rg 'rpc BatchScoreTasks' proto/scheduler/v1/scheduler.proto` - matched

## User Setup Required

None for runtime use; Scheduler remains disabled by default. Developer setup requires `protoc-gen-go` and `protoc-gen-go-grpc` on PATH before regenerating scheduler protobuf bindings.

## Next Phase Readiness

Ready for 14-02 Queue backend. The Gateway has the safe scoring contract and FIFO-safe Scheduler client boundary that queue scoring can call.

---
*Phase: 14-scheduler-queue-foundation*
*Completed: 2026-07-04*
