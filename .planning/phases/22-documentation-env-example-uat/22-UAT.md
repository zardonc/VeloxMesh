---
status: testing
phase: 22-documentation-env-example-uat
source:
  - 22-01-SUMMARY.md
started: 2026-07-06T22:00:00Z
updated: 2026-07-06T22:00:00Z
---

# Phase 22 UAT

## Current Test

number: 1
name: Local automated Scheduler 1.0 evidence
expected: |
  Config examples, admin APIs, scheduler degradation, semantic-neighbor behavior, and vector-backed smoke paths pass with 60-second backend test timeouts.
awaiting: completion review

## Tests

| # | Check | Command | Expected Result | Actual Result | Notes | Failure Classification |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Config examples parse/load and stay secret-safe | `go test -timeout 60s ./internal/config` | `.env.example` and JSON examples keep optional systems disabled and reject secret-shaped placeholders. | Passed: `ok veloxmesh/internal/config 0.715s`. | Covers disabled-by-default startup and example safety. | None |
| 2 | Scheduler admin APIs | `go test -timeout 60s ./internal/http/handlers` | Scheduler status, SLA rules, training export, validation failures, auth, and safe audit behavior pass. | Passed: `ok veloxmesh/internal/http/handlers (cached)`. | Covers admin status/SLA/export APIs and admin validation failure. | None |
| 3 | Scheduler enable/disable and degradation | `go test -timeout 60s ./internal/scheduler` | Scheduler client fallback, Redis queue fallback, semantic-neighbor fallback, quality attribution, SLA promotion, and ONNX/predictive fallbacks pass. | Passed: `ok veloxmesh/internal/scheduler (cached)`. | Covers scheduler enable/disable, Scheduler down, Redis unavailable, Qdrant unavailable, ONNX unhealthy, and semantic-neighbor behavior through unit coverage. | None |
| 4 | Qdrant semantic-cache smoke | `go test -timeout 60s ./tests/integration -run TestSemanticCache -count=1` | Semantic cache/vector path returns expected cache headers with vector-backed behavior. | Passed: `ok veloxmesh/tests/integration 4.274s`. | Uses available local test environment. | None |
| 5 | Plan 4 PostgreSQL/pgvector-adjacent smoke | `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1` | Plan 4 PostgreSQL startup and chat path pass when required env is present. | Passed: `ok veloxmesh/tests/integration 4.246s`. | Covers pgvector deployment adjacency via Plan 4 PostgreSQL smoke. | None |
| 6 | Full real-provider UAT | `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1` | With `.env.local` providing `POSTGRES_TEST_DSN`, `DEV_API_KEY`, and `SANS_*` provider vars, a non-Gemini provider returns HTTP 200. | Not run. | Gated by real-provider resources in `.env.local`; reserve Gemini resources for Gemini-specific scenarios. | Non-blocking gated external check |

## Summary

total: 6
passed: 5
issues: 0
pending: 1
skipped: 0
blocked: 0

## Gaps

- Full real-provider UAT was not run in this pass. Required env: `POSTGRES_TEST_DSN`, `DEV_API_KEY`, `SANS_BASE_URL`, `SANS_PRIMARY_API_KEY`, `SANS_PRIMARY_MODELS`, `SANS_PRIMARY_DEFAULT_MODEL`. Classification: non-blocking gated external check.

## Failure Handling

No checks failed. If a future UAT row fails, record the command output, root cause, and blocking/non-blocking classification in the row before closing the phase.
