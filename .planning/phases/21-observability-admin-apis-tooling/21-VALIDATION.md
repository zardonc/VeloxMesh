---
phase: 21
slug: observability-admin-apis-tooling
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-07-06
audited: 2026-07-06T16:25:00-07:00
---

# Phase 21 - Validation Strategy

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Go `testing` via `go test` |
| Config file | `go.mod` |
| Quick run command | `go test -timeout 60s ./internal/http/handlers ./internal/scheduler ./internal/controlstate/sqlite ./internal/controlstate/postgres` |
| Full suite command | `go test -timeout 60s ./...` |
| Estimated runtime | ~60 seconds |

## Sampling Rate

- After every task commit: run the package command listed for that task.
- After every plan wave: run `go test -timeout 60s ./internal/http ./internal/http/handlers ./internal/scheduler ./internal/controlstate/sqlite ./internal/controlstate/postgres`.
- Before `$gsd-verify-work`: full suite must be green.
- Max feedback latency: 60 seconds.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 21-01-01 | 01 | 1 | SCH-08 | T-21-01-01 | Status endpoint returns safe partial runtime state with warnings. | unit/integration | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerStatus -count=1 -v` | yes | green |
| 21-01-02 | 01 | 1 | OBS-03 | T-21-01-02 | SLA rule replacement validates full submitted set before atomic swap. | unit | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerSLARules -count=1 -v` | yes | green |
| 21-01-03 | 01 | 1 | OBS-03 | T-21-01-03 | SLA replacement audit stores safe counts/keys only. | integration | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerSLARulesReplaceAuditsSafeMetadata -count=1 -v` | yes | green |
| 21-02-01 | 02 | 1 | QDR-07 | T-21-02-03 | SQLite and Postgres hydrate exact requested sample IDs in order. | integration | `go test -timeout 60s ./internal/controlstate/sqlite ./internal/controlstate/postgres -run 'ListByIDsPreservesOrderAndOmitsMissing' -count=1 -v` | yes | green |
| 21-02-02 | 02 | 1 | QDR-07 | T-21-02-03 | Semantic-neighbor hydration uses vector result IDs and preserves order. | unit | `go test -timeout 60s ./internal/scheduler -run TestSemanticNeighborHydrationUsesExactIDsInVectorOrder -count=1 -v` | yes | green |
| 21-02-03 | 02 | 1 | OBS-04 | T-21-02-01 / T-21-02-02 | Training export is bounded and projects safe features/labels only. | integration | `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerTrainingExportJSONAndNDJSONAreSafe -count=1 -v` | yes | green |
| 21-03-01 | 03 | 2 | QDR-08 | T-21-03-02 | Semantic-neighbor embedding model defaults and overrides are honored. | unit | `go test -timeout 60s ./internal/config ./internal/scheduler -run 'SemanticNeighbors.*Model|SemanticNeighborEmbeddingUses' -count=1 -v` | yes | green |
| 21-03-02 | 03 | 2 | OBS-06 | T-21-03-01 | Heuristic override file accepts only approved tables and rejects unknowns. | unit | `go test -timeout 60s ./internal/scheduler/heuristic ./internal/config -count=1` | yes | green |
| 21-03-03 | 03 | 2 | OBS-05 | T-21-03-03 | Score metadata never records empty `SchedulerType`. | unit | `go test -timeout 60s ./internal/scheduler -run 'TestScoreWithDefaultTypePreventsEmptyQualityMetadata|Test.*SchedulerType|Test.*Quality' -count=1 -v` | yes | green |

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. No new test framework or fixtures are required.

## Manual-Only Verifications

All phase behaviors have automated verification. Mock-only or skipped tests are not counted as validation evidence.

## Real Component Evidence

| Component | Command | Result |
|-----------|---------|--------|
| Admin API + SQLite | `go test -timeout 60s ./internal/http/handlers -run 'TestAdminSchedulerStatus|TestAdminSchedulerSLARules|TestAdminSchedulerTrainingExport|TestAdminSchedulerRolloutUsesRealSQLiteRepository|TestAdminSchedulerAuditMetadataIsSanitized' -count=1 -v` | passed |
| SQLite training sample lookup | `go test -timeout 60s ./internal/controlstate/sqlite -run TestSchedulerTrainingSamplesListByIDsPreservesOrderAndOmitsMissing -count=1 -v` | passed |
| Postgres training sample lookup | `go test -timeout 60s ./internal/controlstate/postgres -run TestPostgresSchedulerTrainingSamplesListByIDsPreservesOrderAndOmitsMissing -count=1 -v` | passed against real Postgres; plaintext DSN warning logged |
| Scheduler tuning + ONNX | `go test -timeout 60s ./internal/scheduler/heuristic ./cmd/scheduler ./internal/scheduler/onnx -count=1` | passed |

## Validation Sign-Off

- [x] All tasks have automated verify commands.
- [x] Sampling continuity: no 3 consecutive tasks without automated verify.
- [x] Wave 0 covers all MISSING references.
- [x] No watch-mode flags.
- [x] Feedback latency < 60s.
- [x] `nyquist_compliant: true` set in frontmatter.

**Approval:** approved 2026-07-06
