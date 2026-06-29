# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Mode:** brownfield retrospective initialization
**Current focus:** Architecture refactor — SQLite + LanceDB + Redis Stack

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-4 established the runnable Go/Chi OpenAI-compatible data-plane skeleton with provider adapters, durable control state, streaming, rate limits, caching, and cost governance. Phase 5 added tool/function calling and multimodal capabilities.

The architecture has been redesigned (v2.0) to use **SQLite + LanceDB + optional Redis Stack**, replacing the previous PostgreSQL + pgvector design. The new architecture supports a 4-tier deployment progression: single-node lite → single-node enhanced → multi-node → PostgreSQL extension.

## Milestones

- 🚧 **v7** — Phases 7-12: Architecture Refactor & New Capabilities (planning)
- 🚧 **v5** — Phases 5-6 (in progress)
- ✅ **v4** — Phases 1-4 (shipped 2026-06-23)

## Deployment Tiers

The gateway supports progressive deployment tiers, each adding capability without redesign:

| Tier | Components | Priority | Status |
|---|---|---|---|
| **Plan 1**: Standalone Lite | App + SQLite + LanceDB (optional) | P0 | Planning |
| **Plan 2**: Standalone Enhanced | App + Redis Stack + SQLite + LanceDB | P1 | Planning |
| **Plan 3**: Multi-Node | Multi App + Redis Stack + SQLite + LanceDB | P2 | Planning |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Future |

## Phases

<details open>
<summary>🚧 v7 (Phases 7-12) — PLANNING</summary>

- [ ] Phase 7: Adapter Interfaces & SQLite Foundation (Plan 1 core)
- [ ] Phase 8: BFF Layer & Admin Console (JWT + Role-based access)
- [ ] Phase 9: Semantic Pipeline (RTK/Headroom/PII/Caveman/Ponytail)
- [ ] Phase 10: Redis Stack Integration (Plan 2)
- [ ] Phase 11: Multi-Node Coordination (Plan 3)
- [ ] Phase 12: PostgreSQL Extension (Plan 4, low priority)

### Phase 7: Adapter Interfaces & SQLite Foundation

**Goal:** Replace PostgreSQL with SQLite as the primary data store. Define and implement all adapter interfaces (CacheAdapter, CoordAdapter, DBAdapter, VectorAdapter) with SQLite and in-memory implementations. Migrate existing schema to SQLite with WAL mode.
**Priority:** P0
**Depends on:** Phase 6

Key deliverables:
- SQLite WAL mode initialization and schema migration
- CacheAdapter interface + MemoryCacheAdapter implementation
- CoordAdapter interface + NoopCoordAdapter implementation
- DBAdapter interface + SQLiteDBAdapter implementation
- VectorAdapter interface + LanceDBVectorAdapter implementation
- Data Access Layer (DAL) with repository pattern
- Fallback log table for disaster recovery
- Config hot-reload via in-memory TTL cache

### Phase 8: BFF Layer & Admin Console

**Goal:** Implement the BFF layer with JWT authentication, role-based access control (SUPER_ADMIN/ADMIN/USER), session management, and the Admin Console foundation.
**Priority:** P0
**Depends on:** Phase 7

Key deliverables:
- JWT-based authentication (login, logout, forced logout)
- Role-based permission system (users table, role field)
- BFF session verification and route permission checking
- Dynamic route table per role
- X-Verified-User-ID / X-Verified-Role header injection
- Admin Console React SPA foundation
- Revoked tokens blacklist (SQLite-based for Plan 1)

### Phase 9: Semantic Pipeline

**Goal:** Implement the configurable input/output processing pipeline with handler registry, per-rule toggles, and hot-reloadable configuration.
**Priority:** P1
**Depends on:** Phase 7

Key deliverables:
- Handler interface and pipeline executor
- Input handlers: RTK (token compression), Headroom, PII Redaction, Input Rewrite
- Output handlers: Caveman, Ponytail, PII Restore, Output Filter
- YAML configuration with per-rule enabled toggle
- Pipeline rule registration and hot-reload

### Phase 10: Redis Stack Integration

**Goal:** Integrate Redis Stack for hot caching, vector similarity search (VSS), atomic rate limiting, Pub/Sub config reload, and token cost aggregation. All features must gracefully degrade when Redis is unavailable.
**Priority:** P1
**Depends on:** Phase 7

Key deliverables:
- RedisCacheAdapter implementation
- Redis VSS hot cache for vector data (hot-cold tiering)
- Atomic rate limiting via Redis INCR (replacing memory counters)
- Config Pub/Sub hot-reload
- Token cost aggregation buffer (Redis HINCR → batch SQLite flush)
- Session blacklist via Redis SET
- API key hot cache with 5min TTL

