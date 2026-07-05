---
status: passed
phase: 18-anomaly-and-ood-conservative-scoring
requirements: [ANOM-01, ANOM-02, ANOM-03, ANOM-04]
verified: 2026-07-05
human_verification: []
gaps: []
---

# Phase 18 Verification

## Result

Passed. Phase 18 now delivers anomaly/OOD conservative scoring through a model-neutral predictor boundary and proves a real ONNX Runtime invocation without changing the Scheduler `BatchScoreTasks` RPC contract.

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

## Automated Checks

- `uv run pytest tests/test_onnx_worker.py tests/test_train_publish.py` - passed, 8 tests
- `go test -timeout 60s ./internal/scheduler/predictive ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler ./cmd/scheduler` - passed
- `go test -timeout 60s ./...` - passed
- `go build ./...` - passed

## Security Checks

- Scheduler receives only safe scalar/enum task features; raw prompts, embeddings, semantic-cache payloads, authorization headers, API keys, provider secrets, tenant IDs, and provider payloads remain outside predictor RPC.
- Predictor worker transport is local and configurable; default tests bind only to loopback.
- Metrics/status paths use low-cardinality status values and do not expose raw threshold details.

## Code Review

- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-REVIEW.md` - clean

## Gaps

None.
