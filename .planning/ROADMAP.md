# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-04
**Current focus:** Planning next milestone

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-10 established the runnable Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, and observability.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.4 shipped the optional stateless Scheduler path while the gateway keeps ownership of intake, queue storage, execution, and fallback behavior.

## Milestones

- [x] **v7.4 Gateway Scheduler** - Phases 14-16 (shipped 2026-07-04; archive: `.planning/milestones/v7.4-ROADMAP.md`)
- [x] **v7.3 PostgreSQL Compatibility** - Phase 13 (shipped 2026-07-03)
- [x] **v7.2 Multi-Node Coordination** - Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- [x] **v7.1 Advanced Routing & Observability** - Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- [x] **v7.0 Plan 1 Foundation** - Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- [x] **v5** - Phases 5-6 (shipped 2026-06-29)
- [x] **v4** - Phases 1-4 (shipped 2026-06-23)
- [ ] **Future milestones** - BFF/Admin Console, scheduler enhancements, or another gateway priority

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Shipped in v7.3 |

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** - JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.
- **Scheduler enhancements** - optional Qdrant semantic-neighbor features, anomaly detection, and SLA waiting-time promotion after the core Scheduler path is stable.

## Notes

- Scheduler is optional and disabled by default.
- Gateway remains the source of truth for queue ownership, task state, execution, and fallback behavior.
- Scheduler must not receive raw prompts, provider secrets, API keys, or authorization headers.
- Static virtual deadline scoring is preferred over dynamic score updates to avoid Redis write amplification.
- Source code committed to git must not contain hardcoded configuration.

---
*Roadmap refreshed: 2026-07-04 after archiving v7.4 Gateway Scheduler*
