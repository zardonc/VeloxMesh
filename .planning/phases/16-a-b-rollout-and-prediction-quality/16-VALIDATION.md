---
phase: 16
slug: a-b-rollout-and-prediction-quality
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-04
---

# Phase 16 - Validation Strategy

> Retroactive Nyquist validation reconstructed from Phase 16 plans, summaries, verification, and rerun tests.

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` via `go test` |
| **Config file** | `go.mod` |
| **Quick run command** | `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/http/handlers` |
| **Full suite command** | `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/observability ./internal/controlstate/... ./internal/http/handlers ./internal/app` |
| **Estimated runtime** | ~23 seconds |

## Sampling Rate

- **After every task commit:** Run the quick command for rollout/config/admin coverage.
- **After every plan wave:** Run the full Phase 16 command.
- **Before `$gsd-verify-work`:** Full suite must be green.
- **Max feedback latency:** 60 seconds per Go test package timeout.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 16-01-01 | 16-01 | 1 | ML-03 | N/A | Rollout config validates percent bounds and ONNX endpoint requirements. | unit | `go test -count=1 -timeout 60s ./internal/config` | yes | green |
| 16-01-02 | 16-01 | 1 | ML-03 | N/A | Weighted scorer assigns tasks deterministically and falls ONNX failures back to heuristic/FIFO. | unit/integration | `go test -count=1 -timeout 60s ./internal/scheduler` | yes | green |
| 16-01-03 | 16-01 | 1 | ML-03 | N/A | App startup supports heuristic-only and weighted rollout without data-plane contract changes. | integration | `go test -count=1 -timeout 60s ./internal/app` | yes | green |
| 16-02-01 | 16-02 | 1 | OBS-02 | N/A | Live quality metrics use scheduler type, scheduler version, and task type only. | unit | `go test -count=1 -timeout 60s ./internal/observability` | yes | green |
| 16-02-02 | 16-02 | 1 | OBS-02 | N/A | SQLite/PostgreSQL quality rollups store aggregate evidence and safe sample IDs. | integration | `go test -count=1 -timeout 60s ./internal/controlstate/...` | yes | green |
| 16-02-03 | 16-02 | 1 | OBS-02 | N/A | Prediction quality recorder writes MAPE, wait, call latency, and error evidence best-effort. | unit/integration | `go test -count=1 -timeout 60s ./internal/scheduler ./internal/app` | yes | green |
| 16-03-01 | 16-03 | 1 | ML-03 | N/A | Runtime controller updates rollout percent without rebuilding the scorer. | unit | `go test -count=1 -timeout 60s ./internal/scheduler -run TestRolloutController` | yes | green |
| 16-03-02 | 16-03 | 1 | OBS-02, ML-03 | N/A | Admin rollout status/update routes require admin auth and validate exact JSON bodies. | integration | `go test -count=1 -timeout 60s ./internal/http/handlers` | yes | green |
| 16-03-03 | 16-03 | 1 | OBS-02, ML-03 | N/A | MAPE and error-spike alerts notify operators without automatic rollout changes. | unit | `go test -count=1 -timeout 60s ./internal/scheduler ./internal/observability` | yes | green |

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

- Initial parallel package rerun surfaced transient failures in `internal/scheduler` and `internal/controlstate/postgres`; each failed test passed in isolation.
- `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/observability ./internal/controlstate/... ./internal/http/handlers ./internal/app` - passed on serial rerun.

## Validation Sign-Off

- [x] All tasks have automated verification or existing Wave 0 coverage.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s per package timeout.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-04

