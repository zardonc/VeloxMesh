# Gateway Admin Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect Dashboard System Management to authenticated, persistent VeloxMesh Admin APIs without production fallback to Dashboard-local state.

**Architecture:** Add formal Gateway management contracts and storage, then introduce a server-side BFF client and switch handlers by explicit DemoMode. Keep Gateway contracts snake_case and browser contracts camelCase; keep all secrets server-side.

**Tech Stack:** Go 1.26, chi, SQLite, PostgreSQL/pgx, React TypeScript, Vitest, Playwright.

---

## Chunk 1: Gateway Management Contracts

### Task 1: Models and repositories

**Files:**
- Modify: `internal/controlstate/types.go`
- Modify: `internal/controlstate/repository.go`
- Create: `internal/controlstate/admin_control_service.go`
- Test: `internal/controlstate/admin_control_service_test.go`

- [ ] Write failing service tests for routing, tenants, keys, audit, usage, and settings.
- [ ] Add Tenant and safe Settings models, filters, and optional management repository interfaces.
- [ ] Implement validation, API key random-secret/hash behavior, audit recording, and error contracts.
- [ ] Run `go test ./internal/controlstate -run AdminControl -count=1`.

### Task 2: SQLite and PostgreSQL persistence

**Files:**
- Create: `internal/controlstate/migrations/sqlite/0011_gateway_admin.sql`
- Create: `internal/controlstate/migrations/postgres/0010_gateway_admin.sql`
- Create: `internal/controlstate/sqlite/admin_management.go`
- Create: `internal/controlstate/postgres/admin_management.go`
- Modify: SQLite/PostgreSQL API key and usage repository queries
- Test: repository and migration tests

- [ ] Write failing SQLite migration/repository tests.
- [ ] Add tenants, system settings, and API key `tenant_id` migrations.
- [ ] Implement Tenant, Settings, and Usage read repositories for both backends.
- [ ] Run SQLite tests and compile PostgreSQL packages.

### Task 3: Gateway HTTP handlers

**Files:**
- Create: `internal/http/handlers/admin_control.go`
- Create: `internal/http/handlers/admin_control_test.go`
- Modify: `internal/http/router.go`

- [ ] Write failing authenticated contract tests for every new endpoint.
- [ ] Implement JSON decoding, validation/status mapping, safe responses, and Admin middleware registration.
- [ ] Verify unauthenticated requests return `401` and writes respect writable-node state.
- [ ] Run handler and router tests.

## Chunk 2: Dashboard BFF Client

### Task 4: GatewayAdminClient

**Files:**
- Create: `dashboard/internal/bff/gateway_admin_client.go`
- Create: `dashboard/internal/bff/gateway_admin_client_test.go`
- Modify: `dashboard/internal/bff/server.go`

- [ ] Write failing client tests for URLs, Bearer auth, timeout, upstream errors, malformed payloads, and missing fields.
- [ ] Implement typed client methods for Provider, Routing, Tenant, API Key, Audit, Usage, Settings, health, readiness, and topology.
- [ ] Ensure errors never contain the Admin key or Authorization header.
- [ ] Run Dashboard BFF client tests.

### Task 5: Production handler switching

**Files:**
- Modify: `dashboard/internal/bff/server.go`
- Modify: `dashboard/internal/bff/server_test.go`
- Modify: `dashboard/cmd/gateway/main.go`
- Modify: `dashboard/cmd/gateway/main_test.go`

- [ ] Write failing tests proving production reads/writes Gateway and never mutates local state.
- [ ] Add `VELOXMESH_ADMIN_URL`, `VELOXMESH_DATA_URL`, `VELOXMESH_METRICS_URL`, `VELOXMESH_ADMIN_API_KEY`, and `VELOXMESH_API_TIMEOUT` configuration.
- [ ] Map Gateway contracts to browser DTOs with explicit source/partial warnings.
- [ ] Keep local storage only when DemoMode is explicitly enabled.
- [ ] Run Dashboard Go tests.

## Chunk 3: Frontend, Documentation, and Verification

### Task 6: Frontend contract adaptation

**Files:**
- Modify: `dashboard/web/admin-console/src/api.ts`
- Modify: `dashboard/web/admin-console/src/SystemManagement.tsx`
- Modify: frontend unit and E2E tests

- [ ] Add failing tests for real-source metadata, Gateway routing fields, and production errors.
- [ ] Display real source/revision and disable unsupported operations where the Gateway contract is read/write-singleton.
- [ ] Preserve loading, empty, error, partial, and responsive behavior.
- [ ] Run frontend tests, build, and E2E.

### Task 7: Examples and final verification

**Files:**
- Modify: `dashboard/README.md`
- Create: `dashboard/.env.example` if no safe example exists
- Modify: relevant deploy example environment files only with variable names/placeholders

- [ ] Document production versus DemoMode and all new environment variables without real credentials.
- [ ] Run root Go tests, Dashboard Go tests, frontend tests, build, and Playwright.
- [ ] Run `git diff --check` and sensitive-information scans.
- [ ] Stop after Step 4; do not implement Step 5 verification flow.
