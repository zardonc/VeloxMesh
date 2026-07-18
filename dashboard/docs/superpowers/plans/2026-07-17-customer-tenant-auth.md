# Customer Tenant Authentication Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement real Customer registration, server-side tenant authorization, Customer APIs, and live Customer Dashboard data without regressing Admin behavior.

**Architecture:** Extend the existing JSON-backed Go BFF identity store and Redis operational store. Keep the current React application, add typed Customer API methods, and replace only Customer mock data paths.

**Tech Stack:** Go `net/http`, bcrypt, JSON state persistence, Redis operational snapshots, React 19, TypeScript, Vite, Vitest, Playwright.

---

### Task 1: Registration And Tenant Identity

**Files:**
- Modify: `internal/bff/server.go`
- Modify: `internal/bff/server_test.go`
- Modify: `go.mod`

- [ ] Add failing tests for Customer-only registration, unique email/username, generated tenant/user IDs, ignored role/tenant injection, persistence rollback, and challenge response.
- [ ] Run the focused tests and confirm expected failures.
- [ ] Add backward-compatible Tenant/User/API key fields and atomic state snapshot persistence.
- [ ] Add bcrypt while retaining legacy hash verification.
- [ ] Run focused and full Go tests.

### Task 2: Verification, Session, And RBAC

**Files:**
- Modify: `internal/bff/server.go`
- Modify: `internal/bff/server_test.go`

- [ ] Add failing tests for verification, Session identity, expiry, logout, Admin/Customer 401/403, and client tenant override attempts.
- [ ] Run the tests and confirm expected failures.
- [ ] Store full server-side Session records and add Customer middleware.
- [ ] Preserve existing auth endpoint aliases and Admin behavior.
- [ ] Run focused and full Go tests.

### Task 3: Tenant-Scoped Customer APIs

**Files:**
- Modify: `internal/bff/server.go`
- Modify: `internal/bff/server_test.go`

- [ ] Add failing Customer A/B tests for summary, usage, requests, API key list/create/delete, filters, pagination, empty data, and cross-tenant denial.
- [ ] Run the tests and confirm expected failures.
- [ ] Implement Customer endpoints using Session tenant ID and Redis-backed request rows.
- [ ] Persist only API key hash and mask; return full secret once.
- [ ] Run focused and full Go tests.

### Task 4: Frontend API Contract

**Files:**
- Modify: `web/admin-console/src/api.ts`
- Modify: `web/admin-console/src/api.test.ts`
- Modify: `web/admin-console/src/mvp.test.ts`

- [ ] Add failing tests for registration payloads, Customer API paths, data mapping, and no Admin calls.
- [ ] Run Vitest and confirm expected failures.
- [ ] Add typed Session tenant fields and Customer data/API key service methods.
- [ ] Remove Customer mock fallback behavior.
- [ ] Run Vitest.

### Task 5: Customer Authentication UI

**Files:**
- Modify: `web/admin-console/src/App.tsx`
- Modify: `web/admin-console/src/authCopy.ts`
- Modify: `web/admin-console/src/authCopy.test.ts`
- Modify: `web/admin-console/src/styles.css`

- [ ] Add failing tests for Customer registration copy and absence of Admin registration.
- [ ] Run tests and confirm expected failures.
- [ ] Implement Customer/Admin sign-in modes, Customer registration form, verification, Session restore, and logout.
- [ ] Preserve existing Admin navigation.
- [ ] Run tests and build.

### Task 6: Live Customer Dashboard

**Files:**
- Modify: `web/admin-console/src/App.tsx`
- Modify: `web/admin-console/src/api.ts`
- Modify: `web/admin-console/src/api.test.ts`
- Modify: `web/admin-console/src/styles.css`

- [ ] Add failing tests for summary, Usage, Requests, API Keys, empty/error/partial states, and masked key behavior.
- [ ] Run tests and confirm expected failures.
- [ ] Connect all Customer pages to `/bff/customer/*` and add key creation/revocation.
- [ ] Run tests and build.

### Task 7: End-To-End Acceptance

**Files:**
- Modify: `web/admin-console/e2e/dashboard.spec.ts`
- Create: `reports/customer-tenant-acceptance-<timestamp>/...`

- [ ] Add E2E coverage for Customer A/B registration, verification, Session restore, logout, permissions, live pages, and API key one-time display.
- [ ] Run all Go, Vitest, build, and Playwright checks.
- [ ] Capture 1440x900, 1024x768, and 390x844 evidence.
- [ ] Record passed requirements and remaining integration risks.

