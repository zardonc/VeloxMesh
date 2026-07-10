---
phase: 23
slug: scheduler-queue-hardening
status: passed
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-08
validated: 2026-07-08
---

# Phase 23 - Validation Strategy

Per-phase validation contract for Scheduler queue hardening.

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | go test |
| Config file | `.env` loaded by real Redis queue tests |
| Quick run command | `go test -v -timeout 60s ./internal/scheduler -run 'TestRedisQueue|TestRedisTaskLocker|TestExecutorSkipsRedisLockedTask|TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure' -count=1` |
| Full suite command | `go test -timeout 60s ./...` |
| Estimated runtime | ~8 seconds targeted, ~12 seconds full suite |

## Sampling Rate

- After every task commit: run the quick command.
- After every plan wave: run `go test -timeout 60s ./internal/app ./internal/scheduler`.
- Before `$gsd-verify-work`: run `go test -timeout 60s ./...` and `go build ./...`.
- Max feedback latency: 60 seconds.

## Real Component Evidence

| Component | Command | Result |
|-----------|---------|--------|
| Redis Scheduler queue | `go test -v -timeout 60s ./internal/scheduler -run 'TestRedisQueue|TestRedisTaskLocker|TestExecutorSkipsRedisLockedTask|TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure' -count=1` | Passed: `TestRedisQueueRealZSetOperations`, `TestRedisQueueStoresOnlyTaskIDMember`, `TestRedisQueuePeekMinDoesNotPopAndPushReplacesScore`, `TestRedisTaskLockerUsesSetNXAndTTL`, `TestExecutorSkipsRedisLockedTaskWithoutDelivery`, `TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure` |
| Backend full regression | `go test -timeout 60s ./...` | Passed |
| Build | `go build ./...` | Passed |

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 23-01-01 | 01 | 1 | SCHQ-01 | component/unit | `go test -timeout 60s ./internal/app -run TestNewSchedulerQueueDefaultsToMemoryWhenRedisIsEnabled -count=1` | yes | green |
| 23-01-01 | 01 | 1 | SCHQ-02 | component/unit | `go test -timeout 60s ./internal/app -run TestNewSchedulerQueueExplicitRedisIsNodeScoped -count=1` | yes | green |
| 23-01-02 | 01 | 1 | SCHQ-03 | component/unit | `go test -timeout 60s ./internal/scheduler -run 'TestFallbackQueuePopMinReadsMemoryWhenPrimaryEmptyAfterRecovery|TestFallbackQueuePopMinMergesPrimaryAndFallback' -count=1` | yes | green |
| 23-01-03 | 01 | 1 | SCHQ-04 | real component | `go test -v -timeout 60s ./internal/scheduler -run 'TestRedisQueue|TestRedisTaskLocker|TestExecutorSkipsRedisLockedTask|TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure' -count=1` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

## Manual-Only Verifications

All phase behaviors have automated verification.

## Validation Audit 2026-07-08

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

## Validation Sign-Off

- [x] All tasks have automated verify commands.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing references.
- [x] No watch-mode flags.
- [x] Feedback latency under 60 seconds.
- [x] `nyquist_compliant: true` set in frontmatter.

Approval: approved 2026-07-08

