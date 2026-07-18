# Role-Specific Login Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add separate Admin and Customer login URLs with server-enforced portal-role matching.

**Architecture:** The React application derives a fixed login portal from the browser path and calls a role-specific BFF endpoint. The BFF shares existing credential and challenge logic but rejects a verified account when its stored role does not match the requested portal.

**Tech Stack:** React, TypeScript, Vite, Vitest, Go `net/http`, Go tests, Playwright.

---

## Chunk 1: BFF Role Enforcement

### Task 1: Add role-specific login endpoint tests

**Files:**
- Modify: `internal/bff/server_test.go`

- [ ] Add a test that an Admin account can request a challenge from `/bff/auth/admin/login`.
- [ ] Add a test that the same Admin account receives 403 from `/bff/auth/customer/login`.
- [ ] Add the equivalent Customer assertions.
- [ ] Assert mismatch responses contain the portal-specific error, set no Session Cookie, and do not add a login challenge.
- [ ] Assert the generic `/bff/auth/login` endpoint still accepts valid credentials for compatibility.
- [ ] Run `go test ./internal/bff -run RoleSpecificLogin -count=1` and verify the new tests fail because the routes do not exist.

### Task 2: Implement shared role-aware login handling

**Files:**
- Modify: `internal/bff/server.go`

- [ ] Register `/bff/auth/admin/login` and `/bff/auth/customer/login`.
- [ ] Refactor the existing handler into a shared helper accepting an optional expected role.
- [ ] Compare expected and stored roles only after successful password verification.
- [ ] Return 403 without creating a challenge on mismatch.
- [ ] Run the focused Go test and verify it passes.
- [ ] Run `go test ./...`.

## Chunk 2: Frontend Portal Routing

### Task 3: Add failing API and portal tests

**Files:**
- Modify: `web/admin-console/src/api.test.ts`
- Modify: `web/admin-console/src/authCopy.test.ts`

- [ ] Assert Admin login uses `/bff/auth/admin/login`.
- [ ] Assert Customer login uses `/bff/auth/customer/login`.
- [ ] Assert `/admin/login` and `/customer/login` map to fixed roles.
- [ ] Assert every other unauthenticated pathname falls back to the Customer portal.
- [ ] Assert the role-mismatch response message is surfaced by the API service.
- [ ] Run `npm.cmd --prefix web/admin-console test` and verify failures describe the missing behavior.

### Task 4: Implement fixed portal UI

**Files:**
- Modify: `web/admin-console/src/api.ts`
- Modify: `web/admin-console/src/authCopy.ts`
- Modify: `web/admin-console/src/App.tsx`

- [ ] Add the portal role to `LoginInput` and choose the role-specific endpoint.
- [ ] Export a small pathname-to-role helper.
- [ ] Pass the fixed portal role into `LoginScreen`.
- [ ] Replace the role segmented control with a link to the other portal.
- [ ] Use `history.pushState` for portal links and a `popstate` listener so browser Back and Forward update the rendered portal without a reload.
- [ ] Keep Customer registration available only on the Customer portal.
- [ ] Run Vitest and verify all tests pass.

## Chunk 3: Acceptance Verification

### Task 5: Update end-to-end login helpers

**Files:**
- Modify: `web/admin-console/e2e/dashboard.spec.ts`

- [ ] Navigate Admin tests to `/admin/login`.
- [ ] Navigate Customer tests to `/customer/login`.
- [ ] Assert a valid Customer account receives the role-mismatch message on `/admin/login` and remains logged out.
- [ ] Assert browser Back and Forward switch between the two login portals.

### Task 6: Run complete verification

- [ ] Run `go test ./...`.
- [ ] Run `npm.cmd --prefix web/admin-console test`.
- [ ] Run `npm.cmd --prefix web/admin-console run build`.
- [ ] Run `npm.cmd --prefix web/admin-console run test:e2e`.
- [ ] Confirm only `dashboard/` files changed.
