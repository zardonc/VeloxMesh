# Benchmark Store Integration Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the Benchmarks page to the local Redis and Qdrant services and show their live connection status while preserving the existing fallback rows.

**Architecture:** The BFF owns benchmark storage access through a small interface. Redis supplies an optional JSON benchmark snapshot, Qdrant supplies vector-store health metadata, and the API response includes both benchmark rows and storage status. The frontend renders that status as Benchmark metrics and keeps table behavior unchanged.

**Tech Stack:** Go `net/http` and `net` for BFF store checks, React/TypeScript/Vitest for UI mapping, existing Docker Redis and Qdrant services.

---

## Chunk 1: BFF Benchmark Storage

### Task 1: Add benchmark response contract tests

**Files:**
- Modify: `internal/bff/server_test.go`
- Modify: `internal/bff/server.go`

- [ ] Write a failing test that configures a fake benchmark store and expects `/bff/admin/benchmarks` to return rows plus Redis/Qdrant status.
- [ ] Run `go test ./internal/bff` and confirm the test fails because the contract is not implemented.
- [ ] Add the benchmark store interface, snapshot DTOs, fallback rows, Redis PING/GET reader, and Qdrant health reader.
- [ ] Run `go test ./internal/bff` and confirm the test passes.

### Task 2: Wire runtime config

**Files:**
- Modify: `cmd/gateway/main.go`
- Modify: `internal/bff/server.go`

- [ ] Read `REDIS_ADDR`, `QDRANT_URL`, `QDRANT_API_KEY`, and `QDRANT_BENCHMARK_COLLECTION` from `.env2.local` with local defaults.
- [ ] Ensure missing/empty stores degrade to fallback benchmark rows with visible status.
- [ ] Run `go test ./...`.

## Chunk 2: Frontend Benchmark Status

### Task 3: Map storage metadata into the Benchmark page

**Files:**
- Modify: `web/admin-console/src/api.ts`
- Modify: `web/admin-console/src/api.test.ts`

- [ ] Write a failing Vitest case expecting Benchmark metrics to include Source, Redis, and Qdrant status.
- [ ] Run `pnpm test` and confirm the test fails.
- [ ] Extend `BenchmarksResponse` and `buildBenchmarksPageViewModel` to surface storage metadata.
- [ ] Run `pnpm test` and confirm it passes.

## Chunk 3: End-to-End Verification

### Task 4: Verify locally

**Files:**
- No new files.

- [ ] Restart the local dev environment.
- [ ] Verify Redis and Qdrant are reachable.
- [ ] Open `http://127.0.0.1:5173/`, click Benchmarks, and confirm the page shows Redis/Qdrant status.
- [ ] Run `go test ./...`, `pnpm test`, and `pnpm build`.
