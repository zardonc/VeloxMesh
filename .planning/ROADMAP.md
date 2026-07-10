# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-08
**Current focus:** Phase 26 - Scheduler Scoring Backpressure Hardening

## Overview

VeloxMesh is being built as vertical gateway slices. The current gateway includes the Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, observability, multi-node coordination, PostgreSQL compatibility, and the optional Gateway Scheduler path.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.7 shipped Scheduler queue default hardening, Redis node-scoped queueing, FallbackQueue recovery reads, and Plan 3 single-node LanceDB/Qdrant vector compatibility. Phase 26 hardens synchronous Scheduler scoring against external predictor backpressure.

## Milestones

- [x] **v7.8 Scheduler Scoring Backpressure Hardening** - Phase 26 (planned) (completed 2026-07-08)
- [x] **v7.7 Scheduler Hardening + Plan 3 Vector Compatibility** - Phases 23-25 (shipped 2026-07-08; archive: `.planning/milestones/v7.7-ROADMAP.md`)
- [x] **v7.6 Scheduler 1.0 + Config** - Phases 20-22 (shipped 2026-07-06; archive: `.planning/milestones/v7.6-ROADMAP.md`)
- [x] **v7.5 Scheduler Enhancements** - Phases 17-19 (shipped 2026-07-05; archive: `.planning/milestones/v7.5-ROADMAP.md`)
- [x] **v7.4 Gateway Scheduler** - Phases 14-16 (shipped 2026-07-04; archive: `.planning/milestones/v7.4-ROADMAP.md`)
- [x] **v7.3 PostgreSQL Compatibility** - Phase 13 (shipped 2026-07-03; archive: `.planning/milestones/v7.3-ROADMAP.md`)
- [x] **v7.2 Multi-Node Coordination** - Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- [x] **v7.1 Advanced Routing & Observability** - Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- [x] **v7.0 Plan 1 Foundation** - Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- [x] **v5** - Phases 5-6 (shipped 2026-06-29)
- [x] **v4** - Phases 1-4 (shipped 2026-06-23; archive: `.planning/milestones/v4-ROADMAP.md`)
- [ ] **Future milestones** - BFF/Admin Console or another gateway priority

## Planned v7.8 Phase

| Phase | Name | Goal | Requirements | Status |
|-------|------|------|--------------|--------|
| 26 | Scheduler Scoring Backpressure Hardening | 2/2 | Complete   | 2026-07-08 |

## Recently Shipped v7.7 Phases

| Phase | Name | Goal | Requirements | Status |
|-------|------|------|--------------|--------|
| 23 | Scheduler Queue Hardening | Make memory the default queue, keep Redis node-scoped when explicit, and fix FallbackQueue recovery reads. | SCHQ-01..04 | Shipped |
| 24 | Plan 3 Vector Compatibility | Document and implement Plan 3 LanceDB/Qdrant selection while preserving single-node limits and Qdrant semantic-neighbor compatibility. | PLAN3-01..04 | Shipped |
| 25 | Runbooks and Verification | Update queue/deployment docs and record test coverage and known limits. | DOC-01..02 | Shipped |

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB by default, or Qdrant when configured | P3 | Documented in v7.7 |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Shipped in v7.3 |

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** - JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.
- **Scheduler automation** - optional automatic ONNX rollout changes after explicit operator opt-in.

## Notes

- Scheduler is optional and disabled by default.
- Scheduler queueing defaults to in-memory in v7.7; Redis queueing is explicit and node-scoped.
- Scheduler scoring is an optimization path; slow or overloaded external scorer calls must degrade quickly to heuristic/FIFO instead of blocking Gateway ingress.
- Gateway remains the source of truth for queue ownership, task state, execution, promotion, and fallback behavior.
- Scheduler must not receive raw prompts, embeddings, semantic-cache payloads, provider secrets, API keys, or authorization headers.
- Static virtual deadline scoring remains the default; v7.5 only adds bounded, policy-driven SLA promotion.
- Source code committed to git must not contain hardcoded configuration.
- Config unification in v7.6 is backward-compatible: existing ENV variables remain valid; nested struct grouping is the new preferred form.

---
*Roadmap refreshed: 2026-07-08 - Phase 26 planned*
