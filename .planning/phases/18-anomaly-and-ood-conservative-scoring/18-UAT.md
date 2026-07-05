---
status: complete
phase: 18-anomaly-and-ood-conservative-scoring
source:
  - 18-01-SUMMARY.md
  - 18-02-SUMMARY.md
  - 18-03-SUMMARY.md
  - 18-04-SUMMARY.md
started: 2026-07-05T09:42:00-07:00
updated: 2026-07-05T12:21:27-07:00
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

### 5. Real ONNX worker and Scheduler smoke
expected: Verification starts a real Python worker, loads a published runtime artifact through `onnxruntime.InferenceSession`, serves predictor gRPC, connects Scheduler ONNX mode to that worker, calls `BatchScoreTasks`, and receives a non-fallback predictive score. Tests that only mock the worker or parse a constant ONNX graph in Go do not satisfy this check.
result: issue
reported: "Acceptance requires tests to use the same model artifact shape and call chain that production will ship; only training data volume may differ. Current verification uses a `write_constant_onnx` Constant-node artifact, so the worker and Scheduler call path is real but the model artifact is not final production shape."
severity: blocker
evidence:
  - Visible verification artifact generated at `C:\Users\inthe\IdeaProjects\VeloxMesh\.tmp\phase18-real-onnx-verification\artifacts\scheduler-predictor-v1\model.onnx`; size `630` bytes; SHA-256 `5e3b7c7d76386ce7694d475801f6b6b819142c07b630753b492499d14ceaae6a`; manifest `model_sha256` matched.
  - Direct `onnxruntime.InferenceSession(...\model.onnx, providers=["CPUExecutionProvider"])` call returned outputs `p50=16`, `p70=20`, `p90=24`, `quantile_spread=8`, `ood_distance=0`.
  - Real worker gRPC call over `scheduler_training.onnx_worker.start_server` returned health ready and quantiles `{50:16, 70:20, 90:24}` with signals `quantile_spread=8`, `feature_coverage=1`, `ood_distance=0`.
  - `uv run pytest tests/test_onnx_worker.py tests/test_train_publish.py` from `tools/scheduler_training` - 8 passed
  - `go test -timeout 60s -count=1 ./cmd/scheduler ./internal/scheduler/predictor ./internal/scheduler/predictive` - passed
  - `go test -timeout 60s -count=1 -v ./cmd/scheduler -run TestSchedulerServiceUsesPythonONNXWorkerSmoke` - passed; ran `TestSchedulerServiceUsesPythonONNXWorkerSmoke`, started the Python worker process, and returned a non-fallback predictive score

## Summary

total: 5
passed: 4
issues: 1
pending: 0
skipped: 0
blocked: 0

## Debug Notes

- Initial real Plan4/Postgres smoke exposed `crypto/cipher: incorrect nonce length given to GCM`.
- Root cause was shared real Postgres schema state plus `AESGCMSecretCipher.DecryptProviderSecret` allowing invalid nonce length to reach `cipher.GCM.Open`.
- Fixed by isolating the Plan4 smoke schema and returning an explicit invalid nonce error before decryption.
- Re-ran the real Redis/Postgres/provider smoke; it passed without skips.

## Gaps

- truth: "Tests must use the same model artifact shape and call chain that production will ship, with only training data volume differing."
  status: failed
  reason: "Current publish path writes a Constant-node ONNX artifact via `tools/scheduler_training/scheduler_training/artifacts.py`; tests call ONNX Runtime and Scheduler through the real worker path, but the artifact is not a production-shape quantile model over scheduler features."
  severity: blocker
  test: 5
  artifacts:
    - tools/scheduler_training/scheduler_training/artifacts.py
    - tools/scheduler_training/scheduler_training/publish.py
    - tools/scheduler_training/tests/test_onnx_worker.py
    - cmd/scheduler/main_test.go
  missing:
    - "Production-shape ONNX export that consumes scheduler feature tensors and emits quantile/signal outputs through the same worker and Scheduler path."
