# Gateway Admin Integration Design

## Scope

Step 4 replaces Dashboard-local management truth with authenticated VeloxMesh Admin API calls. It covers Provider reads, Routing, Tenants, Admin API Keys, Audit, Usage, Settings, health, readiness, and topology. Runtime apply-and-verify behavior remains Step 5; Admin Home aggregation remains Step 6.

## Architecture

The VeloxMesh process exposes a new `AdminControlHandler` under `/admin/v1`. Its service uses the existing control-state repository for routing, API keys, audit, and usage, plus formal Tenant and safe System Settings repositories implemented by SQLite and PostgreSQL. Tenant identity is stored in VeloxMesh and API keys carry `tenant_id`; Dashboard-local Tenant rows are never treated as production Gateway entities.

The Dashboard BFF owns a `GatewayAdminClient`. In production, System Management handlers call this client and return `source: veloxmesh-admin`. `DemoMode` remains the only explicit path to `admin-state.json`. Missing configuration, network errors, timeouts, upstream status codes, and malformed payloads produce explicit errors or partial-data metadata; no automatic mock fallback is allowed.

## Security

- `VELOXMESH_ADMIN_API_KEY` stays in the BFF process and is sent only as `Authorization: Bearer`.
- Gateway API key secrets are generated with cryptographic randomness, returned once on create, and stored only as prefix plus SHA-256 hash.
- Provider secrets, API key hashes, Gateway Admin keys, SMTP credentials, and authorization headers are excluded from responses and audit metadata.
- All new Gateway endpoints use the existing Admin authentication middleware; writes also use the writable-node middleware.

## Gateway API

- `GET/PUT /admin/v1/routing`
- `GET/POST/PUT/DELETE /admin/v1/tenants`
- `GET/POST/DELETE /admin/v1/api-keys`
- `GET /admin/v1/audit`
- `GET /admin/v1/usage`
- `GET/PUT /admin/v1/settings`
- Existing Provider, Combo, Semantic Rules, Scheduler, Topology, Health, Readiness, and Metrics endpoints remain unchanged.

## Data Models

`TenantRecord` contains `id`, `owner`, `status`, `daily_quota`, `revision`, and timestamps. `APIKeyRecord` gains `tenant_id`. `SystemSettings` contains only safe operational values: default provider, default model, request timeout seconds, data retention days, revision, and timestamps.

SQLite and PostgreSQL receive additive migrations. New repository capabilities are exposed through optional management interfaces so existing lightweight test repositories do not need unrelated methods.

## BFF Mapping

Gateway snake-case contracts are converted into the existing browser-facing camel-case DTOs. Real responses carry `partialData: false`; errors include source and warnings. Production writes never modify Dashboard local management state. DemoMode keeps the Step 3 behavior for isolated tests and demonstrations.

## Testing

- Gateway service tests for validation, persistence, secret handling, tenant association, and audit.
- Gateway handler/router contract tests for authentication and JSON contracts.
- SQLite repository tests and migration checks; PostgreSQL compile and repository coverage where an external DSN is not required.
- BFF client tests for Bearer auth, timeout, status mapping, malformed payloads, and secret non-disclosure.
- BFF handler tests proving production uses Gateway and DemoMode remains local.
- Existing root, Dashboard Go, frontend, build, and Playwright suites must remain green.

## Deferred

- Runtime revision readback and verification requests are Step 5.
- Admin Home metric aggregation is Step 6.
- Full SMTP hardening and request-level report packaging are Steps 8 and 7 respectively.
