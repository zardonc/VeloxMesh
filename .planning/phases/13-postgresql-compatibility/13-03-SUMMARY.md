---
phase: 13-postgresql-compatibility
plan: 13-03
subsystem: semantic-cache
tags: [postgresql, pgvector, semantic-cache]

provides:
  - pgvector VectorAdapter
  - pgvector semantic-cache migration
  - scoped vector lookup mapped through repository entries

key-files:
  created:
    - internal/storage/pgvector.go
    - internal/storage/pgvector_test.go
    - internal/controlstate/migrations/postgres/0004_pgvector_semantic_cache.sql
  modified:
    - internal/cache/semantic.go
    - internal/cache/semantic_test.go
    - internal/app/semantic_cache.go
    - internal/controlstate/postgres/migrations.go

requirements-completed: [VECT-01, VECT-02]
completed: 2026-07-03
---

# Phase 13-03 Summary: pgvector Semantic Cache Path

## Accomplishments

- Added `PGVectorAdapter` behind the existing `storage.VectorAdapter` interface.
- Added dimension validation for pgvector insert and search.
- Added safe pgvector metadata allowlisting so raw prompts are not stored.
- Added pgvector schema/index migration and wired it into ordered PostgreSQL migrations.
- Wired `SEMANTIC_CACHE_VECTOR_STORE=pgvector` in app startup with degraded fallback on initialization failure.
- Updated semantic cache behavior so vector results map back through scoped repository entries instead of trusting vector metadata as the response source.

## Verification

- `go test -timeout 60s ./internal/cache ./internal/storage ./internal/app`
- `go test -timeout 60s ./internal/storage -run TestPGVector -count=1`
- `go test -timeout 60s ./...`
- `git diff --check`

`POSTGRES_TEST_DSN` integration coverage remains externally gated.

## Deviations from Plan

None.

## Self-Check: PASSED
