# Dashboard Production Data Design

## Goal

Close the benchmark demonstration loop: dataset -> VeloxMesh Gateway -> upstream model -> report files -> Redis -> authenticated BFF -> Admin/Customer Dashboard -> CSV/HTML report.

## Data contracts

- `veloxmesh:benchmarks`: aggregate `BenchmarkRun` snapshot.
- `veloxmesh:request_logs`: request-level rows derived from each run's `responses.jsonl` and `summary.json`.
- `veloxmesh:provider_health`: provider/model health aggregates derived from real run results.

The publisher writes all three keys after a benchmark. BFF reads them through a Redis-backed operational store. Missing keys produce explicit empty/partial states; production screens do not substitute mock rows.

## Authentication

The React app uses `/bff/auth/login`, `/bff/auth/verify-login`, `/bff/session`, and `/bff/auth/logout`. Session identity comes only from the HttpOnly BFF cookie. Demo-code login remains available only when the BFF runs in demo mode.

## Comparison model

Each benchmark row carries one of four method labels: `Local Baseline`, `Our Gateway Method`, `Improved Model`, or `Our Gateway + Improved Model`. Comparisons are valid only for rows with the same dataset and setup. Improvement is calculated against the matching Local Baseline; absent methods remain visibly missing rather than being fabricated.

## Persistence

Redis uses a named volume plus AOF (`appendonly yes`, `appendfsync everysec`). The publisher issues `SET` for the three canonical keys. Restart verification checks the key values without relying on a manual `SAVE`.

## UI

The benchmark page offers dataset/method filters, compact summary comparison, sticky table headers, and controlled horizontal scrolling. Provider Health and Requests/Logs use live BFF data. Loading, empty, error, forbidden, and partial states remain explicit.

## Verification

Go unit tests cover Redis decoding, operational endpoints, and authorization. Python tests cover report-to-Redis transformation. Vitest covers API mapping, authentication, comparison grouping, and exports. Playwright covers Admin login, Customer denial, live pages, filtering, and export actions.
