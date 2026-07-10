---
phase: 22
slug: documentation-env-example-uat
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-06
audited: 2026-07-06T16:25:00-07:00
---

# Phase 22 - Validation Strategy

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing`, ripgrep doc checks, Python `pytest` for ONNX worker evidence |
| Config file | `go.mod`, `tools/scheduler_training/pyproject.toml` |
| Quick run command | `go test -timeout 60s ./internal/config ./internal/http/handlers ./internal/scheduler` |
| Full suite command | `go test -timeout 60s ./...` |
| Estimated runtime | ~60 seconds |

## Sampling Rate

- After every task commit: run the package or doc command listed for that task.
- After every plan wave: run `go test -timeout 60s ./internal/config ./internal/http/handlers ./internal/scheduler`.
- Before `$gsd-verify-work`: full suite must be green.
- Max feedback latency: 60 seconds.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 22-01-01 | 01 | 1 | SCH-08, OBS-03, OBS-04, OBS-05, OBS-06, QDR-06, QDR-08 | T-22-01-04 | Runbook documents admin APIs, config, degradation, vector ownership, and 60s test commands. | doc check | `rg -n "Scheduler 1.0|SCHEDULER_ENABLED|/admin/v1/scheduler/status|Qdrant|pgvector|go test -timeout 60s" README.md docs/scheduler-1.0-runbook.md` | yes | green |
| 22-01-02 | 01 | 1 | CFG-03, CFG-04 | T-22-01-01 / T-22-01-02 | Examples parse, stay disabled by default, and reject secret-shaped placeholders. | unit | `go test -timeout 60s ./internal/config -count=1` | yes | green |
| 22-01-03 | 01 | 1 | QDR-06, QDR-08 | T-22-01-04 | Qdrant/pgvector guidance keeps vector lookup gateway-owned and scheduler inputs aggregate-only. | doc check | `rg -n "Qdrant|pgvector|vector_store|scheduler receives only aggregate" docs/scheduler-1.0-runbook.md config.cache.example.json README.md` | yes | green |
| 22-01-04 | 01 | 1 | CFG-03, CFG-04, SCH-08, QDR-06, QDR-08, OBS-03, OBS-04, OBS-05, OBS-06 | T-22-01-03 | UAT records command, expected, actual, notes, and classification for release evidence. | integration/UAT | `go test -timeout 60s ./internal/config ./internal/http/handlers ./internal/scheduler -count=1` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. No new test framework or fixtures are required.

## Manual-Only Verifications

All phase behaviors have automated verification in this environment. Skipped commands are listed below and not counted as validation evidence.

## Real Component Evidence

| Component | Command | Result |
|-----------|---------|--------|
| Config/examples | `go test -timeout 60s ./internal/config -count=1` | passed |
| Admin APIs | `go test -timeout 60s ./internal/http/handlers -run 'TestAdminSchedulerStatus|TestAdminSchedulerSLARules|TestAdminSchedulerTrainingExport' -count=1 -v` | passed |
| Scheduler degradation and attribution | `go test -timeout 60s ./internal/scheduler -count=1` | passed |
| Qdrant | `go test -timeout 60s ./internal/storage -run 'TestQdrantEnsureCollectionCreatesRealCollection|TestQdrantInsertReusesEnsureCollection' -count=1 -v` | passed against real Qdrant; plaintext API-key warning logged |
| Redis | `go test -timeout 60s ./internal/storage -run TestRedisVSSVectorAdapter_Integration -count=1 -v` | passed; no skip |
| Postgres + real provider/model | `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1 -v` | passed with HTTP 200 through `/v1/chat/completions`; plaintext Postgres DSN warning logged |
| ONNX Runtime | `UV_CACHE_DIR=../../.tmp/uv-cache timeout 60s uv run --group dev pytest tests/test_train_publish.py tests/test_onnx_worker.py --basetemp ../../.tmp/pytest-tmp -p no:cacheprovider` | passed, 8 Python ONNX Runtime/worker tests |

## Non-Counted Checks

| Command | Result | Reason |
|---------|--------|--------|
| `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1 -v` | skipped | `PLAN4_*` env vars were not present in this shell. This skipped fake-provider smoke is not counted; real provider/Postgres evidence above is counted. |

## Validation Sign-Off

- [x] All tasks have automated verify or real UAT evidence.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all MISSING references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-06
