---
phase: 19-sla-waiting-time-promotion
plan: "02"
subsystem: scheduler
tags: [scheduler, queue, sla-promotion, redis, priority]
requires:
  - phase: 19-sla-waiting-time-promotion
    provides: disabled-by-default SLA promotion config and rule validation
provides:
  - Bounded queue peeking for memory, Redis, and fallback queues.
  - Safe queued task snapshots keyed by task ID.
  - Gateway-owned SLA promotion before queue pop.
affects: [scheduler, app, queue, priority]
tech-stack:
  added: []
  patterns: [bounded pre-pop inspection, QueueBackend.Push score replacement, safe task snapshot registry]
key-files:
  created:
    - internal/scheduler/sla_promotion.go
  modified:
    - internal/scheduler/queue.go
    - internal/scheduler/queue_memory.go
    - internal/scheduler/queue_redis.go
    - internal/scheduler/queue_fallback.go
    - internal/scheduler/result_registry.go
    - internal/scheduler/task.go
    - internal/scheduler/intake.go
    - internal/scheduler/executor.go
    - internal/app/app.go
key-decisions:
  - "Promotion reuses QueueBackend.Push score replacement instead of adding a second queue mutation API."
  - "Promotion decisions read only tenant ID/class, model class, request kind, priority, and enqueue time from safe snapshots."
patterns-established:
  - "Pre-pop scheduler enhancements fail open: promotion errors are ignored and PopMin continues."
requirements-completed: ["SLA-02", "SLA-03"]
duration: 18 min
completed: 2026-07-05
---

# Phase 19 Plan 02: Queue Promotion Runtime Summary

**Bounded SLA promotion reorders eligible queued tasks through existing memory/Redis score replacement without priority escalation.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-07-05T21:30:00Z
- **Completed:** 2026-07-05T21:48:16Z
- **Tasks:** 3
- **Files modified:** 17

## Accomplishments

- Added `QueueBackend.PeekMin` to inspect a bounded candidate window without popping.
- Stored safe task snapshots in `ResultRegistry` using trusted auth identity and existing safe scheduler features.
- Added `SLAPromoter` and wired `Executor.RunOne` to promote once before `PopMin`.

## Task Commits

1. **Task 1 RED: Add failing queue peek tests** - `d4c32853` (test)
2. **Task 1 GREEN: Add bounded queue peeking** - `dda6f6f0` (feat)
3. **Task 2 RED: Add failing safe task snapshot tests** - `108729fb` (test)
4. **Task 2 GREEN: Store safe queued task snapshots** - `5a13ec46` (feat)
5. **Task 3 RED: Add failing SLA promotion runtime tests** - `4de4bf3b` (test)
6. **Task 3 GREEN: Promote SLA candidates before queue pop** - `30f67c87` (feat)

## Files Created/Modified

- `internal/scheduler/sla_promotion.go` - Implements bounded SLA promotion policy.
- `internal/scheduler/queue.go` - Adds the `PeekMin` queue contract.
- `internal/scheduler/queue_memory.go` - Adds non-mutating heap-copy peek.
- `internal/scheduler/queue_redis.go` - Adds Redis ZSET `ZRangeWithScores` peek.
- `internal/scheduler/queue_fallback.go` - Adds primary-to-memory fallback peek.
- `internal/scheduler/result_registry.go` - Stores and returns safe task snapshots.
- `internal/scheduler/task.go` - Adds trusted tenant ID/class fields.
- `internal/scheduler/intake.go` - Registers task snapshots from `AuthIdentity`.
- `internal/scheduler/executor.go` - Calls promoter once before pop.
- `internal/app/app.go` - Wires enabled SLA promotion config into the executor.

## Decisions Made

- Used `math.Nextafter(firstSamePriorityScore, -Inf)` so promoted tasks move just ahead of the first same-priority candidate.
- Kept promotion fail-open: queue peek/push errors do not block normal `PopMin`.
- Kept prompt-derived feature fields out of match and priority decisions.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `verify.key-links` still reports the two 19-02 key links as unverified because it looks for direct file references. The implemented path is interface-based and covered by tests: `Executor -> SLAPromoter -> QueueBackend.PeekMin/Push` and `TaskIntake -> ResultRegistry.RegisterTask`.

## Verification

- `go test -timeout 60s ./internal/scheduler ./internal/app` - passed
- `go build ./...` - passed

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for 19-03 metrics and sanitized durable audit evidence.

---
*Phase: 19-sla-waiting-time-promotion*
*Completed: 2026-07-05*
