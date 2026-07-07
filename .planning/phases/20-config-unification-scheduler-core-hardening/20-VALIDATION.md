---
phase: 20
slug: config-unification-scheduler-core-hardening
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-06
audited: 2026-07-06T16:25:00-07:00
---

# Phase 20 - Validation Strategy

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing` via `go test` |
| Config file | `go.mod` |
| Quick run command | `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/storage ./internal/app ./internal/observability` |
| Full suite command | `go test -timeout 60s ./...` |
| Estimated runtime | ~60 seconds |

## Sampling Rate

- After every task commit: run the package command listed for that task.
- After every plan wave: run `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/storage ./internal/app ./internal/observability`.
- Before `$gsd-verify-work`: full suite must be green.
- Max feedback latency: 60 seconds.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 20-01-01 | 01 | 1 | CFG-01 | T-20-01-01 | Nested config is canonical while legacy ENV/flat JSON remains compatible. | unit | `go test -timeout 60s ./internal/config -count=1` | yes | green |
| 20-01-02 | 01 | 1 | CFG-02 | T-20-01-01 | Component files override only scheduler/cache blocks. | unit | `go test -timeout 60s ./internal/config -count=1` | yes | green |
| 20-01-03 | 01 | 1 | CFG-03, CFG-04 | T-20-01-02 / T-20-01-03 | Examples stay secret-safe and optional systems stay disabled by default. | unit | `go test -timeout 60s ./internal/config -count=1` | yes | green |
| 20-02-01 | 02 | 2 | SCH-05 | T-20-02-03 | Executor concurrency is bounded by runner slots. | unit | `go test -timeout 60s ./internal/scheduler -run TestSynchronousRunnerHonorsExecutorConcurrency -count=1 -v` | yes | green |
| 20-02-02 | 02 | 2 | SCH-06 | T-20-02-01 | Redis `SET NX` lock prevents duplicate task execution and releases after delivery. | integration | `go test -timeout 60s ./internal/scheduler -run 'TestRedisTaskLockerUsesSetNXAndTTL|TestExecutorSkipsRedisLockedTaskWithoutDelivery' -count=1 -v` | yes | green |
| 20-02-03 | 02 | 2 | SCH-07 | T-20-02-02 | QueueGuard and Prometheus labels use bounded, sanitized admission evidence. | unit | `go test -timeout 60s ./internal/scheduler ./internal/observability -run 'TestQueueGuard|TestPrometheus.*Queue' -count=1 -v` | yes | green |
| 20-03-01 | 03 | 2 | QDR-05 | T-20-03-01 / T-20-03-02 | Embedding input is capped before provider calls without logging raw prompt text. | unit | `go test -timeout 60s ./internal/scheduler -run TestSemanticNeighborEmbedding -count=1 -v` | yes | green |
| 20-03-02 | 03 | 2 | QDR-06 | T-20-03-03 | Qdrant collection creation is explicit and uses configured dimension. | integration | `go test -timeout 60s ./internal/storage -run 'TestQdrantEnsureCollectionCreatesRealCollection|TestQdrantInsertReusesEnsureCollection' -count=1 -v` | yes | green |
| 20-03-03 | 03 | 2 | QDR-06 | T-20-03-03 | Semantic-neighbor startup fails open when collection ensure is unavailable. | unit | `go test -timeout 60s ./internal/app -count=1` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. No new test framework or fixtures are required.

## Manual-Only Verifications

All phase behaviors have automated verification. Mock-only or skipped tests are not counted as validation evidence.

## Real Component Evidence

| Component | Command | Result |
|-----------|---------|--------|
| Redis task lock | `go test -timeout 60s ./internal/scheduler -run 'TestRedisQueueRealZSetOperations|TestRedisTaskLockerUsesSetNXAndTTL|TestExecutorSkipsRedisLockedTaskWithoutDelivery' -count=1 -v` | passed; no skip |
| Qdrant collection | `go test -timeout 60s ./internal/storage -run 'TestQdrantEnsureCollectionCreatesRealCollection|TestQdrantInsertReusesEnsureCollection' -count=1 -v` | passed against real Qdrant 1.18.2; plaintext API-key warning logged |
| Redis VSS fallback | `go test -timeout 60s ./internal/storage -run TestRedisVSSVectorAdapter_Integration -count=1 -v` | passed; no skip |
| App/config/observability | `go test -timeout 60s ./internal/app ./internal/observability ./internal/config -count=1` | passed |

## Validation Sign-Off

- [x] All tasks have automated verify commands.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all MISSING references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-06
