# VeloxMesh Request-Level Benchmark Closed-Loop Runbook

## Data Flow

```text
MMLU / LMSYS dataset
  -> request_level_benchmark.py
  -> VeloxMesh Gateway /v1/chat/completions
  -> configured Provider / model
  -> canonical request rows + recomputed summary + ZIP report
  -> publish_request_level_results.py
  -> Redis keys veloxmesh:benchmarks and veloxmesh:benchmark_requests
  -> Go BFF Admin endpoints
  -> Admin Dashboard / Benchmarks
  -> raw CSV and complete ZIP report
```

The Dashboard never calls a model directly. The runner sends model requests only through VeloxMesh Gateway. Failed, timed out, and partial attempts remain visible and participate in the recomputed rates.

## Stable Methods

| Method ID | Display name |
|---|---|
| `local_baseline` | Local Baseline |
| `gateway` | Our Gateway Method |
| `improved_model` | Improved Model |
| `gateway_improved_model` | Our Gateway + Improved Model |

Never label a public or unchanged model as `improved_model`. Use that method only after the model owner supplies the real service contract and version.

## Register An Improved Model

Set secrets only in the current process environment, then run the Gateway registration verifier:

```powershell
cd dashboard
$env:VELOXMESH_ADMIN_API_KEY = "runtime-admin-key"
$env:VELOXMESH_DATA_API_KEY = "runtime-data-key"
$env:IMPROVED_MODEL_API_KEY = "runtime-provider-key"

powershell -NoProfile -ExecutionPolicy Bypass `
  -File scripts\benchmark\register-improved-model.ps1 `
  -GatewayUrl http://127.0.0.1:18080 `
  -ProviderId improved-model `
  -BaseUrl https://model-service.example/v1 `
  -ModelId improved-model-id `
  -ModelVersion v1 `
  -TimeoutSeconds 30
```

The command must verify Gateway health, Provider registration, `/v1/models`, and a minimal `/v1/chat/completions` request. Its output contains identifiers and verification state only, never credentials.

## Run A Small Closed-Loop Test

Run from the repository root in Windows PowerShell:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass `
  -File dashboard\scripts\test-scenarios\run-real-gateway-dashboard-flow.ps1 `
  -GatewayUrl http://127.0.0.1:18080 `
  -Model improved-model-id `
  -ModelVersion v1 `
  -Provider improved-model `
  -MethodId improved_model `
  -Concurrency 1 `
  -RequestRate 0.05 `
  -TimeoutSeconds 120
```

The script uses the two five-item datasets by default, writes one complete report directory per dataset, and publishes both aggregate and request-level records to Redis. It returns a non-zero exit code if any attempt is invalid or failed, while still publishing the failure evidence for Dashboard inspection.

For a four-method comparison, repeat the run with all four stable method IDs while keeping dataset, request count, concurrency, request rate, warm-up, repeats, timeout, and measurement environment equivalent.

## Export Contract

Every canonical request row contains:

`run_id`, `request_id`, `dataset`, `row_index`, `method_id`, `method`, `provider`, `model`, `model_version`, `route`, `started_at`, `ended_at`, `latency_ms`, `ttft_ms`, token counts, `status`, `http_status`, `error_type`, `timeout`, `retry_count`, and `cache_hit`.

The Admin BFF exposes:

- `GET /bff/admin/benchmarks/raw.csv`
- `GET /bff/admin/benchmarks/export.zip`

The ZIP includes `report.html`, `metadata.json`, `summary.csv`, `raw_requests.csv`, `errors_and_timeouts.csv`, and four SVG charts under `charts/`. The HTML Appendix contains field definitions, bounded error samples, and raw file references. Full request rows stay in CSV instead of being embedded in HTML.

All aggregates are recomputed from canonical requests. The CSV data-row count must equal the actual request-attempt count, and the Dashboard count, `summary.csv`, and HTML summary must agree.

## Verification Commands

```powershell
cd dashboard
python -m unittest -v scripts\benchmark\test_request_level_benchmark.py
python -m py_compile scripts\benchmark\request_level_benchmark.py scripts\benchmark\publish_request_level_results.py
go test ./...

cd web\admin-console
npm.cmd test
npm.cmd run build
npm.cmd run test:e2e
```

## Acceptance Gates

Do not run 20, 100, or full datasets until a stable Provider passes both five-item datasets with non-empty model content and at least 95% success. Do not start the full comparison until the real improved-model contract is registered and verified through Gateway.

Before a 20,000-row run, verify that all four methods use comparable settings, every row has a request ID, the export ZIP is complete, the summary can be recalculated from `raw_requests.csv`, and no artifact contains an API key.
# Configuration application verification

Provider and Routing mutations in production mode are not considered complete after an HTTP write alone. The BFF writes through the VeloxMesh Admin API, reads the saved revision back, then verifies the live data plane through `/v1/models` or a one-token `/v1/chat/completions` request.

Configure both server-side credentials before starting the BFF:

```text
VELOXMESH_ADMIN_API_KEY=replace_with_gateway_admin_key
VELOXMESH_DATA_API_KEY=replace_with_gateway_data_plane_key
```

The browser never receives either key. Mutation responses contain an `application` object whose `state` is one of:

- `verified`: revision readback and live request evidence both match.
- `applied`: the Gateway confirmed runtime activation; live verification has not yet completed.
- `warning`: the configuration is persisted, but runtime activation or live verification is incomplete. Read `message` before proceeding.
- `failed`: persistence, runtime activation, or revision readback failed. Do not treat this as success.

The evidence fields are `revision`, `providerId`, `route`, and `requestId`. Retry verification without another configuration write by calling `POST /bff/admin/runtime/verify` with `resource`, `target`, `revision`, and optional `model`.

Audit records use the same operation request ID and include actor, action, target, outcome, and revision. They must never include an Admin key, data-plane key, provider secret, authorization header, or raw prompt.
