# Benchmark Dashboard Closed-Loop Design

## Goal

Build one traceable data path from MMLU/LMSYS input through VeloxMesh Gateway and the configured model, into durable benchmark artifacts and Redis, through the BFF to the Admin Dashboard, and finally into CSV/HTML reports generated from the same run data.

## Canonical Contract

Every aggregate benchmark run uses the same `BenchmarkRun` object:

```json
{
  "runId": "20260716-mmlu-001",
  "method": "Our Gateway Method",
  "dataset": "mmlu_20",
  "requestCount": 20,
  "concurrency": 1,
  "requestRate": null,
  "warmUp": 0,
  "repeatedRuns": 1,
  "timeoutSettingSeconds": 120,
  "provider": "openai-compatible",
  "targetModel": "configured-model",
  "gatewayVersion": "VeloxMesh",
  "avgLatencyMs": 1000.0,
  "p50LatencyMs": 900.0,
  "p95LatencyMs": 1500.0,
  "p99LatencyMs": 1700.0,
  "ttftMs": null,
  "throughputRps": 0.8,
  "successRatePct": 95.0,
  "errorRatePct": 5.0,
  "timeoutRatePct": 0.0,
  "improvementPct": null,
  "testDate": "2026-07-16T00:00:00Z",
  "source": "gateway-runner",
  "rawFilePath": "reports/run-id",
  "exportId": "run-id",
  "status": "partial",
  "partialData": true
}
```

Numeric measurements remain numeric. Unavailable measurements are `null`; no layer may manufacture substitute values. `runId` and `exportId` connect raw artifacts, Redis, BFF responses, the Dashboard, and exports.

## Components And Flow

1. `run-gateway-dataset.py` sends every JSONL request to `/v1/chat/completions`, validates non-empty model output, records request-level results, and writes aggregate `BenchmarkRun` data.
2. A publisher reads one or more completed report directories, validates the canonical contract, and writes a snapshot to Redis key `veloxmesh:benchmarks`.
3. The Go BFF returns the complete contract from `/bff/admin/benchmarks`, including storage/source state. Fallback rows are only permitted when explicit demo mode is enabled.
4. The React Admin Dashboard renders numeric fields without inventing defaults. Empty, error, partial, permission, and demo states remain visible.
5. CSV and HTML exports use the exact `BenchmarkRun[]` currently displayed. Analysis is computed from valid data and reports insufficient data when comparison is impossible.

## Error Handling

- HTTP errors, timeouts, empty model content, invalid JSON, Redis failures, and incomplete runs remain explicit.
- A benchmark can be `passed`, `failed`, or `partial`; partial data is never presented as a successful comparison.
- Secrets are accepted from environment/config only and are excluded from all artifacts and exports.

## Verification

Use Python unit tests for runner aggregation and publication, Go tests for Redis/BFF DTO behavior and permissions, and Vitest for frontend mapping/export behavior. End-to-end acceptance requires the same `runId` in `summary.json`, Redis, BFF, Dashboard rows, CSV, and HTML report.
