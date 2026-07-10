---
status: complete
phase: 18-anomaly-and-ood-conservative-scoring
requirements: [ANOM-01, ANOM-02, ANOM-03, ANOM-04]
verified: 2026-07-05
human_verification: []
gaps: []
---

# Phase 18 Verification

## Result

Complete under the clarified production-identical model acceptance. Phase 18 now publishes a feature-driven ONNX graph that consumes scheduler feature tensors and emits quantile/signal outputs through the same Python worker and Scheduler call chain used in production.

## Requirement Coverage

- **ANOM-01:** Offline tooling computes anomaly/OOD threshold metadata and now publishes predictor-v1 manifests with protocol version, quantiles, feature schema, training data hash, model version, and compatibility fields.
- **ANOM-02:** Scheduler startup validates predictor manifests before prediction and degrades to NoopPredictor/heuristic scoring when manifest or worker health checks fail.
- **ANOM-03:** `PredictiveScorer` applies predictor quantiles and OOD/spread signals to lower confidence, increase uncertainty, and produce more conservative virtual-deadline scores for unfamiliar tasks.
- **ANOM-04:** Existing quality rollups and metrics continue to track anomaly status separately from scheduler fallback, and the Scheduler smoke test returns non-fallback predictive scores through the unchanged Scheduler RPC.

## Corrective Acceptance

- Predictor contract returns `map[int]float64` quantiles and model-native signals, not P70-specific Scheduler decisions.
- Python worker loads one `onnxruntime.InferenceSession` and serves gRPC health plus batch prediction.
- Go predictor client validates worker health, uses call timeouts, trips a breaker on failures, and degrades to NoopPredictor without failing Scheduler startup.
- Scheduler mode `onnx` now routes through `PredictiveScorer`; the legacy Go constant ONNX parser is no longer accepted as production runtime evidence.
- Service mapping preserves all safe scalar and semantic aggregate `TaskFeature` fields.
- End-to-end smoke starts the real Python ONNX worker, connects Scheduler to it, calls `BatchScoreTasks`, and receives a non-fallback predictive score.
- The smoke uses the production-shape ONNX model artifact emitted by `publish_artifact`; only the amount of training data differs.

## Automated Checks

- `uv run pytest -v tests/test_artifacts.py tests/test_onnx_worker.py tests/test_train_publish.py tests/test_export_schema.py` - passed, 14 tests
- `go test -timeout 60s -count=1 ./internal/scheduler/predictive ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler ./cmd/scheduler` - passed
- `go test -timeout 60s -count=1 ./...` - passed
- `go build ./...` - passed

## Security Checks

- Scheduler receives only safe scalar/enum task features; raw prompts, embeddings, semantic-cache payloads, authorization headers, API keys, provider secrets, tenant IDs, and provider payloads remain outside predictor RPC.
- Predictor worker transport is local and configurable; default tests bind only to loopback.
- Metrics/status paths use low-cardinality status values and do not expose raw threshold details.

## Code Review

- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-REVIEW.md` - clean

## Gaps

None.
