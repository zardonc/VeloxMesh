# VeloxMesh Benchmark Closed-Loop Runbook

## Data Flow

```text
MMLU / LMSYS JSONL
  -> VeloxMesh Gateway /v1/chat/completions
  -> configured Provider / model
  -> summary.json + latency.csv + responses.jsonl + failures
  -> Redis key veloxmesh:benchmarks
  -> GET /bff/admin/benchmarks
  -> Admin Dashboard / Benchmarks
  -> Export CSV / Export Report
```

All aggregate layers use the same `BenchmarkRun` contract and `runId`. Missing measurements remain `null`; failed or partial runs stay visible and are never converted into successful comparison data.

## Run A Small Closed-Loop Test

Run from the capstone workspace in Windows PowerShell:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File dashboard\scripts\test-scenarios\run-real-gateway-dashboard-flow.ps1 `
  -GatewayUrl http://127.0.0.1:18080 `
  -Model openrouter/nvidia/nemotron-3-super-120b-a12b:free `
  -Provider openai-compatible `
  -Concurrency 1 `
  -RequestRate 0.05 `
  -TimeoutSeconds 120
```

The script runs the two five-item datasets by default, writes report directories, publishes the canonical summaries to Redis, and returns a non-zero exit code when any request is invalid or failed. Publication still occurs so the Dashboard can show the real failure state.

## Verified Run: 2026-07-16

Report root:

`VeloxMesh/reports/dashboard-closed-loop-20260716`

| Dataset | Run ID | Requests | Success | Status | Error |
|---|---|---:|---:|---|---|
| MMLU | `20260716T073418-mmlu_5` | 5 | 0% | failed | `no_healthy_provider` (5) |
| LMSYS | `20260716T073418-lmsys_5` | 5 | 0% | failed | `no_healthy_provider` (5) |

Redis publication succeeded with two rows. The BFF returned:

- `source`: `redis`
- Redis status: `connected`
- Run IDs: `20260716T073418-mmlu_5`, `20260716T073418-lmsys_5`
- Statuses: `failed`, `failed`

The Admin Dashboard at `http://127.0.0.1:5173/` displayed both rows as `BFF / Redis live data`, including the same run IDs, model, raw paths, status, partial-data flag, and rates.

## Verification Commands

```powershell
cd VeloxMesh\scripts
python -m unittest -v test_run_gateway_dataset.py test_publish_benchmark_results.py

cd ..\..\dashboard
$env:GOCACHE="$PWD\.gocache"
$env:GOTMPDIR="$PWD\.gotmp"
go test ./...

cd web\admin-console
npm.cmd test
npm.cmd run build
```

## Acceptance Gates

Do not run 20, 100, or full datasets until a stable Provider passes both five-item datasets with non-empty model content and at least 95% success. A full comparison additionally requires at least two complete passed methods using the same dataset and comparable settings.

The current closed-loop integration is working, but the 2026-07-16 model benchmark is not passed because the Gateway reported no healthy upstream Provider.
