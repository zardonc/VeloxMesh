---
phase: 13-postgresql-compatibility
plan: 13-04
subsystem: migration-smoke
tags: [postgresql, migration, smoke-test]

provides:
  - SQLite-to-PostgreSQL migration command
  - Stop-first migration failure report
  - Plan 4 PostgreSQL smoke test
  - Operator migration and smoke runbook

key-files:
  created:
    - cmd/controlstate-migrate/main.go
    - internal/controlstate/migration/sqlite_to_postgres.go
    - internal/controlstate/migration/sqlite_to_postgres_test.go
    - tests/integration/plan4_postgres_smoke_test.go
    - scripts/smoke/plan4-postgres.sh
  modified:
    - README.md

requirements-completed: [MIGR-01, MIGR-02, TEST-01, PG-01, PG-03]
completed: 2026-07-03
---

# Phase 13-04 Summary: Migration and Plan 4 Smoke

## Accomplishments

- Added `cmd/controlstate-migrate` for repeatable SQLite-to-PostgreSQL control-state migration.
- Added idempotent table-by-table upserts for supported control-state records.
- Added `MigrationReport` stop-first failure reporting with completed tables, failed table, record key, root error, and repair guidance.
- Added Plan 4 smoke test gated by external PostgreSQL/provider environment values.
- Added `scripts/smoke/plan4-postgres.sh` as a thin wrapper around the Go smoke test.
- Documented migration, repair, retry, and smoke flow in `README.md`.

## Verification

- `go test -timeout 60s ./internal/controlstate/migration`
- `go test -timeout 60s ./internal/controlstate/migration -run TestMigrationStopsWithReport -count=1`
- `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1`
- `go test -timeout 60s ./...`
- `git diff --check`

External Docker and deployed-service lifecycle checks remain operator-managed.

## Deviations from Plan

- `Agent-gateway/gateway-postgres.md` could not be created because the apply-patch approval service repeatedly returned `codex-auto-review 404`. The runbook content was added to `README.md` instead.

## Self-Check: PASSED
