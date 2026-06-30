# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Mode:** brownfield retrospective initialization
**Current focus:** Next milestone planning — advanced routing, admin UX, and multi-node capabilities

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-4 established the runnable Go/Chi OpenAI-compatible data-plane skeleton with provider adapters, durable control state, streaming, rate limits, caching, and cost governance. Phase 5 added tool/function calling and multimodal capabilities.

The architecture has been redesigned (v2.1) to use **SQLite + Redis Stack + Qdrant** for the main Plans 1/2 path. Qdrant replaces LanceDB as the primary vector store and semantic-cache backend. LanceDB is retained only as a Plan 3 edge-only option behind a build tag, while PostgreSQL + pgvector remains a Plan 4 extension.

## Milestones

- ✅ **v7.0 Plan 1 Foundation** — Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- 🚧 **Next milestone** — Phases 10-13: Advanced routing, observability, BFF/Admin Console, multi-node coordination, PostgreSQL extension (planned)
- ✅ **v5** — Phases 5-6 (shipped 2026-06-29)
- ✅ **v4** — Phases 1-4 (shipped 2026-06-23)

## Deployment Tiers

The gateway supports progressive deployment tiers, each adding capability without redesign:

| Tier | Components | Priority | Status |
|---|---|---|---|
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Planning |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Future |

## Phases

<details>
<summary>✅ v7.0 Plan 1 Foundation (Phases 7-9) — SHIPPED 2026-06-30</summary>

- [x] Phase 7: Adapter Interfaces & SQLite Foundation (Plan 1 core)
- [x] Phase 8: Semantic Pipeline (RTK/Headroom/PII/Caveman/Ponytail)
- [x] Phase 9: Redis Stack + Qdrant Fallback Integration (Plan 1 hardening)

Archive: `.planning/milestones/v7.0-ROADMAP.md`

</details>

<details open>
<summary>🚧 Next milestone (Phases 10-13) — PLANNED</summary>

- [ ] Phase 10: Advanced Routing & Observability
- [ ] Phase 11: BFF Layer & Admin Console (JWT + Role-based access)
- [ ] Phase 12: Multi-Node Coordination (Plan 2)
- [ ] Phase 13: PostgreSQL Extension (Plan 4, low priority)

### Phase 10: Advanced Routing & Observability

**Goal:** Implement the Composite Score Router for normalized multi-signal scoring and add comprehensive OpenTelemetry/Prometheus observability.
**Priority:** P1
**Depends on:** Phase 9

Key deliverables:
- Composite Score Router (latency, pending requests, error rates, costs, health bonuses)
- Z-score normalization for routing signals
- OpenTelemetry traces (TTFT, TPOT, E2E, cache hit)
- Prometheus metrics histograms

### Phase 11: BFF Layer & Admin Console

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

### Phase 12: Multi-Node Coordination

**Goal:** Enable v2.1 Plan 2 multi-node deployment with leader election, SQLite-only WAL replication, SQLite-write fencing, and disaster recovery. Vector sync is removed because Qdrant owns vector storage and replication.
**Priority:** P2
**Depends on:** Phase 9

Key deliverables:
- RedisCoordAdapter implementation
- Leader election (Redis SET NX + TTL 10s + heartbeat 3s)
- WAL Stream (Redis Stream Consumer Group) for master→replica SQLite relational sync only
- Fencing mechanism for SQLite writes only
- Node registration and health endpoint (/health with role, wal_lag)
- BFF cluster topology awareness (read/write routing)
- Fallback log + Recovery Worker
- Graceful shutdown with leader lock release
- Chaos testing (random node kill, network partition)

### Phase 13: PostgreSQL Extension (Low Priority)

**Goal:** Implement PostgreSQL + pgvector adapter for enterprise deployments requiring multi-node concurrent writes and vector+relational JOIN queries.
**Priority:** P3
**Depends on:** Phase 12

Key deliverables:
- PostgresDBAdapter implementation
- PgvectorAdapter implementation
- SQLite → PostgreSQL data migration tool
- Performance comparison benchmarks

</details>

<details>
<summary>✅ v5 (Phases 5-6) — SHIPPED 2026-06-29</summary>

- [x] Phase 5: Tool/Function Calling and Multimodal capabilities
- [x] Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing)

### Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing)

**Goal:** Add user-defined combo models that can route through multiple provider models using round-robin, fusion, and capability-aware filtering.
**Requirements**: Phase 6 Model Combo Feature
**Depends on:** Phase 5
**Architecture note:** Keep completed combo functionality where it fits, but align persistence/runtime loading with architecture v2.1: SQLite relational state, Redis hot-state where configured, Qdrant for vector/semantic-cache features, and PostgreSQL deferred to Phase 12 adapter extension.
**Plans:** 1 plan

Plans:
- [x] 06-01 Persistent Combo Models and Routing

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
- **Plan 1 (Standalone Enhanced)**: SQLite + Redis Stack + Qdrant. This is the P0 mainline: single-node production with durable relational state, hot cache/rate/config coordination, and Qdrant semantic cache.
- **Plan 2 (Multi-Node)**: Multi App + Redis Stack + SQLite + Qdrant. Redis coordinates cluster state and SQLite WAL replication; Qdrant handles vector storage independently.
- **Plan 3 (Edge)**: SQLite + LanceDB behind `-tags lancedb`, Linux/macOS only, P3. Useful for zero-external-dependency edge deployments, but not the default path.
- **Plan 4 (Extension)**: PostgreSQL + pgvector. Enterprise scale with concurrent writes and vector+relational JOINs.

## Notes

- Phase 4 is complete.
- Architecture v2.1 replaces the v2.0 LanceDB mainline with Qdrant for Plans 1/2. LanceDB remains only for Plan 3 edge builds.
- Phase 7-9 shipped as v7.0 Plan 1 Foundation on 2026-06-30.
- Phase 10-13 remain planned for the next milestone.
- All storage access goes through adapter interfaces; switching backends requires only adapter implementation swap.
- Native provider SDK details stay inside adapter packages; handlers and routing consume provider-neutral contracts.
- **Rule**: Source code committed to git must not contain any hardcoded configuration information. Configuration must only be obtained from local environment variables, configuration files, or the database.

## Local Development Resources

The local development environment has been verified and configured. The following resources are available and their specific connection details, models, and credentials can be found in the local `.env` and `.env.local` files:

- **Infrastructure**:
  - SQLite (embedded, data directory)
  - Redis Stack (Plan 1/2 hot cache, rate limiting, Pub/Sub, and coordination)
  - Qdrant (Plan 1/2 vector store and semantic cache)
  - LanceDB (Plan 3 edge-only, build-tag isolated)
- **LLM Providers**:
  - `sans` (SANS Primary, with multiple models configured)

---
*Roadmap refreshed: 2026-06-30 after v7.0 Plan 1 Foundation close*