### Phase 11: Multi-Node Coordination

**Goal:** Enable multi-node deployment with leader election, WAL-based replication, fencing, and disaster recovery.
**Priority:** P2
**Depends on:** Phase 10

Key deliverables:
- RedisCoordAdapter implementation
- Leader election (Redis SET NX + TTL 10s + heartbeat 3s)
- WAL Stream (Redis Stream Consumer Group) for master→replica sync
- Fencing mechanism (check lock holder before writes)
- Node registration and health endpoint (/health with role, wal_lag)
- BFF cluster topology awareness (read/write routing)
- Fallback log + Recovery Worker
- Graceful shutdown with leader lock release
- Chaos testing (random node kill, network partition)

### Phase 12: PostgreSQL Extension (Low Priority)

**Goal:** Implement PostgreSQL + pgvector adapter for enterprise deployments requiring multi-node concurrent writes and vector+relational JOIN queries.
**Priority:** P3
**Depends on:** Phase 11

Key deliverables:
- PostgresDBAdapter implementation
- PgvectorAdapter implementation
- SQLite → PostgreSQL data migration tool
- Performance comparison benchmarks

</details>

<details open>
<summary>🚧 v5 (Phases 5-6) — IN PROGRESS</summary>

- [x] Phase 5: Tool/Function Calling and Multimodal capabilities
- [ ] Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing)

### Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing)

**Goal:** Add user-defined combo models that can route through multiple provider models using round-robin, fusion, and capability-aware filtering.
**Requirements**: Phase 6 Model Combo Feature
**Depends on:** Phase 5
**Plans:** 1 plan

Plans:
- [ ] 06-01 Persistent Combo Models and Routing

</details>

<details>
<summary>✅ v4 (Phases 1-4) — SHIPPED 2026-06-23</summary>

- [x] Phase 1: Gateway Walking Skeleton (1/1 complete)
- [x] Phase 2.1: Health-Aware Multi-Provider Routing (1/1 complete)
- [x] Phase 2.2: Go Version Baseline for Official Provider SDKs (1/1 complete)
- [x] Phase 2.3: Native Anthropic and Gemini Provider Adapters (1/1 complete)
- [x] Phase 2.4: Provider Reliability and Error Contract (1/1 complete)
- [x] Phase 2.5: Provider Retry and Fallback Execution (1/1 complete)
- [x] Phase 2.6: Active Provider Health Probing and Recovery (1/1 complete)
- [x] Phase 2.7: Provider Adapter Capability Contract (2/2 complete)
- [x] Phase 2.8: Provider Configuration Schema and Secret-Safe Validation (1/1 complete)
- [x] Phase 2.9: Provider Model Catalog and Routing Eligibility (1/1 complete)
- [x] Phase 2.10: Adapter Conformance Test Harness (1/1 complete)
- [x] Phase 3: Durable Control State (7/7 complete)
- [x] Phase 4: Streaming, Rate Limits, Cache, and Cost (12/12 complete)

</details>

## Gateway Runtime Modes

VeloxMesh supports progressive deployment tiers:
- **Plan 1 (Lite)**: SQLite + LanceDB (optional) — no external dependencies. Suitable for personal, edge, and low-concurrency deployments.
- **Plan 2 (Enhanced)**: SQLite + LanceDB + Redis Stack — adds hot caching, vector VSS, atomic rate limiting, Pub/Sub config reload. Single-node production.
- **Plan 3 (Multi-Node)**: Same as Plan 2 with leader election, WAL replication, and fencing. High availability.
- **Plan 4 (Extension)**: PostgreSQL + pgvector replaces SQLite + LanceDB. Enterprise scale with concurrent writes and vector+relational JOINs.

## Notes

- Phase 4 is complete.
- Architecture v2.0 replaces PostgreSQL + pgvector with SQLite + LanceDB + optional Redis Stack.
- All storage access goes through adapter interfaces; switching backends requires only adapter implementation swap.
- Native provider SDK details stay inside adapter packages; handlers and routing consume provider-neutral contracts.
- **Rule**: Source code committed to git must not contain any hardcoded configuration information. Configuration must only be obtained from local environment variables, configuration files, or the database.

## Local Development Resources

The local development environment has been verified and configured. The following resources are available and their specific connection details, models, and credentials can be found in the local `.env` and `.env.local` files:

- **Infrastructure**:
  - SQLite (embedded, data directory)
  - Redis Stack (optional, for Plan 2+ features)
  - LanceDB (embedded, vector store)
- **LLM Providers**:
  - `sans` (SANS Primary, with multiple models configured)

---
*Roadmap refreshed: 2026-06-29 after architecture v2.0 refactor (SQLite + LanceDB + Redis Stack)*
