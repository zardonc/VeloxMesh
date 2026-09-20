---
phase: 19-sla-waiting-time-promotion
plan: "01"
subsystem: config
tags: [scheduler, sla-promotion, config, validation]
requires:
  - phase: 14-scheduler-queue-foundation
    provides: gateway-owned scheduler config and priority boundaries
provides:
  - Disabled-by-default scheduler SLA promotion config fields.
  - JSON and env loading for promotion enablement and candidate window.
  - Enabled-only validation for promotion rules.
affects: [scheduler, config, app-wiring]
tech-stack:
  added: []
  patterns: [disabled-by-default scheduler enhancement config, enabled-only validation]
key-files:
  created: []
  modified:
    - internal/config/config.go
    - internal/config/config_validation.go
    - internal/config/config_test.go
    - internal/config/env.go
key-decisions:
  - "SLA promotion rules are inert unless scheduler.sla_promotion_enabled is true."
  - "Request kind validation stays in config as a string allow-list to avoid an import cycle with scheduler."
patterns-established:
  - "Optional scheduler enhancements default through applySchedulerDefaults and validate only active behavior."
requirements-completed: ["SLA-01"]
duration: 19 min
completed: 2026-07-05
---

# Phase 19 Plan 01: Promotion Policy Config Summary

**Disabled-by-default SLA promotion policy config with JSON/env loading and enabled-only rule validation.**

## Performance

- **Duration:** 19 min
- **Started:** 2026-07-05T21:10:00Z
- **Completed:** 2026-07-05T21:29:01Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Added `SchedulerConfig` fields for SLA promotion enablement, candidate window, and operator rules.
- Loaded `SCHEDULER_SLA_PROMOTION_ENABLED`, `SCHEDULER_SLA_PROMOTION_CANDIDATE_WINDOW`, and JSON `scheduler.sla_promotion_rules`.
- Added validation that rejects malformed enabled rules while keeping disabled promotion harmless.

## Task Commits

1. **Task 1 RED: Add failing SLA promotion config tests** - `10c0a71d` (test)
2. **Task 1 GREEN: Add SLA promotion config surface** - `77ac3503` (feat)
3. **Task 2 RED: Add failing SLA promotion validation tests** - `a6678868` (test)
4. **Task 2 GREEN: Validate enabled SLA promotion rules** - `a8ea2acd` (feat)

## Files Created/Modified

- `internal/config/config.go` - Adds SLA promotion config fields, JSON merge behavior, and env loading.
- `internal/config/config_validation.go` - Adds enabled-only SLA rule validation.
- `internal/config/config_test.go` - Covers defaults, env, JSON rules, enabled validation failures, and disabled malformed rules.
- `internal/config/env.go` - Adds the named default candidate window constant.

## Decisions Made

- Kept malformed rule validation gated behind `SLAPromotionEnabled` so dormant config remains harmless.
- Used a config-local request-kind allow-list because `internal/scheduler` already imports `internal/config`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Placed default constant in existing config constants file**
- **Found during:** Task 1 (Add scheduler SLA promotion config fields and defaults)
- **Issue:** The plan named `defaultSLAPromotionCandidateWindow` but not its file location.
- **Fix:** Added it to `internal/config/env.go` beside the existing default config constants.
- **Files modified:** `internal/config/env.go`
- **Verification:** `go test -timeout 60s ./internal/config`; `go build ./...`
- **Committed in:** `77ac3503`

---

**Total deviations:** 1 auto-fixed (missing critical placement detail).
**Impact on plan:** No behavior change; keeps constants in the existing config pattern.

## Issues Encountered

None.

## Verification

- `go test -timeout 60s ./internal/config` - passed
- `go build ./...` - passed

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for 19-02 queue promotion runtime wiring.

---
*Phase: 19-sla-waiting-time-promotion*
*Completed: 2026-07-05*
