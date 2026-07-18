# Configuration Application Verification Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Provider and Routing changes verifiably active in the real Gateway and expose a truthful application result with complete, secret-free audit evidence.

**Architecture:** Persist through the real Admin API, synchronously replace the Gateway runtime snapshot, and let the BFF perform revision readback plus a minimal authenticated data-plane request. Return a shared application-status envelope to the React UI and propagate actor/request identity into audit records.

**Tech Stack:** Go, React, TypeScript, Vitest, Playwright, HTTP/JSON, VeloxMesh Admin API and data-plane API.

---

## Chunk 1: Runtime activation and audit context

### Task 1: Deterministic runtime route activation

**Files:**
- Modify: `internal/routing/router.go`
- Modify: `internal/routing/router_test.go`
- Modify: `internal/controlstate/runtime.go`
- Modify: `internal/controlstate/runtime_test.go`

- [ ] Write failing tests showing `default-provider` selects the configured provider and a new routing snapshot changes the next request decision.
- [ ] Run the focused Go tests and confirm the new assertions fail.
- [ ] Apply `RoutingConfig.DefaultProvider` to the runtime registry and implement the deterministic strategy.
- [ ] Run the focused tests and confirm they pass.

### Task 2: Apply Routing writes to the live runtime

**Files:**
- Modify: `internal/controlstate/admin_control_service.go`
- Modify: `internal/controlstate/admin_control_service_test.go`
- Modify: `internal/http/router.go`
- Modify: `internal/app/app.go`

- [ ] Write failing tests for synchronous apply, publication, and activation failure without false success.
- [ ] Run focused tests and confirm failure.
- [ ] Inject a runtime apply callback and configuration publisher; retain the previous atomic snapshot when activation fails.
- [ ] Run focused service and router tests.

### Task 3: Carry actor and request identity into Audit

**Files:**
- Modify: `internal/controlstate/audit.go`
- Modify: `internal/http/middleware/adminauth.go`
- Modify: `internal/http/middleware/requestid.go`
- Modify: `internal/controlstate/admin_control_service_test.go`

- [ ] Write failing tests for actor, outcome, revision, and request ID, including secret exclusion.
- [ ] Add typed context helpers and populate them only after Admin authentication.
- [ ] Add safe audit metadata for application and verification outcomes.
- [ ] Run audit, middleware, and control-state tests.

## Chunk 2: BFF closed-loop verification

### Task 4: Add data-plane verification client

**Files:**
- Modify: `dashboard/internal/bff/gateway_admin_client.go`
- Modify: `dashboard/internal/bff/gateway_admin_client_test.go`
- Modify: `dashboard/cmd/gateway/main.go`
- Modify: `dashboard/cmd/gateway/main_test.go`
- Modify: `dashboard/.env.example`
- Modify: `dashboard/README.md`

- [ ] Write failing tests for `/v1/models`, minimal chat verification, evidence headers, redirect protection, response limits, and key redaction.
- [ ] Add a separate `VELOXMESH_DATA_API_KEY` and verification result type.
- [ ] Implement authenticated verification calls without exposing either API key.
- [ ] Run focused client and startup configuration tests.

### Task 5: Orchestrate mutation, readback, and verification

**Files:**
- Modify: `dashboard/internal/bff/server.go`
- Modify: `dashboard/internal/bff/server_test.go`

- [ ] Write failing tests for Provider and Routing verified, warning, and failed outcomes.
- [ ] Require revision/updated-at change on readback.
- [ ] Return the shared application envelope and add `POST /bff/admin/runtime/verify` for safe retries.
- [ ] Forward only trusted actor and operation request ID to the Gateway Admin API.
- [ ] Run all BFF Go tests.

## Chunk 3: Dashboard state and end-to-end proof

### Task 6: Render truthful application status

**Files:**
- Modify: `dashboard/web/admin-console/src/api.ts`
- Modify: `dashboard/web/admin-console/src/api.test.ts`
- Modify: `dashboard/web/admin-console/src/SystemManagement.tsx`
- Modify: `dashboard/web/admin-console/src/mvp.test.ts`
- Modify: `dashboard/web/admin-console/src/styles.css`

- [ ] Write failing tests for verified, warning, and failed result rendering.
- [ ] Add TypeScript application-status types and preserve mutation responses.
- [ ] Render evidence and safe diagnostics; never turn warning/failed into success.
- [ ] Run Vitest and production build.

### Task 7: Prove the real closed loop

**Files:**
- Modify: `dashboard/web/admin-console/e2e/dashboard.spec.ts`
- Modify: `dashboard/scripts/test-scenarios/run-real-gateway-dashboard-flow.ps1`
- Modify: `dashboard/docs/benchmark-closed-loop-runbook.md`

- [ ] Add an E2E scenario that changes the default route and observes the new provider on the next request.
- [ ] Assert Audit contains the actor, revision, outcome, and verification request ID and contains no secret.
- [ ] Run root focused Go tests, Dashboard Go tests, Vitest, build, and Playwright.
- [ ] Record known environment-only failures separately; do not claim they are Step 5 regressions.

