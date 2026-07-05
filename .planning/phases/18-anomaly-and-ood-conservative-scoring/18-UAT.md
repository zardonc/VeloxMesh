---
status: complete
phase: 18-anomaly-and-ood-conservative-scoring
source:
  - 18-01-SUMMARY.md
  - 18-02-SUMMARY.md
  - 18-03-SUMMARY.md
started: 2026-07-05T09:42:00-07:00
updated: 2026-07-05T09:48:00-07:00
---

## Current Test

[testing complete]

## Tests

### 1. Offline anomaly artifact metadata
expected: Training tooling computes anomaly thresholds from successful safe samples only, keeps failure/timeout rows as evidence, publishes nested anomaly_thresholds in manifest.json, and rejects forbidden exported fields.
result: pass
evidence:
  - `uv run pytest -v tests/test_train_publish.py tests/test_export_schema.py` from `tools/scheduler_training`

### 2. ONNX anomaly runtime scoring
expected: ONNX artifact loading preserves anomaly metadata, missing metadata remains loadable, invalid metadata degrades anomaly behavior only, OOD tasks lower confidence and raise score conservatism, and missing anomaly metadata leaves scoring unchanged.
result: pass
evidence:
  - `go test -v -timeout 60s -count=1 ./internal/scheduler/onnx ./internal/scheduler ./internal/observability ./internal/controlstate -run 'TestLoadArtifact.*Anomaly|TestAnomaly|TestMissingAnomaly|TestPredictionQualityRecorderRecordsUnavailableAnomalySeparately|TestPrometheusSchedulerAnomalyStatusLabelsAreBounded|TestSecretCipherRejectsInvalidNonce'`

### 3. Durable quality and metrics evidence
expected: Scheduler quality rollups record anomaly unavailable separately from errors/fallback, bounded anomaly metric labels are enforced, and SQLite/PostgreSQL/control-state paths pass without cached results.
result: pass
evidence:
  - `go test -timeout 60s -count=1 ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/observability ./internal/controlstate/... ./internal/scheduler ./cmd/scheduler ./tests/integration`

### 4. Real test-environment component smoke
expected: Tests load `.env`/`.env.local`, use real Postgres, real Redis, and the configured non-Gemini provider path; Plan4 smoke returns HTTP 200, real provider smoke returns HTTP 200, and Redis pub/sub/cache/limiter/session blacklist tests pass without skips.
result: pass
evidence:
  - `go test -v -timeout 60s -count=1 ./tests/integration -run 'TestRedisHotState|TestPlan4Postgres'`

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Debug Notes

- Initial real Plan4/Postgres smoke exposed `crypto/cipher: incorrect nonce length given to GCM`.
- Root cause was shared real Postgres schema state plus `AESGCMSecretCipher.DecryptProviderSecret` allowing invalid nonce length to reach `cipher.GCM.Open`.
- Fixed by isolating the Plan4 smoke schema and returning an explicit invalid nonce error before decryption.
- Re-ran the real Redis/Postgres/provider smoke; it passed without skips.

## Gaps

None.

