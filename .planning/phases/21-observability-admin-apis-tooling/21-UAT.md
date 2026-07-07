---
status: complete
phase: 21-observability-admin-apis-tooling
source:
  - 21-01-SUMMARY.md
  - 21-02-SUMMARY.md
  - 21-03-SUMMARY.md
started: 2026-07-06T20:53:07Z
updated: 2026-07-06T23:25:00Z
---

# Phase 21 UAT

## Current Test

[testing complete]

## Tests

| # | Check | Command | Expected Result | Actual Result | Notes | Failure Classification |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Scheduler status partial runtime visibility | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerStatus -count=1 -v` | Admin status returns queue depth, executor slots, rollout status, circuit breaker state, quality rollups, and warnings for unavailable components. | Passed. | Covers auth, default/limit rollup behavior, partial warnings, and circuit breaker state. | None |
| 2 | SLA rules read and successful replacement | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerSLARulesReplaceAuditsSafeMetadata -count=1 -v` | Writable admin replacement swaps active rules and emits safe audit metadata. | Passed. | Audit metadata includes counts and safe rule keys only. | None |
| 3 | SLA rules invalid replacement rejection | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerInvalidSLARulesLeaveOldRules -count=1 -v` | Invalid submitted rules are rejected and old rules remain active. | Passed. | Validates all-or-nothing replacement behavior. | None |
| 4 | Scheduler training export safe formats | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerTrainingExportJSONAndNDJSONAreSafe -count=1 -v` | JSON default and NDJSON export return safe features/labels only. | Passed. | Uses real SQLite-backed repository fixtures through the admin handler. | None |
| 5 | Scheduler training export validation failures | `go test -timeout 60s ./internal/http/handlers -count=1` | Invalid limits, time filters, or formats are rejected without raw record leakage. | Passed. | Covered by the handler package's admin scheduler request parsing and safe export tests; no raw training records are returned on validation failures. | None |
| 6 | Exact semantic neighbor hydration across repositories | `go test -timeout 60s ./internal/scheduler ./internal/controlstate/sqlite ./internal/controlstate/postgres -run 'TestSemanticNeighborHydrationUsesExactIDsInVectorOrder|ListByIDsPreservesOrderAndOmitsMissing' -count=1 -v` | Hydration uses exact vector result IDs, preserves result order, and omits missing IDs across SQLite/Postgres repositories. | Passed. | Postgres test used real configured Postgres and logged the expected non-TLS DSN warning. | None |
| 7 | Semantic neighbor embedding model configuration | `go test -timeout 60s ./internal/config ./internal/scheduler -run 'SemanticNeighbors.*Model|SemanticNeighborEmbeddingUses' -count=1 -v` | Default model is `text-embedding-3-small`; env/JSON overrides flow into `SemanticNeighborService`. | Passed. | Covers default, configured model, and empty fallback behavior. | None |
| 8 | Narrow heuristic override file handling | `go test -timeout 60s ./internal/scheduler/heuristic ./internal/config -count=1` | Only `base_latency` and `model_multipliers` overrides are accepted; unknown fields fail explicitly. | Passed. | Covers safe template and narrow loader behavior. | None |
| 9 | Non-empty SchedulerType quality attribution | `go test -timeout 60s ./internal/scheduler -run 'TestScoreWithDefaultTypePreventsEmptyQualityMetadata|Test.*SchedulerType|Test.*Quality' -count=1 -v` | FIFO, heuristic, gRPC, weighted, fallback, and quality metadata paths record non-empty scheduler type. | Passed. | Prevents empty scheduler type in quality rollups. | None |

## Summary

total: 9
passed: 9
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

None.

## Failure Handling

No checks failed. Future failures should record the command output, root cause, and blocking/non-blocking classification before phase close.
