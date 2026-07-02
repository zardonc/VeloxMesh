# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-02
**Current focus:** v7.2 Multi-Node Coordination

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-10 established the runnable Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, and observability.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path. Qdrant owns vector storage and replication. Phase 12 adds Plan 2 multi-node coordination for SQLite relational state and node roles; PostgreSQL + pgvector remains a later Phase 13 extension.

## Milestones

- ✅ **v7.0 Plan 1 Foundation** — Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- ✅ **v7.1 Advanced Routing & Observability** — Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- 🚧 **v7.2 Multi-Node Coordination** — Phase 12 (active)
- ○ **Future milestones** — BFF/Admin Console, PostgreSQL extension
- ✅ **v5** — Phases 5-6 (shipped 2026-06-29)
- ✅ **v4** — Phases 1-4 (shipped 2026-06-23)

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Active in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Future |

## v7.2 Multi-Node Coordination

**Goal:** Enable Plan 2 multi-node deployment with Redis coordination, SQLite relational replication, write fencing, recovery, and chaos verification.
**Requirements:** COORD-01, COORD-02, COORD-03, REPL-01, REPL-02, REPL-03, FENCE-01, FENCE-02, RECOV-01, RECOV-02, HLTH-01, TEST-01

### Phases

- [ ] Phase 12: Multi-Node Coordination

### Phase 12: Multi-Node Coordination

**Goal:** Enable Plan 2 multi-node deployment with leader election, SQLite-only WAL replication, SQLite write fencing, health reporting, recovery, and chaos testing. Vector sync stays out of scope because Qdrant owns vector storage and replication.
**Priority:** P1
**Depends on:** Phase 9
**Requirements:** COORD-01, COORD-02, COORD-03, REPL-01, REPL-02, REPL-03, FENCE-01, FENCE-02, RECOV-01, RECOV-02, HLTH-01, TEST-01
**Plans:** 4 plans

Success criteria:
1. Operators can start multiple gateway nodes and inspect each node's identity, role, leader identity, and WAL lag.
2. Exactly one node owns SQLite relational writes while followers reject or forward write attempts with clear retryable errors.
3. SQLite relational changes replicate from leader to replicas through Redis Streams without attempting vector replication.
4. Failed sync operations enter a fallback log and can be replayed by a recovery worker.
5. Chaos tests cover leader loss, graceful shutdown, and Redis/network disruption without corrupting SQLite state.

Key deliverables:
- Redis coordination adapter for registration, leadership lock, heartbeat, pub/sub, and stream operations.
- Leader election using Redis `SET NX` with a 10s TTL and 3s heartbeat.
- Redis Stream consumer group for leader-to-replica SQLite relational sync only.
- SQLite write fencing in multi-node mode.
- Health output with role, node identity, leader identity, and WAL lag.
- Fallback log integration and recovery worker.
- Graceful shutdown that releases leadership when possible.
- Chaos tests for leader loss, node shutdown, and Redis/network disruption.

Plans:
- [ ] 12-01-PLAN.md - Coordination runtime, Redis leadership, heartbeat, and graceful release.
- [ ] 12-02-PLAN.md - SQLite write fencing and relational stream producer.
- [ ] 12-03-PLAN.md - Replica stream consumer, recovery worker, readiness, and internal topology.
- [ ] 12-04-PLAN.md - In-process multi-node harness and required abnormal scenario coverage.

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** — JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.
- **Phase 13: PostgreSQL Extension** — PostgreSQL + pgvector adapter for enterprise deployments requiring multi-node concurrent writes and vector+relational JOIN queries. Depends on Phase 12.

## Gateway Runtime Modes

VeloxMesh supports progressive deployment tiers:
- **Plan 1 (Standalone Enhanced)**: SQLite + Redis Stack + Qdrant.
- **Plan 2 (Multi-Node)**: Multi App + Redis Stack + SQLite + Qdrant.
- **Plan 3 (Edge)**: SQLite + LanceDB behind `-tags lancedb`, Linux/macOS only.
- **Plan 4 (Extension)**: Redis Stack + PostgreSQL + pgvector.

## Notes

- Phase 12 intentionally skips BFF/Admin Console UI; any topology display belongs after Phase 11 exists.
- Phase 13 remains deferred until Phase 12 defines and verifies the multi-node write and recovery boundaries.
- All storage access goes through adapter interfaces; switching backends requires adapter implementation swaps.
- Redis is hot coordination and transport, not relational source of truth.
- Source code committed to git must not contain hardcoded configuration. Configuration must come from local environment variables, configuration files, or the database.

---
*Roadmap refreshed: 2026-07-02 after starting v7.2 Multi-Node Coordination*
