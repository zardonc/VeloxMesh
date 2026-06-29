# Phase 7: Adapter Interfaces & SQLite Foundation - Context

**Gathered:** 2026-06-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the Plan 1 foundation for architecture v2.0: SQLite-first durable state, simple adapter seams for cache/coordination/database/vector storage, and a single-node runtime path that does not require PostgreSQL or Redis.

</domain>

<decisions>
## Implementation Decisions

### Storage Baseline
- **D-01:** SQLite is the primary durable control-state store for Phase 7 and Plan 1.
- **D-02:** PostgreSQL code stays only as retained extension code; do not expand it in Phase 7.
- **D-03:** SQLite must be initialized with architecture v2.0 pragmas: foreign keys, WAL mode, busy timeout, and normal synchronous mode.
- **D-04:** Static/env provider config remains only for `ControlStateBackend=disabled` and explicit local seed compatibility.

### Adapter Scope
- **D-05:** Add only adapter seams needed by Phase 7: cache, coordination, database, and vector boundaries.
- **D-06:** Plan 1 implementations are in-memory cache, no-op coordination, SQLite database, and optional vector adapter placeholder.
- **D-07:** Do not add Redis behavior in Phase 7 beyond preserving existing optional hot-state code; Redis Stack belongs to Phase 10.
- **D-08:** Do not implement PostgreSQL/pgvector adapter work in Phase 7; Phase 12 owns it.

### LanceDB / Vector Store
- **D-09:** LanceDB is optional in Plan 1, but Phase 7 should include the first usable LanceDB-backed semantic cache path behind `VectorAdapter` when the dependency is locally viable.
- **D-10:** Existing SQLite semantic cache behavior must keep working without LanceDB; LanceDB is enabled only by explicit vector-store configuration and degrades to the current SQLite behavior when disabled.
- **D-13:** Redis VSS hot cache is not part of Phase 7. Phase 10 adds the Redis hot layer on top of the Phase 7 LanceDB warm/cold layer.

### Runtime Defaults
- **D-11:** Developer-facing defaults and docs should make SQLite the normal durable path, but tests and legacy local flows may keep `ControlStateBackend=disabled` where needed.
- **D-12:** App startup should make missing SQLite durable config actionable instead of silently falling back to PostgreSQL-era assumptions.

### the agent's Discretion
- Prefer small interfaces and existing repository patterns over a broad DAL rewrite.
- If a proposed adapter has only one current caller and no immediate Phase 7 behavior, write the minimum interface that Phase 8-10 can consume later.
- Keep migrations and config changes boring and testable; do not introduce a new migration framework.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Architecture
- `Agent-gateway/gateway-architecture.md` — current architecture v2.0 source of truth.
- `Agent-gateway/gateway-refactor-design.md` — Chinese refactor design, startup sequence, adapter contracts, and deployment tiers.
- `.planning/ROADMAP.md` — Phase 7 goal and dependency chain.
- `.planning/PROJECT.md` — project-level source of truth and current architecture notes.
- `.planning/phases/06-model-combo-feature-rr-fusion-capability-based-routing/06-CONTEXT.md` — system-wide architecture conflict audit and Phase 6 carry-forward constraints.

### Existing Code
- `internal/app/app.go` — app startup, control-state backend selection, Redis/local hot-state selection, provider reload.
- `internal/config/config.go` — current config shape and defaults.
- `internal/controlstate/repository.go` — existing durable repository interface.
- `internal/controlstate/sqlite/repository.go` — SQLite repository and connection pragmas.
- `internal/controlstate/sqlite/migrations.go` and `internal/controlstate/migrations/sqlite` — embedded SQLite migration pattern.
- `internal/controlstate/capabilities.go` — current capability profile model.
- `internal/hotstate/hotstate.go`, `internal/hotstate/local.go`, `internal/hotstate/redis.go` — existing hot-state abstraction to reuse where possible.
- `internal/cache` — current semantic cache service and repository usage.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- SQLite repository already implements provider, combo, routing, API key, rate, usage, audit, idempotency, and semantic cache storage.
- `sqlite.Open` now configures core SQLite pragmas.
- `hotstate.Client` already provides local and Redis implementations for health/auth/config-change hot state.
- `RuntimeProviderManager` already centralizes provider snapshot activation and reload.

### Conflict Notes
- `.planning/REQUIREMENTS.md` still lists an older Phase 7 meaning (observability). Treat `.planning/ROADMAP.md` and architecture v2.0 docs as authoritative for this phase.
- Current `ControlStateBackend` default is still `disabled`. Phase 7 should decide whether to switch the default or document a local bootstrap path that avoids breaking legacy tests.
- Existing PostgreSQL repository code is retained and tested, but it is not the active architecture path.

### Integration Points
- Config loading and app startup are the smallest places to expose SQLite-first runtime behavior.
- Adapter contracts should sit near existing storage/hot-state packages, not in provider routing code.
- Tests should cover startup with SQLite durable state, migration behavior, fallback log schema, and no-Redis local operation.

</code_context>

<specifics>
## Specific Ideas

- Keep Phase 7 as a foundation slice: adapter contracts, SQLite defaults, fallback log, optional LanceDB semantic cache, and documentation.
- Leave BFF/Admin UI, Semantic Pipeline, Redis VSS, multi-node, and PostgreSQL adapter work to Phases 8-12.

</specifics>

<deferred>
## Deferred Ideas

- Full Admin Console and JWT session flows — Phase 8.
- Semantic Pipeline handlers — Phase 9.
- Redis Stack adapters and VSS hot cache — Phase 10.
- Multi-node leader election and WAL replication — Phase 11.
- PostgreSQL + pgvector adapters and migration tool — Phase 12.

</deferred>

---

*Phase: 7-Adapter Interfaces & SQLite Foundation*
*Context gathered: 2026-06-29*
