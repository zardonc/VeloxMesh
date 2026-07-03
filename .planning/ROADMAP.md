# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-03
**Current focus:** v7.3 PostgreSQL Compatibility

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-10 established the runnable Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, and observability.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path. Qdrant owns vector storage and replication there. Phase 13 now adds the Plan 4 PostgreSQL + pgvector extension path after Phase 12 finalized the multi-node write and recovery boundaries.

## Milestones

- ✅ **v7.0 Plan 1 Foundation** — Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- ✅ **v7.1 Advanced Routing & Observability** — Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- ✅ **v7.2 Multi-Node Coordination** — Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- ◆ **v7.3 PostgreSQL Compatibility** — Phase 13 (active)
- ○ **Future milestones** — BFF/Admin Console
- ✅ **v5** — Phases 5-6 (shipped 2026-06-29)
- ✅ **v4** — Phases 1-4 (shipped 2026-06-23)

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Active in v7.3 |

<details>
<summary>✅ v7.2 Multi-Node Coordination (Phase 12) — SHIPPED 2026-07-03</summary>

- [x] Phase 12: Multi-Node Coordination (5/5 plans) — completed 2026-07-03

</details>

<details open>
<summary>◆ v7.3 PostgreSQL Compatibility (Phase 13) — ACTIVE</summary>

**Goal:** Add PostgreSQL-compatible Plan 4 deployment without changing the OpenAI-compatible data-plane contract.

| Phase | Name | Requirements | Success Criteria |
| --- | --- | --- | --- |
| 13 | PostgreSQL Compatibility | PG-01, PG-02, PG-03, CTRL-01, CTRL-02, CTRL-03, VECT-01, VECT-02, MIGR-01, MIGR-02, TEST-01 | Operators can deploy Redis Stack + PostgreSQL + pgvector, migrate supported SQLite state, and serve chat traffic through the gateway using PostgreSQL-compatible storage paths. |

**Candidate plan slices:**

- **13-01 Deployment and config**: Add PostgreSQL + pgvector deployment docs/templates, configuration examples, readiness behavior, and secret-safe operator guidance.
- **13-02 Repository parity**: Close PostgreSQL gaps for active control-state capabilities and update capability reporting to match implementation.
- **13-03 pgvector semantic path**: Add pgvector vector adapter behavior behind the existing storage boundary while preserving scope/privacy rules.
- **13-04 Migration and verification**: Add SQLite to PostgreSQL migration runbook/tooling and Plan 4 smoke/integration verification.

</details>

## v7.3 PostgreSQL Compatibility

**Goal:** Add PostgreSQL-compatible Plan 4 deployment without changing the OpenAI-compatible data-plane contract.
**Requirements:** PG-01, PG-02, PG-03, CTRL-01, CTRL-02, CTRL-03, VECT-01, VECT-02, MIGR-01, MIGR-02, TEST-01

### Phases

- [ ] Phase 13: PostgreSQL Compatibility

### Phase 13: PostgreSQL Compatibility

**Goal:** Add PostgreSQL-compatible Plan 4 deployment with Redis Stack, PostgreSQL, pgvector, migration guidance, repository parity, and smoke verification. Plans 1/2 stay SQLite + Redis Stack + Qdrant by default.
**Priority:** P3
**Depends on:** Phase 12
**Requirements:** PG-01, PG-02, PG-03, CTRL-01, CTRL-02, CTRL-03, VECT-01, VECT-02, MIGR-01, MIGR-02, TEST-01
**Plans:** 4 plans

Success criteria:

1. Operators can start a local Plan 4 stack with Redis Stack, PostgreSQL, and pgvector using documented configuration without source-controlled secrets.
2. PostgreSQL control-state repositories support the active gateway capabilities required for provider CRUD, API keys, routing, usage settlement, semantic cache metadata, and fallback logging.
3. pgvector is available behind the existing vector adapter boundary for Plan 4 semantic-cache search while preserving tenant/API-key scoping and prompt privacy.
4. Supported SQLite control-state data can be migrated or replayed into PostgreSQL through a repeatable runbook or command.
5. Smoke verification proves OpenAI-compatible chat traffic works against the PostgreSQL-compatible Plan 4 deployment.

Key deliverables:

- PostgreSQL + pgvector deployment docs/templates and secret-safe configuration examples.
- Startup readiness and failure behavior for required Plan 4 dependencies.
- PostgreSQL capability profile updates that match implemented repository behavior.
- pgvector vector adapter or equivalent Plan 4 semantic-cache path behind `storage.VectorAdapter`.
- SQLite to PostgreSQL migration runbook/tooling for supported control-state records.
- Focused integration/smoke checks gated by externally supplied PostgreSQL test DSNs.

Plans:
**Wave 1**

- [ ] 13-01-PLAN.md - Deployment, configuration, readiness, and operator runbook.

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 13-02-PLAN.md - PostgreSQL repository parity and capability reporting.

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 13-03-PLAN.md - pgvector semantic-cache/vector adapter path.

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 13-04-PLAN.md - SQLite-to-PostgreSQL migration and Plan 4 smoke verification.

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** — JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.

## Gateway Runtime Modes

VeloxMesh supports progressive deployment tiers:

- **Plan 1 (Standalone Enhanced)**: SQLite + Redis Stack + Qdrant.
- **Plan 2 (Multi-Node)**: Multi App + Redis Stack + SQLite + Qdrant.
- **Plan 3 (Edge)**: SQLite + LanceDB behind `-tags lancedb`, Linux/macOS only.
- **Plan 4 (Extension)**: Redis Stack + PostgreSQL + pgvector.

## Notes

- Phase 12 intentionally skips BFF/Admin Console UI; any topology display belongs after Phase 11 exists.
- Phase 13 is active because Phase 12 has defined and verified the multi-node write and recovery boundaries.
- All storage access goes through adapter interfaces; switching backends requires adapter implementation swaps.
- Redis is hot coordination and transport, not relational source of truth.
- Source code committed to git must not contain hardcoded configuration. Configuration must come from local environment variables, configuration files, or the database.

---
*Roadmap refreshed: 2026-07-03 after starting v7.3 PostgreSQL Compatibility*
