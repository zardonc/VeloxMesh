---
phase: 19
slug: sla-waiting-time-promotion
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-05T16:01:16-07:00
---

# Phase 19 - Validation Strategy

> Reconstructed from PLAN, SUMMARY, UAT, SECURITY, REVIEW, VERIFICATION, and existing automated tests.

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing` |
| Config file | `go.mod` |
| Quick run command | `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/app ./internal/observability` |
| Full suite command | `go test -timeout 60s ./... && go build ./...` |
| Estimated runtime | ~30 seconds |

## Sampling Rate

- After every task commit: run the task's package-level Go test command.
- After every plan wave: run the phase full suite command.
- Before `$gsd-verify-work`: full suite must be green.
- Max feedback latency: 60 seconds per Go test command.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 19-01-01 | 01 | 1 | SLA-01 | T-19-01 | SLA promotion config is disabled by default with inert dormant rules. | unit | `go test -timeout 60s ./internal/config` | yes | green |
| 19-01-02 | 01 | 1 | SLA-01 | T-19-01 | Enabled rules require tenant/class selector, model class, request kind, wait threshold, and positive window. | unit | `go test -timeout 60s ./internal/config` | yes | green |
| 19-02-01 | 02 | 2 | SLA-02 | T-19-02 | Memory/Redis/fallback queues support bounded non-mutating `PeekMin`; `Push` replaces scores. | unit | `go test -timeout 60s ./internal/scheduler` | yes | green |
| 19-02-02 | 02 | 2 | SLA-03 | T-19-02 | Promotion matches safe task snapshots from trusted tenant/config fields, not prompt text. | unit | `go test -timeout 60s ./internal/scheduler` | yes | green |
| 19-02-03 | 02 | 2 | SLA-02, SLA-03 | T-19-02 | Eligible candidates are promoted before `PopMin`; priority and quota boundaries are not crossed. | unit | `go test -timeout 60s ./internal/scheduler ./internal/app` | yes | green |
| 19-03-01 | 03 | 3 | SLA-04 | T-19-03 | Prometheus promotion labels are bounded and exclude tenant_id, task_id, prompts, secrets, payloads, embeddings, and cache payloads. | unit | `go test -timeout 60s ./internal/observability` | yes | green |
| 19-03-02 | 03 | 3 | SLA-04 | T-19-03 | Durable audit/log evidence is sanitized and written only for promoted, blocked, or error outcomes. | unit | `go test -timeout 60s ./internal/scheduler ./internal/app` | yes | green |
| 19-03-03 | 03 | 3 | SLA-02, SLA-03, SLA-04 | T-19-03 | Each pre-pop attempt emits exactly one outcome and promotion errors fail open to normal `PopMin`. | unit | `go test -timeout 60s ./internal/scheduler ./internal/observability` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

## Manual-Only Verifications

All phase behaviors have automated verification.

## Validation Sign-Off

- [x] All tasks have automated verify commands.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-05
