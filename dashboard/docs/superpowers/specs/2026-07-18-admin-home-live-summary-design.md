# Admin Home Live Summary Design

## Goal

Replace every production Admin Home demo metric with a BFF-owned summary assembled from live VeloxMesh and dashboard stores. Missing sources must remain visible as unavailable data instead of being replaced by zeroes or fixed examples.

## Sources

- Gateway health and readiness: `/healthz`, `/readyz`
- Active providers: `/admin/v1/providers`
- Topology: `/admin/v1/topology`
- Queue depth: Prometheus metrics
- Requests today and latency/outcome rates: Operational Store request logs
- Provider health and recent errors: Operational Store
- Latest benchmark: Benchmark Store

The BFF is the only browser-facing integration boundary. The browser does not call VeloxMesh, Redis, Qdrant, or Prometheus directly.

## Response Contract

`GET /bff/admin/summary` returns nullable metrics together with `generatedAt`, `dataSources`, `partial`, and `warnings`. A source entry identifies its name, concrete source, status, detail, and source timestamp when available.

When at least one source succeeds, the endpoint returns HTTP 200. Any failed or empty required source sets `partial=true` while preserving successful values. When every source is unavailable, the endpoint returns HTTP 503 and an explicit error payload; it never manufactures zero metrics.

Demo values are allowed only when both the server and frontend are explicitly started in demo mode.

## Calculations

Only request logs whose timestamp falls on the current UTC date contribute to Requests Today. Average latency, P95 latency, success rate, error rate, and timeout rate use the same filtered request set. Timeout rows form a subset distinct from other errors, and all three outcome percentages use the same denominator.

Active Providers counts enabled providers returned by the real Admin API. Queue Depth sums the current `gateway_queue_depth` Prometheus gauge series. Latest Benchmark is selected by the newest valid test date, with the snapshot order used as a stable fallback.

## Frontend

Admin Home renders the exact summary values returned by the BFF. Nullable metrics display `Unavailable`. It shows gateway status, source timestamp, source labels, warnings, provider snapshot, latest benchmark, and the existing global Refresh command. `partial=true` produces a detailed Partial data banner; HTTP 503 produces the existing Error state.

## Testing

- BFF aggregation with all live sources
- one failed source preserves other values and marks Partial
- all sources failed returns 503 without demo numbers
- request calculations and Prometheus queue parsing
- frontend values match the BFF response exactly
- nullable metrics, source labels, warnings, timestamp, and refresh
- production mode cannot fall back to demo data

