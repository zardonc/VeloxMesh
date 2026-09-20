---
phase: 18
slug: anomaly-and-ood-conservative-scoring
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-05T16:01:16-07:00
---

# Phase 18 - Validation Strategy

> Reconstructed from PLAN, SUMMARY, UAT, VERIFICATION, and existing automated tests.

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`; Python `pytest` via `uv`; real Python ONNX Runtime worker smoke |
| Config file | `go.mod`; `tools/scheduler_training/pyproject.toml` |
| Quick run command | `go test -timeout 60s ./internal/scheduler/predictive ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler ./cmd/scheduler` |
| Full suite command | `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests/test_artifacts.py tools/scheduler_training/tests/test_onnx_worker.py tools/scheduler_training/tests/test_train_publish.py tools/scheduler_training/tests/test_export_schema.py -q && go test -timeout 60s ./... && go build ./...` |
| Estimated runtime | ~35 seconds |

## Sampling Rate

- After every task commit: run the task's package-level Go or Python command.
- After every plan wave: run the phase full suite command.
- Before `$gsd-verify-work`: full suite must be green, including the real ONNX worker smoke.
- Max feedback latency: 60 seconds per Go test command.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 18-01-01 | 01 | 1 | ANOM-01 | T-18-01 | Thresholds computed from safe successful samples; failure/timeout rows remain evidence. | unit | `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests/test_train_publish.py tools/scheduler_training/tests/test_export_schema.py` | yes | green |
| 18-01-02 | 01 | 1 | ANOM-01 | T-18-01 | Runtime manifests publish anomaly thresholds and evidence without raw datasets. | unit | `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests/test_train_publish.py` | yes | green |
| 18-01-03 | 01 | 1 | ANOM-01 | T-18-01 | Go artifact loader parses nested anomaly metadata. | unit | `go test -timeout 60s ./internal/scheduler/onnx` | yes | green |
| 18-02-01 | 02 | 2 | ANOM-02 | T-18-02 | Missing metadata is unavailable; invalid metadata degrades anomaly behavior only. | unit | `go test -timeout 60s ./internal/scheduler/onnx ./cmd/scheduler` | yes | green |
| 18-02-02 | 02 | 2 | ANOM-03 | T-18-02 | OOD signals lower confidence and raise uncertainty before scoring. | unit | `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic` | yes | green |
| 18-02-03 | 02 | 2 | ANOM-04 | T-18-02 | Anomaly status is emitted through low-cardinality metrics/status only. | unit | `go test -timeout 60s ./internal/scheduler/onnx ./internal/observability` | yes | green |
| 18-03-01 | 03 | 3 | ANOM-04 | T-18-03 | Durable rollups include anomaly and coverage fields with SQLite/PostgreSQL parity. | unit/integration | `go test -timeout 60s ./internal/controlstate/...` | yes | green |
| 18-03-02 | 03 | 3 | ANOM-04 | T-18-03 | Scheduler quality recording separates anomaly unavailable from errors/fallback. | unit | `go test -timeout 60s ./internal/scheduler` | yes | green |
| 18-03-03 | 03 | 3 | ANOM-04 | T-18-03 | Live comparison metrics use bounded coverage/anomaly labels. | unit | `go test -timeout 60s ./internal/observability ./internal/scheduler` | yes | green |
| 18-04-01 | 04 | 4 | ANOM-01, ANOM-02 | T-18-04-01 | Predictor contract validates protocol, quantiles, schema, training hash, and model version before prediction. | unit | `go test -timeout 60s ./internal/scheduler/predictor && uv run --project tools/scheduler_training pytest tools/scheduler_training/tests/test_train_publish.py` | yes | green |
| 18-04-02 | 04 | 4 | ANOM-02, ANOM-03 | T-18-04-02/T-18-04-03 | Real Python worker loads `onnxruntime.InferenceSession`; Go client uses health, timeout, breaker, and fallback. | smoke | `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests/test_onnx_worker.py && go test -timeout 60s ./internal/scheduler/predictor ./cmd/scheduler` | yes | green |
| 18-04-03 | 04 | 4 | ANOM-03, ANOM-04 | T-18-04-04/T-18-04-05 | Predictive scorer converts quantiles/signals to conservative scores without changing Scheduler RPC. | unit/smoke | `go test -timeout 60s ./internal/scheduler/predictive ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler ./cmd/scheduler` | yes | green |

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
