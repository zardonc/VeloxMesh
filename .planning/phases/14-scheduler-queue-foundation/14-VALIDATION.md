---
phase: 14
slug: scheduler-queue-foundation
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-04
---

# Phase 14 - Validation Strategy

> Retroactive Nyquist validation reconstructed from Phase 14 plans, summaries, verification, UAT, and rerun tests.

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` via `go test` |
| **Config file** | `go.mod` |
| **Quick run command** | `go test -count=1 -timeout 60s ./internal/scheduler ./internal/scheduler/heuristic ./cmd/scheduler` |
| **Full suite command** | `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/scheduler/heuristic ./cmd/scheduler ./internal/admission ./internal/gateway ./internal/http/handlers ./internal/app ./internal/observability` |
| **Estimated runtime** | ~23 seconds |

## Sampling Rate

- **After every task commit:** Run the quick command for scheduler/scorer coverage.
- **After every plan wave:** Run the full Phase 14 command.
- **Before `$gsd-verify-work`:** Full suite must be green.
- **Max feedback latency:** 60 seconds per Go test package timeout.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 14-01-01 | 14-01 | 1 | SCH-01, SCH-02 | N/A | Scheduler proto carries bounded fields only; disabled scheduler returns FIFO without dialing. | unit/integration | `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler` | yes | green |
| 14-01-02 | 14-01 | 1 | SCH-01, SCH-02 | N/A | gRPC scorer uses real TCP calls, timeout fallback, breaker fallback, and missing-score FIFO fallback. | integration | `go test -count=1 -timeout 60s ./internal/scheduler -run TestGRPCScorer` | yes | green |
| 14-02-01 | 14-02 | 1 | SCH-03 | N/A | Queue metadata excludes raw prompts, auth material, secrets, and payload bodies. | unit | `go test -count=1 -timeout 60s ./internal/scheduler -run TestResultRegistry` | yes | green |
| 14-02-02 | 14-02 | 1 | SCH-03 | N/A | Memory queue preserves min-score ordering and FIFO ties. | unit | `go test -count=1 -timeout 60s ./internal/scheduler -run TestMemoryQueue` | yes | green |
| 14-02-03 | 14-02 | 1 | SCH-03 | N/A | Redis ZSET queue stores task IDs only and falls back to memory after primary failure. | integration | `go test -count=1 -timeout 60s ./internal/scheduler -run TestRedisQueue` | yes | green |
| 14-03-01 | 14-03 | 1 | SCORE-02 | N/A | Feature extraction and request-kind classification stay bounded and local. | unit | `go test -count=1 -timeout 60s ./internal/scheduler` | yes | green |
| 14-03-02 | 14-03 | 1 | SCORE-01, SCORE-02 | N/A | Heuristic scoring uses static virtual deadline, priority multiplier, and uncertainty penalty. | unit | `go test -count=1 -timeout 60s ./internal/scheduler/heuristic` | yes | green |
| 14-03-03 | 14-03 | 1 | SCH-04, OBS-01 | N/A | Scheduler service exposes real gRPC scoring, HTTP health, and Prometheus metrics. | integration | `go test -count=1 -timeout 60s ./internal/scheduler/heuristic ./cmd/scheduler` | yes | green |
| 14-04-01 | 14-04 | 1 | PRIO-01, PRIO-02 | N/A | Priority comes from trusted structured inputs and over-policy claims are downgraded. | unit | `go test -count=1 -timeout 60s ./internal/scheduler ./internal/admission` | yes | green |
| 14-04-02 | 14-04 | 1 | SCH-01, SCH-02, SCH-03 | N/A | Gateway chat and stream paths keep the OpenAI-compatible contract through the synchronous runner. | integration | `go test -count=1 -timeout 60s ./internal/gateway ./internal/http/handlers ./internal/app` | yes | green |
| 14-04-03 | 14-04 | 1 | OBS-01 | N/A | Queue, scheduler, breaker, and priority metrics use allowlisted low-cardinality labels. | unit | `go test -count=1 -timeout 60s ./internal/observability` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

## Manual-Only Verifications

All phase behaviors have automated verification.

## Validation Audit 2026-07-04

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

## Rerun Evidence

- `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/scheduler/heuristic ./cmd/scheduler ./internal/admission ./internal/gateway ./internal/http/handlers ./internal/app ./internal/observability` - passed.

## Validation Sign-Off

- [x] All tasks have automated verification or existing Wave 0 coverage.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s per package timeout.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-04

