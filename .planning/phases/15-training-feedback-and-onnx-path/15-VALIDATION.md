---
phase: 15
slug: training-feedback-and-onnx-path
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-04
---

# Phase 15 - Validation Strategy

> Retroactive Nyquist validation reconstructed from Phase 15 plans, summaries, verification, UAT, and rerun tests.

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing`; Python `pytest` via `uv` |
| **Config file** | `go.mod`; `tools/scheduler_training/pyproject.toml` |
| **Quick run command** | `go test -count=1 -timeout 60s ./internal/controlstate/... ./internal/scheduler/onnx ./internal/config ./cmd/scheduler` |
| **Full suite command** | `go test -count=1 -timeout 60s ./internal/controlstate/... ./internal/scheduler/onnx ./internal/config ./cmd/scheduler` and `uv run pytest` from `tools/scheduler_training` |
| **Estimated runtime** | ~20 seconds Go, ~16 seconds Python |

## Sampling Rate

- **After every task commit:** Run the quick Go command for runtime/control-state coverage.
- **After Python tooling changes:** Run `uv run pytest` from `tools/scheduler_training`.
- **After every plan wave:** Run both full commands.
- **Before `$gsd-verify-work`:** Go and Python suites must be green.
- **Max feedback latency:** 60 seconds per Go test package timeout.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 15-01-01 | 15-01 | 1 | FEED-01 | N/A | SQLite/PostgreSQL sample schemas store allowlisted feature/label columns only. | integration | `go test -count=1 -timeout 60s ./internal/controlstate/...` | yes | green |
| 15-01-02 | 15-01 | 1 | FEED-01 | N/A | Feedback config is opt-in and independent from scheduler enablement. | unit | `go test -count=1 -timeout 60s ./internal/config ./internal/app` | yes | green |
| 15-01-03 | 15-01 | 1 | FEED-01 | N/A | Scheduler runner records completed samples best-effort without changing data-plane responses. | unit/integration | `go test -count=1 -timeout 60s ./internal/scheduler ./internal/app` | yes | green |
| 15-02-01 | 15-02 | 1 | ML-01 | N/A | Export tooling rejects sensitive field names and writes only safe sample fields. | unit | `uv run pytest` | yes | green |
| 15-02-02 | 15-02 | 1 | ML-01 | N/A | Training/evaluation produces deterministic P70 output-token metrics. | unit | `uv run pytest` | yes | green |
| 15-02-03 | 15-02 | 1 | ML-01 | N/A | Publisher writes versioned `model.onnx` plus `manifest.json` only. | unit | `uv run pytest` | yes | green |
| 15-03-01 | 15-03 | 1 | ML-02 | N/A | ONNX artifact loading validates checksum and schema at startup. | unit | `go test -count=1 -timeout 60s ./internal/scheduler/onnx` | yes | green |
| 15-03-02 | 15-03 | 1 | ML-02 | N/A | ONNX scorer serves predicted latency, confidence, and scheduler version without per-request reload. | unit | `go test -count=1 -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic` | yes | green |
| 15-03-03 | 15-03 | 1 | ML-02 | N/A | Scheduler binary selects heuristic or ONNX mode without proto changes. | integration | `go test -count=1 -timeout 60s ./internal/config ./cmd/scheduler` | yes | green |

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

- `go test -count=1 -timeout 60s ./internal/controlstate/... ./internal/scheduler/onnx ./internal/config ./cmd/scheduler` - passed.
- `uv run pytest` from `tools/scheduler_training` - passed, 5 tests.

## Validation Sign-Off

- [x] All tasks have automated verification or existing Wave 0 coverage.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all missing references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s per package timeout.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-04

