# Request-Level Benchmark and Improved Model Design

## Goal

Complete the benchmark evidence chain from dataset request, through VeloxMesh Gateway and the selected model provider, into a request-level store, Dashboard export, and reproducible report package. The Dashboard never calls an upstream model directly.

## External Dependency

The improved model owner must provide a redacted OpenAI-compatible contract: base URL, model ID, model version, health behavior, concurrency limit, and timeout. Until that contract exists, the repository can validate the integration through a local OpenAI-compatible fixture, but must not label an unrelated public model as the improved model.

## Stable Method Identity

Every run uses one stable machine ID and one display label:

| Method ID | Display label |
| --- | --- |
| `local_baseline` | Local Baseline |
| `gateway` | Our Gateway Method |
| `improved_model` | Improved Model |
| `gateway_improved_model` | Our Gateway + Improved Model |

The method ID is stored in run metadata and every request row. Labels remain compatible with the existing Dashboard comparison.

## Canonical Request Record

Each attempted request records:

- `run_id`, `request_id`, `dataset`, `row_index`
- `method_id`, `method`, `provider`, `model`, `model_version`, `route`
- `started_at`, `ended_at`, `latency_ms`, `ttft_ms`
- `input_tokens`, `output_tokens`, `total_tokens`
- `status`, `http_status`, `error_type`, `timeout`
- `retry_count`, `cache_hit`

The record contains no prompt, response body, API key, Authorization header, or provider secret. Dataset rows remain referenced by dataset and row index.

## Storage

The publisher writes two Redis JSON documents:

- `veloxmesh:benchmarks`: aggregate run metadata for Dashboard tables
- `veloxmesh:benchmark_requests`: canonical request rows and generation metadata

The request snapshot is the evidence source. Aggregate metrics are recalculated from those rows rather than trusted from an independently edited summary.

## BFF Export

Authenticated Admin endpoints:

- `GET /bff/admin/benchmarks/raw.csv`
- `GET /bff/admin/benchmarks/export.zip`

The ZIP contains:

- `report.html`
- `metadata.json`
- `summary.csv`
- `raw_requests.csv`
- `errors_and_timeouts.csv`
- `charts/latency.svg`
- `charts/tail-latency.svg`
- `charts/throughput.svg`
- `charts/error-timeout-rate.svg`

`report.html` shows metadata, recomputed summaries, chart references, field definitions, bounded error samples, and raw file references. It never embeds all request rows.

## Recalculation Rules

- Request count is the number of raw rows for the run.
- Average, P50, P95, and P99 latency use all attempted request rows with nonnegative latency.
- TTFT uses rows with a measured TTFT.
- Success, error, and timeout rates share request count as denominator.
- Throughput is successful requests divided by the interval from the earliest start to latest end.
- Errors exclude timeout rows; timeout rate is reported separately.

## Security

Provider credentials remain runtime-only. Registration accepts the key as input, sends it only to the authenticated Gateway Admin API, and never writes it to output files. Export generation uses an allowlisted schema and scans generated text for secret-like fields.

## Acceptance

- A fake OpenAI-compatible improved model is registered and invoked through Gateway in automated tests.
- Four stable method IDs are validated.
- Raw CSV data row count equals attempted requests.
- `summary.csv` can be reproduced from `raw_requests.csv`.
- ZIP contents are complete and contain no credential.
- Customer sessions cannot access either export endpoint.

