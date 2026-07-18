# Dashboard Latest Benchmark and Request Logs Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display the newest Redis benchmark on Admin Home and merge current request-level benchmark evidence into Requests / Logs.

**Architecture:** Keep Redis access behind the existing `benchmarkStore` and `benchmarkRequestStore` interfaces. Add pure mapping and merge helpers in the BFF, then use them from demo summary and request-log handlers while preserving partial-source behavior.

**Tech Stack:** Go 1.24, `net/http`, existing BFF stores, Vitest, Playwright

---

## Chunk 1: BFF Behavior

### Task 1: Admin Home latest benchmark

**Files:**
- Modify: `internal/bff/admin_summary.go`
- Modify: `internal/bff/admin_summary_test.go`

- [ ] Add a failing handler test proving Demo Mode returns the newest row from `BenchmarkStore`.
- [ ] Run `go test ./internal/bff -run TestDemoAdminSummaryUsesLatestPublishedBenchmark -count=1` and confirm the current response has a nil latest benchmark.
- [ ] Pass the request context into demo summary generation and overlay the latest benchmark snapshot.
- [ ] Include benchmark source status without making otherwise usable demo data fail.
- [ ] Rerun the focused test and confirm it passes.

### Task 2: Merge request-level benchmark logs

**Files:**
- Modify: `internal/bff/server.go`
- Modify: `internal/bff/server_test.go`

- [ ] Add failing tests for benchmark mapping, operational/benchmark merge, duplicate preference, descending sort, 1,000-row cap, and one-source failure.
- [ ] Run the focused request-log tests and confirm the latest benchmark IDs are missing before implementation.
- [ ] Map `benchmarkRequestDTO` to `requestLogDTO`, including dataset tenant, method, timestamps, HTTP/error details, tokens, latency, and TTFT.
- [ ] Merge by request ID, prefer benchmark rows, sort newest first, and cap at 1,000.
- [ ] Return source metadata, warnings, total rows, and truncation/partial flags.
- [ ] Rerun focused tests and confirm they pass.

## Chunk 2: Regression and Live Verification

### Task 3: Automated regression

**Files:**
- Test: `internal/bff/*_test.go`
- Test: `web/admin-console/src/*.test.ts`
- Test: `web/admin-console/e2e/dashboard.spec.ts`

- [ ] Run `go test ./...` in `dashboard`.
- [ ] Run `npm.cmd test` in `dashboard/web/admin-console`.
- [ ] Run `npm.cmd run build`.
- [ ] Run `npm.cmd run test:e2e`.

### Task 4: Live Redis and browser acceptance

**Files:**
- No source changes expected.

- [ ] Rebuild and restart the local BFF using the latest working tree.
- [ ] Confirm `/bff/admin/summary` reports the latest Step 9 Run ID.
- [ ] Confirm `/bff/admin/request-logs` includes both Step 9 Run IDs and reports the merged source.
- [ ] Confirm Admin Home and Requests / Logs display the new records.
- [ ] Confirm CSV and Report exports still contain 40 request rows.
- [ ] Record any remaining provider-rate-limit issue separately from Dashboard correctness.
