---
phase: 25
slug: runbooks-and-verification
status: passed
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-08
validated: 2026-07-08
---

# Phase 25 - Validation Strategy

Per-phase validation contract for v7.7 runbooks and verification.

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | go test plus ripgrep source assertions |
| Config file | `.env` loaded by real Redis, Qdrant, and Postgres tests |
| Quick run command | `rg -n "SCHEDULER_QUEUE_BACKEND|FallbackQueue|Plan 3|LanceDB|Qdrant" README.md docs/scheduler-1.0-runbook.md .env.example` |
| Full suite command | `go test -timeout 60s ./...` |
| Estimated runtime | ~1 second quick, ~12 seconds full suite |

## Sampling Rate

- After every task commit: run the quick command.
- After every plan wave: run the quick command plus the real component commands listed below.
- Before `$gsd-verify-work`: run `go test -timeout 60s ./...` and `go build ./...`.
- Max feedback latency: 60 seconds.

## Real Component Evidence

| Component | Command | Result |
|-----------|---------|--------|
| Redis queue/runbook behavior | `go test -v -timeout 60s ./internal/scheduler -run 'TestRedisQueue|TestRedisTaskLocker|TestExecutorSkipsRedisLockedTask|TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure' -count=1` | Passed |
| Redis hotstate runbook behavior | `go test -v -timeout 60s ./tests/integration -run 'TestRedisHotState' -count=1` | Passed: pubsub, byte cache, atomic limiter, session blacklist |
| Qdrant and pgvector vector behavior | `go test -v -timeout 60s ./internal/storage -run 'TestQdrant|TestPGVector' -count=1` | Passed |
| App semantic-neighbor startup | `go test -v -timeout 60s ./internal/app -run 'TestApp_SemanticNeighborsEnsureCollectionKeepsServiceEnabled|TestApp_SemanticNeighborsPGVectorEnsureKeepsServiceEnabled' -count=1` | Passed |
| Plan4 smoke | `go test -v -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1` | Not counted: skipped because PLAN4_* provider smoke variables were not present in the process environment |
| Backend full regression | `go test -timeout 60s ./...` | Passed |
| Build | `go build ./...` | Passed |

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 25-01-01 | 01 | 1 | DOC-01 | docs/source assertion | `rg -n "SCHEDULER_QUEUE_BACKEND|FallbackQueue|Redis recovers|defaults to memory" docs/scheduler-1.0-runbook.md README.md` | yes | green |
| 25-01-01 | 01 | 1 | DOC-01 | real component | `go test -v -timeout 60s ./internal/scheduler -run 'TestRedisQueue|TestRedisTaskLocker|TestExecutorSkipsRedisLockedTask|TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure' -count=1` | yes | green |
| 25-01-02 | 01 | 1 | DOC-02 | docs/source assertion | `rg -n "Plan 1|Plan 3|LanceDB|Qdrant|single-node|migration" README.md docs/scheduler-1.0-runbook.md .env.example` | yes | green |
| 25-01-02 | 01 | 1 | DOC-02 | real component | `go test -v -timeout 60s ./internal/storage -run 'TestQdrant|TestPGVector' -count=1` | yes | green |
| 25-01-03 | 01 | 1 | DOC-02 | regression | `go test -timeout 60s ./... && go build ./...` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real external Plan4 provider smoke | DOC-02 | The smoke requires PLAN4_* or SANS_* provider credentials and is not needed to prove queue/vector docs. It was intentionally not counted as automated evidence in this validation. | Provide `POSTGRES_TEST_DSN`, `PLAN4_CONTROL_STATE_ENCRYPTION_KEY`, `PLAN4_PROVIDER_API_KEY`, and `PLAN4_DEV_API_KEY`, then run `go test -v -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1`. |

## Validation Audit 2026-07-08

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 1 external-provider optional smoke |

## Validation Sign-Off

- [x] All tasks have automated verify commands or documented manual-only limits.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing references.
- [x] No watch-mode flags.
- [x] Feedback latency under 60 seconds.
- [x] `nyquist_compliant: true` set in frontmatter.

Approval: approved 2026-07-08

