# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-06
**Current focus:** v7.6 Scheduler 1.0 + Config System Unification

## Overview

VeloxMesh is being built as vertical gateway slices. The current gateway includes the Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, observability, multi-node coordination, PostgreSQL compatibility, and the optional Gateway Scheduler path.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.5 completed scheduler enhancement items deferred from v7.4: safe semantic-neighbor aggregate features, anomaly/OOD conservative scoring, and SLA waiting-time promotion.

## Active Milestone: v7.6 Scheduler 1.0 + Config System Unification

**Goal:** Polish the Scheduler to 1.0 release quality and unify the gateway-wide config structure.

### Phase 20: Config Unification + Scheduler Core Hardening

- CFG-01..04: Group ControlState/Redis/Cache config into named nested structs; support component-scoped config files.
- SCH-05..07: Executor concurrent slot control, Redis idempotency lock, QueueGuard observability.
- QDR-05..06: Embedding input truncation, automatic Qdrant collection initialization.

### Phase 21: Observability, Admin APIs & Tooling

- SCH-08: Admin status endpoint (queue depth, executor slots, circuit-breaker, quality rollups).
- QDR-07..08: Precise ID hydration for semantic neighbors; configurable embedding model.
- OBS-03..06: SLA rules API, training sample export, SchedulerType rollup fix, heuristic config file.

### Phase 22: Documentation, .env.example & UAT

- Updated `config.json.example` and `.env.example` with structured config layout.
- Scheduler 1.0 operator runbook (deployment guide, config reference, degradation scenarios).
- End-to-end UAT: scheduler enable/disable, degradation, semantic neighbors, admin APIs.

## Milestones

- [ ] **v7.6 Scheduler 1.0 + Config** - Phases 20-22 (active)
- [x] **v7.5 Scheduler Enhancements** - Phases 17-19 (shipped 2026-07-05; archive: `.planning/milestones/v7.5-ROADMAP.md`)
- [x] **v7.4 Gateway Scheduler** - Phases 14-16 (shipped 2026-07-04; archive: `.planning/milestones/v7.4-ROADMAP.md`)
- [x] **v7.3 PostgreSQL Compatibility** - Phase 13 (shipped 2026-07-03; archive: `.planning/milestones/v7.3-ROADMAP.md`)
- [x] **v7.2 Multi-Node Coordination** - Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- [x] **v7.1 Advanced Routing & Observability** - Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- [x] **v7.0 Plan 1 Foundation** - Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- [x] **v5** - Phases 5-6 (shipped 2026-06-29)
- [x] **v4** - Phases 1-4 (shipped 2026-06-23; archive: `.planning/milestones/v4-ROADMAP.md`)
- [ ] **Future milestones** - BFF/Admin Console or another gateway priority

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Shipped in v7.3 |

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** - JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.
- **Scheduler automation** - optional automatic ONNX rollout changes after explicit operator opt-in.

## Notes

- Scheduler is optional and disabled by default.
- Gateway remains the source of truth for queue ownership, task state, execution, promotion, and fallback behavior.
- Scheduler must not receive raw prompts, embeddings, semantic-cache payloads, provider secrets, API keys, or authorization headers.
- Static virtual deadline scoring remains the default; v7.5 only adds bounded, policy-driven SLA promotion.
- Source code committed to git must not contain hardcoded configuration.
- Config unification in v7.6 is backward-compatible: existing ENV variables remain valid; nested struct grouping is the new preferred form.

---
*Roadmap refreshed: 2026-07-06 — v7.6 active*
