# Benchmark Dashboard Closed-Loop Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect real Gateway benchmark results to Redis, BFF, Dashboard, CSV, and HTML report using one traceable contract.

**Architecture:** The Python runner owns request-level measurement and canonical aggregation. A publisher validates and stores completed runs in Redis. The Go BFF exposes that data without silent production fallback, while React displays and exports the same typed records.

**Tech Stack:** Python standard library, Redis RESP, Go `net/http`, React, TypeScript, Vitest.

---

### Task 1: Canonical runner output

**Files:**
- Modify: `VeloxMesh/scripts/run-gateway-dataset.py`
- Create: `VeloxMesh/scripts/test_run_gateway_dataset.py`

- [ ] Write failing aggregation and response-validation tests.
- [ ] Run tests and confirm the expected failures.
- [ ] Implement full request metrics and canonical `BenchmarkRun` summary.
- [ ] Run Python tests and syntax checks.

### Task 2: Redis publisher

**Files:**
- Create: `VeloxMesh/scripts/publish-benchmark-results.py`
- Create: `VeloxMesh/scripts/test_publish_benchmark_results.py`
- Modify: `dashboard/scripts/test-scenarios/run-real-gateway-dashboard-flow.ps1`

- [ ] Write failing contract validation and snapshot tests.
- [ ] Implement report-directory loading and Redis publication.
- [ ] Replace the partial PowerShell mapping with the canonical publisher.
- [ ] Verify publication tests.

### Task 3: Complete BFF contract

**Files:**
- Modify: `dashboard/internal/bff/server.go`
- Modify: `dashboard/internal/bff/server_test.go`
- Modify: `dashboard/cmd/gateway/main.go`

- [ ] Write failing tests for complete numeric fields and empty live mode.
- [ ] Expand DTOs and add explicit demo-mode fallback.
- [ ] Verify admin permissions and Go tests.

### Task 4: Live Dashboard mapping

**Files:**
- Modify: `dashboard/web/admin-console/src/api.ts`
- Modify: `dashboard/web/admin-console/src/api.test.ts`
- Modify: `dashboard/web/admin-console/src/mvp.test.ts`
- Modify: `dashboard/web/admin-console/src/App.tsx`

- [ ] Write failing mapping/state tests.
- [ ] Consume canonical BFF records without fabricated defaults.
- [ ] Keep mock data only behind explicit demo mode.
- [ ] Verify frontend tests.

### Task 5: Trustworthy exports

**Files:**
- Modify: `dashboard/web/admin-console/src/api.ts`
- Modify: `dashboard/web/admin-console/src/mvp.test.ts`

- [ ] Write failing CSV/report content and analysis tests.
- [ ] Export canonical rows and computed evidence-based analysis.
- [ ] Include metadata, setup, methods, summary, charts, source, limitations, and appendix.
- [ ] Verify exports and production build.

### Task 6: End-to-end verification

**Files:**
- Modify: `dashboard/scripts/test-scenarios/run-real-gateway-dashboard-flow.ps1`
- Create: `dashboard/docs/benchmark-closed-loop-runbook.md`

- [ ] Run all offline test suites.
- [ ] Run a 5-item live sample only when a stable Provider is available.
- [ ] Confirm one `runId` across raw files, Redis, BFF, Dashboard, CSV, and report.
- [ ] Record actual pass/fail evidence; never substitute mock data.
