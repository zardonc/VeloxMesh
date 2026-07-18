# Dashboard Latest Benchmark and Request Log Design

## Goal

Make the Admin Home and Requests / Logs views consume the latest request-level benchmark snapshots already published to Redis.

## Admin Home

The admin summary reads the benchmark store in both production and explicit demo mode. It selects the newest benchmark by `testDate` and exposes it as `latestBenchmark`. If the benchmark source is empty or unavailable, the existing summary remains usable and reports a partial source instead of inventing benchmark values.

## Requests / Logs

The request-log endpoint reads both the operational store and `veloxmesh:benchmark_requests`. Benchmark requests are mapped to the existing request-log contract and merged with operational rows.

- Deduplicate by `requestId`.
- Prefer the benchmark row when the same request ID exists in both stores.
- Sort newest first using the request timestamp.
- Return at most the newest 1,000 rows.
- Expose source status, warnings, total rows, and whether the result was truncated.
- Preserve usable data when either source is empty or unavailable.

Benchmark mappings:

- `tenant`: `benchmark/<dataset>`
- `method`: benchmark method label
- `timestamp`: `startedAt`, falling back to `endedAt`
- `errorMessage`: error type and HTTP status for failed requests
- latency, TTFT, provider, model, and token fields map directly

## Error Handling

One failed source does not hide rows from the other source. An empty result is returned only when neither source contains rows. The response identifies empty or unavailable sources without filling demo request records when real benchmark evidence exists.

## Verification

Automated tests cover:

1. Demo Admin Home exposes the latest real benchmark.
2. Request logs merge operational and benchmark rows.
3. Duplicate request IDs prefer benchmark evidence.
4. Results are sorted newest first and capped at 1,000 rows.
5. One unavailable source preserves data from the other source.
6. Existing Go, frontend, build, and E2E suites remain green.
7. The running Dashboard displays the two Step 9 Run IDs and the Admin Home latest benchmark.
