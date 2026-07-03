# Phase 13: PostgreSQL Compatibility - Context

**Gathered:** 2026-07-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 13 delivers the Plan 4 PostgreSQL-compatible deployment path for VeloxMesh: a Redis Stack + PostgreSQL + pgvector runtime option, PostgreSQL repository parity with the active SQLite-backed control-state surface, pgvector semantic-cache vector search, SQLite-to-PostgreSQL migration tooling/runbook, and an end-to-end Plan 4 smoke verification. Plans 1/2 remain SQLite + Redis Stack + Qdrant by default, and the OpenAI-compatible data-plane contract must not change.

</domain>

<decisions>
## Implementation Decisions

### Deployment and Readiness
- **D-01:** Plan 4 should use a separate Docker Compose override for PostgreSQL + pgvector. Keep the current `docker-compose.yml` focused on the existing Plan 1/2 stack.
- **D-02:** Add a secret-safe example config and `.env.example` for Plan 4. Examples must use placeholders only; do not commit real DSNs, passwords, API keys, provider secrets, or encryption keys.
- **D-03:** PostgreSQL control-state availability is fail-closed. If `CONTROL_STATE_BACKEND=postgres` and PostgreSQL is unavailable or migrations fail, gateway startup must fail clearly.
- **D-04:** pgvector/vector availability may degrade. If `SEMANTIC_CACHE_VECTOR_STORE=pgvector` cannot initialize, semantic-cache/vector capability can degrade while core OpenAI-compatible proxying continues when relational control state is healthy.
- **D-05:** Public readiness stays coarse: ordinary `/readyz` reports ready/not ready only. PostgreSQL/pgvector dependency details belong in admin/internal health, logs, or operator diagnostics, following the Phase 12 topology secrecy boundary.
- **D-06:** Plan 4 smoke verification should cover compose override startup, PostgreSQL/pgvector migrations, gateway startup, and one OpenAI-compatible chat request using PostgreSQL-backed control state.

### PostgreSQL Repository Parity
- **D-07:** Phase 13 should pursue SQLite full parity for PostgreSQL repositories, including current SQLite-only surfaces such as `LimitRules` and `SessionBlacklist`, not only the minimum boot/chat path.
- **D-08:** Unimplemented PostgreSQL repository capabilities must fail closed with explicit unsupported errors. Do not return nil repositories or fake success for missing behavior.
- **D-09:** Missing parity items are blocking planner tasks. The planner should either implement them or leave a deliberate explicit unsupported path with tests proving callers see the failure.
- **D-10:** `PostgreSQLCapabilityProfile()` uses truthful per-feature flags. A flag becomes true only when the capability is implemented and verified for PostgreSQL.

### pgvector Semantic Path
- **D-11:** pgvector is the Plan 4 semantic vector store only when `SEMANTIC_CACHE_VECTOR_STORE=pgvector`. Plans 1/2 continue to default to Qdrant.
- **D-12:** The first pgvector implementation should include HNSW/IVFFlat indexing and tunable index/search parameters from day one.
- **D-13:** Vector dimension must be explicit and visible in configuration, with a reasonable documented default. Startup and insert paths must validate actual embedding dimensions to prevent mixed-provider dimension corruption.
- **D-14:** pgvector privacy and scoping must reuse the existing semantic-cache `scope + model` filtering. Do not store raw prompts. Store only vectors, response payloads already allowed by the semantic cache, `usage_id`, and safe metadata.

### Migration and Verification
- **D-15:** Provide both a migration runbook and an idempotent migrator command/script for SQLite-to-PostgreSQL control-state migration.
- **D-16:** Migration scope includes provider configs, encrypted provider secrets, routing config, API keys, provider model rates, usage records, semantic cache records/metadata, and fallback log state.
- **D-17:** Migration failures should stop immediately with a report listing completed tables, failed table/record context, the root error, and suggested repair steps. Do not automatically roll back everything or best-effort skip failed tables.

### the agent's Discretion
- The planner may choose exact file names for the compose override, example config, migration command, and smoke script as long as secrets stay out of git-managed source and operator steps remain clear.
- The planner may choose HNSW vs IVFFlat defaults and parameter names, but must keep them documented, configurable, and covered by focused tests or smoke checks.
- The planner may split Phase 13 into the existing four candidate plans from ROADMAP.md or adjust plan boundaries if coverage remains complete.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning Scope
- `.planning/ROADMAP.md` — Phase 13 goal, success criteria, candidate plan slices, and Plan 4 boundary.
- `.planning/REQUIREMENTS.md` — v7.3 requirement IDs and traceability for PostgreSQL deployment, repository parity, pgvector semantic path, migration, and verification.
- `.planning/PROJECT.md` — project constraints, core value, Plan 4 extension decision, and security/data-plane contract boundaries.
- `.planning/STATE.md` — current milestone and phase status.

