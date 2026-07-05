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

Passed. Phase 18 delivers anomaly/OOD conservative scoring without changing the Scheduler RPC contract.

## Requirement Coverage

- **ANOM-01:** Offline tooling computes grouped anomaly thresholds from successful safe samples and publishes `anomaly_thresholds` / `anomaly_evidence` in `manifest.json`.
- **ANOM-02:** ONNX runtime validates anomaly metadata as optional state, lowering confidence and increasing uncertainty for OOD tasks.
- **ANOM-03:** Missing or invalid anomaly metadata degrades anomaly behavior only; core ONNX startup/scoring remains available and status/reason are low-cardinality.
- **ANOM-04:** Quality rollups and live metrics include coverage/anomaly evidence without conflating unavailable/degraded anomaly metadata with scheduler fallback errors.

## Automated Checks

- `uv run pytest tests/test_train_publish.py tests/test_export_schema.py` - passed
- `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/observability ./internal/controlstate/... ./internal/scheduler ./cmd/scheduler` - passed
- `go test -timeout 60s ./...` - passed
- `go build ./...` - passed

## Security Checks

- No raw prompts, embeddings, semantic-cache payloads, authorization headers, API keys, provider secrets, tenant IDs, task IDs, thresholds, or validation error text are used as metric labels.
- Artifact output remains limited to `model.onnx` and `manifest.json`.
- Anomaly metadata validation errors are retained locally and only status/reason enums are surfaced.

## Gaps

None.
