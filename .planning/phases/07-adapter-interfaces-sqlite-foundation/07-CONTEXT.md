# Phase 7: Adapter Interfaces & SQLite Foundation - Context

**Gathered:** 2026-06-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the Plan 1 foundation for architecture v2.1: SQLite authoritative relational state, Redis Stack hot cache/rate/config coordination, Qdrant primary vector and semantic-cache storage, and adapter seams that keep LanceDB and PostgreSQL as later extension paths.

</domain>

<decisions>
## Implementation Decisions

### Storage Baseline
- **D-01:** SQLite is the primary durable control-state store for Phase 7 and Plan 1.
- **D-02:** PostgreSQL code stays only as retained extension code; do not expand it in Phase 7.
- **D-03:** SQLite must be initialized with architecture v2.1 pragmas: foreign keys, WAL mode, busy timeout, and normal synchronous mode.
- **D-04:** Static/env provider config remains only for `ControlStateBackend=disabled` and explicit local seed compatibility.

### Adapter Scope
- **D-05:** Add only adapter seams needed by Phase 7: cache, coordination, database, and vector boundaries.
- **D-06:** Plan 1 implementations are Memory/Redis cache where already available, no-op coordination for single-node, SQLite database, Qdrant vector adapter, Noop/Degraded vector behavior, and Qdrant semantic cache.
- **D-07:** Preserve and reuse existing Redis hot-state behavior; do not make Redis VSS the default vector path. Redis VSS is only a Qdrant fallback path and may be deferred to Phase 10 if it expands scope.
- **D-08:** Do not implement PostgreSQL/pgvector adapter work in Phase 7; Phase 12 owns it.

### Qdrant / Vector Store
- **D-09:** Qdrant replaces LanceDB as the Plan 1/2 primary vector store and semantic-cache backend.
- **D-10:** Existing SQLite semantic cache repository/service shape can be reused as a compatibility/fallback building block, but the target Plan 1 semantic cache is Qdrant Collection based.
- **D-13:** LanceDB is cancelled from Phase 7 mainline work. Keep only a future Plan 3 note: build-tag isolated, CGO, Linux/macOS only.
- **D-14:** Qdrant failures must degrade only vector/RAG/semantic-cache capability. Core LLM proxying, auth, routing, and provider fallback must continue.
- **D-15:** Vector write failures should be persisted to SQLite `fallback_log` with `type='VECTOR'` for later recovery.
- **D-16:** Multi-node WAL sync must not include vector data. Qdrant owns vector persistence and replication.

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
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` — current architecture v2.1 source of truth.
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-refactor-design.md` — Chinese refactor design, startup sequence, adapter contracts, and deployment tiers.
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
- `internal/storage` — current vector adapter interface and LanceDB implementation/stubs that must be replaced or isolated.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- SQLite repository already implements provider, combo, routing, API key, rate, usage, audit, idempotency, and semantic cache storage.
- `sqlite.Open` now configures core SQLite pragmas.
- `hotstate.Client` already provides local and Redis implementations for health/auth/config-change hot state.
- `RuntimeProviderManager` already centralizes provider snapshot activation and reload.
- The current `VectorAdapter` seam is useful, but its method shape and package naming need adjustment for Qdrant entries, collection names, health checks, and degraded behavior.
- The current semantic cache service already has an opt-in lookup/store lifecycle and can be reused around a Qdrant-backed adapter.
- The `fallback_log` idea remains valuable and should explicitly cover Qdrant/vector replay.

### Conflict Notes
- `.planning/REQUIREMENTS.md` still lists an older Phase 7 meaning (observability). Treat `.planning/ROADMAP.md` and architecture v2.1 docs as authoritative for this phase.
- Current `ControlStateBackend` default is still `disabled`. Phase 7 should decide whether to switch the default or document a local bootstrap path that avoids breaking legacy tests.
- Existing PostgreSQL repository code is retained and tested, but it is not the active architecture path.
- Current code has `github.com/lancedb/lancedb-go` in `go.mod` and `internal/storage/lancedb_*` implementations. This conflicts with v2.1 mainline because LanceDB is Plan 3/P3 edge-only and should not block standard Windows/pure-Go development.
- `internal/app/app.go` still selects `SemanticCacheVectorStore == "lancedb"` and treats LanceDB initialization failure as startup failure. Under v2.1, Qdrant initialization should degrade vector capability without blocking core startup.
- README, `.env.example`, `docker-compose.yml`, and Phase 6 docs may still describe SQLite + LanceDB + optional Redis Stack. These are stale and should be corrected before further implementation.
- Redis VSS wording must be changed from default hot/cold vector tiering to optional Qdrant fallback.

### Integration Points
- Config loading and app startup are the smallest places to expose SQLite-first runtime behavior.
- Adapter contracts should sit near existing storage/hot-state packages, not in provider routing code.
- Tests should cover startup with SQLite durable state, Qdrant unavailable/degraded behavior, migration behavior, fallback log schema, and Redis-enabled Plan 1 behavior.

## System-Wide Conflict / Reuse Matrix

| Area | Current State | v2.1 Decision | Action |
|---|---|---|---|
| LanceDB dependency | `go.mod` and `internal/storage/lancedb_*` exist | Edge-only Plan 3, build-tag isolated | Remove from Phase 7 mainline; defer or isolate behind `-tags lancedb` |
| Vector interface | Generic `VectorAdapter` exists | Keep seam, adapt to Qdrant entry/search/delete/ping semantics | Reuse concept, adjust contract |
| Semantic cache | SQLite-backed service/repository exists | Qdrant Collection is target backend | Reuse lifecycle and opt-in gating; move vector search to Qdrant |
| Redis hot state | Existing local/Redis hot-state abstraction | Redis Stack is Plan 1/2 hot cache/rate/config component | Reuse; avoid duplicating cache/coord seams |
| Redis VSS | Planned as hot vector layer | Only fallback when Qdrant degraded/slow | Defer default VSS work; document fallback policy |
| SQLite repository | Mature durable control-state implementation | Authoritative relational store | Keep and extend with fallback vector records |
| PostgreSQL repository | Existing extension code | Plan 4/P3 | Retain, do not expand |
| Multi-node WAL | Prior plans implied broader sync | SQLite relational data only | Ensure Phase 11 plan excludes vectors |

</code_context>

<specifics>
## Specific Ideas

- Keep Phase 7 as a foundation slice: adapter contracts, SQLite defaults, Qdrant vector/semantic-cache integration path, degraded/noop vector behavior, fallback log, and documentation.
- Leave BFF/Admin UI, Semantic Pipeline, Redis VSS fallback hardening, multi-node, LanceDB edge build, and PostgreSQL adapter work to later phases unless a small stub is needed to prevent mainline build breakage.

</specifics>

<deferred>
## Deferred Ideas

- Full Admin Console and JWT session flows — Phase 8.
- Semantic Pipeline handlers — Phase 9.
- Redis Stack fallback hardening and optional Redis VSS — Phase 10.
- Multi-node leader election and SQLite-only WAL replication — Phase 11.
- LanceDB edge build-tag implementation — future Plan 3/P3 work.
- PostgreSQL + pgvector adapters and migration tool — Phase 12.

</deferred>

---

*Phase: 7-Adapter Interfaces & SQLite Foundation*
*Context gathered: 2026-06-29*