### Prior Phase Decisions
- `.planning/phases/12-multi-node-coordination/12-CONTEXT.md` — fail-closed write/readiness behavior, topology secrecy, Redis coordination boundary, and admin/internal health guidance.
- `.planning/phases/10-advanced-routing-observability/10-CONTEXT.md` — low-cardinality/sanitized observability and opt-in rollout patterns.
- `.planning/phases/09-redis-stack-qdrant-fallback-integration/09-CONTEXT.md` — Redis hot-state boundary, Qdrant primary vector path for Plans 1/2, and Redis VSS fallback constraints.

### Existing Code Touchpoints
- `internal/app/app.go` — app wiring for control-state backend selection, semantic-cache vector store selection, Redis coordination, migrations, and reload paths.
- `internal/config/config.go` — environment/config-file shape and validation for control state, Redis, semantic cache, and Qdrant settings.
- `docker-compose.yml` — current base compose stack that should remain Plan 1/2-oriented while Plan 4 uses an override.
- `internal/controlstate/repository.go` — repository interface that PostgreSQL must match or explicitly fail closed.
- `internal/controlstate/postgres/repository.go` — existing PostgreSQL implementation with current gaps such as nil/stubbed SQLite-only surfaces.
- `internal/controlstate/sqlite/repository.go` — SQLite behavior used as parity reference.
- `internal/controlstate/capabilities.go` — capability profile that must become truthful per feature.
- `internal/storage/interfaces.go` — `VectorAdapter` boundary for the pgvector adapter.
- `internal/storage/adapters.go` — existing no-op/degraded vector patterns.
- `internal/storage/qdrant.go` — existing Qdrant vector adapter and Plans 1/2 default vector path.
- `internal/storage/redis_vss.go` — Redis VSS fallback path; do not conflate with Plan 4 pgvector.
- `internal/cache/semantic.go` — semantic cache lookup/store behavior and `scope + model` scoping expectations.
- `internal/controlstate/migrations/postgres/0001_control_state.sql` — current PostgreSQL schema, including semantic cache storage as `BYTEA`.
- `internal/controlstate/postgres/migrations.go` — current PostgreSQL migrator behavior and migration file list.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `controlstate.Repository`: shared control-state contract for providers, combos, routing, API keys, rates, usage, audit, idempotency, semantic cache, fallback log, limits, session blacklist, transactions, and settlement.
- `postgres.Repository`: existing PostgreSQL repository with provider, combo, routing, API key, rate, usage, audit, idempotency, semantic cache, fallback log, and settlement paths already started.
- `sqlite.Repository`: parity reference for SQLite-only `LimitRules`, `SessionBlacklist`, and mature semantic cache behavior.
- `storage.VectorAdapter`: existing adapter boundary where pgvector should fit without changing gateway handlers.
- `cache.SemanticCacheService`: existing service that already scopes candidates by `scope + model` and delegates vector behavior to the adapter.
- `config.Config`: existing env/config-file pattern for adding Plan 4 settings without hardcoding configuration.

### Established Patterns
- Durable relational state is selected through `CONTROL_STATE_BACKEND`; `postgres` is already an accepted value.
- Migrations can run at startup when `CONTROL_STATE_MIGRATE_ON_STARTUP` is enabled.
- Semantic cache is opt-in and can degrade vector capability without breaking core forwarding.
- Redis is hot state and coordination, not relational source of truth.
- Plans 1/2 use Qdrant as the default vector path; Plan 4 pgvector must not silently replace it.
- Public health/readiness should avoid leaking topology or backend detail; admin/internal surfaces may carry diagnostics.

### Integration Points
- Add Plan 4 config validation in `internal/config/config.go` for pgvector vector store, vector dimension, and any index/search parameters.
- Wire `SEMANTIC_CACHE_VECTOR_STORE=pgvector` in `internal/app/app.go` beside existing `lancedb` and `qdrant` branches.
- Add or update PostgreSQL migrations for `pgvector` extension, vector column types, indexes, and compatibility with existing semantic cache records.
- Implement PostgreSQL parity in `internal/controlstate/postgres/repository.go` and tests beside existing SQLite/PostgreSQL test patterns.
- Add migration tooling/runbook without storing secrets in source-controlled files.
- Add smoke verification that can run only when an external/local PostgreSQL DSN and provider test configuration are supplied.

</code_context>

<specifics>
## Specific Ideas

- Keep Plan 4 operator setup explicit: separate compose override, example env/config, runbook, migration, and smoke test.
- Use explicit configuration plus defaults for pgvector dimension and index/search parameters; defaults should be safe starting points, not hidden magic.
- Prefer noisy failures for relational control-state problems and graceful degradation only for pgvector/semantic-cache capability.
- Capability reporting should describe what is verified now, not the desired end state.

</specifics>

<deferred>
## Deferred Ideas

- BFF/Admin Console UI for viewing PostgreSQL deployment health remains Phase 11 or later.
- Replacing SQLite as the default deployment is out of scope; Plans 1/2 remain SQLite + Redis Stack + Qdrant.
- Replacing Qdrant for Plans 1/2 is out of scope; pgvector is Plan 4 only.
- Online dual-write migration and live cutover are out of scope for Phase 13.

</deferred>

---

*Phase: 13-PostgreSQL Compatibility*
*Context gathered: 2026-07-03*
