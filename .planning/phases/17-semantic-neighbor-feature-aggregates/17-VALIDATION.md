---
phase: 17
slug: semantic-neighbor-feature-aggregates
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-05T16:01:16-07:00
---

# Phase 17 - Validation Strategy

> Reconstructed from PLAN, SUMMARY, VERIFICATION, and existing automated tests.

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`; Python `pytest` via `uv` |
| Config file | `go.mod`; `tools/scheduler_training/pyproject.toml` |
| Quick run command | `go test -timeout 60s ./internal/scheduler ./internal/config ./internal/app ./internal/observability` |
| Full suite command | `go test -timeout 60s ./internal/scheduler ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/controlstate/sqlite ./internal/controlstate/postgres && uv run --project tools/scheduler_training pytest tools/scheduler_training/tests -q && go build ./...` |
| Estimated runtime | ~20 seconds |

## Sampling Rate

- After every task commit: run the task's package-level `go test` or `uv run --project tools/scheduler_training pytest ...` command.
- After every plan wave: run the phase full suite command.
- Before `$gsd-verify-work`: full suite must be green.
- Max feedback latency: 60 seconds per Go test command.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 17-01-01 | 01 | 1 | QDR-02, QDR-04 | T-17-01-01 | First-class bounded TaskFeature/proto fields; no raw text or embeddings. | unit | `go test -timeout 60s ./internal/scheduler` | yes | green |
| 17-01-02 | 01 | 1 | QDR-01, QDR-03 | T-17-01-02/T-17-01-03 | Semantic-neighbor config disabled by default and timeout-bounded. | unit | `go test -timeout 60s ./internal/config` | yes | green |
| 17-02-01 | 02 | 2 | QDR-01, QDR-02 | T-17-02-01/T-17-02-02 | Gateway-side aggregation uses safe sample IDs and bounded aggregate fields only. | unit | `go test -timeout 60s ./internal/scheduler` | yes | green |
| 17-02-02 | 02 | 2 | QDR-01, QDR-03 | T-17-02-03 | Intake enriches after safe extraction and fails open on timeout/error. | unit | `go test -timeout 60s ./internal/scheduler ./internal/app` | yes | green |
| 17-02-03 | 02 | 2 | QDR-02, QDR-03 | T-17-02-04 | Completed samples index only after durable record; metrics use closed labels. | unit | `go test -timeout 60s ./internal/scheduler ./internal/observability` | yes | green |
| 17-03-01 | 03 | 3 | QDR-02, QDR-04 | T-17-03-01 | SQLite/PostgreSQL training samples persist bounded aggregate columns with defaults. | unit/integration | `go test -timeout 60s ./internal/scheduler ./internal/controlstate/sqlite ./internal/controlstate/postgres` | yes | green |
| 17-03-02 | 03 | 3 | QDR-02, QDR-04 | T-17-03-01 | Offline export/training fills semantic aggregate defaults and rejects forbidden fields. | unit | `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests -q` | yes | green |
| 17-03-03 | 03 | 3 | QDR-04 | T-17-03-02/T-17-03-03/T-17-03-04 | ONNX semantic support is manifest-gated; heuristic scoring stays invariant. | unit | `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic` | yes | green |

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
