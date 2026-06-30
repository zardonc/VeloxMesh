# Milestones

## v7.0 Plan 1 Foundation (Shipped: 2026-06-30)

**Delivered:** Completed the Plan 1 architecture foundation: SQLite durable state, Redis Stack hot state, Qdrant vector path, semantic pipeline, and Redis VSS fallback.

**Phases completed:** 7-9 (8 plans total)

**Key accomplishments:**

- Established SQLite-first durable runtime and narrow adapter contracts.
- Implemented configurable semantic pipeline rules, persistence, execution, and hot reload.
- Added Redis hot-state primitives, atomic admission checks, and recoverable security/accounting state.
- Wired Redis VSS fallback and typed config event routing for Qdrant degradation scenarios.

**Known deferred work:**

- Phase 10: Advanced Routing & Observability.
- Phase 11: BFF Layer & Admin Console.
- Phase 12: Multi-Node Coordination.
- Phase 13: PostgreSQL Extension.
- Full `LimitRule` unification across all future scopes.

**Archived:**

- `.planning/milestones/v7.0-ROADMAP.md`
- `.planning/milestones/v7.0-REQUIREMENTS.md`

**What's next:** Define the next milestone around Phase 10-13 scope.

---

## v5 Tool Calling, Multimodality, and Combos (Shipped: 2026-06-29)

**Phases completed:** 2 phases, 3 plans, 0 tasks

**Key accomplishments:**

- Phase 5: Tool/Function Calling and Multimodal capabilities
- Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing)

---

## v4 Streaming, Rate Limits, Cache, and Cost (Shipped: 2026-06-23)

**Phases completed:** 4 phases, 20 plans, 0 tasks

**Key accomplishments:**

- Phase 1: Gateway Walking Skeleton
- Phase 2: Health-Aware Multi-Provider Routing
- Phase 3: Durable Control State
- Phase 4: Streaming, Rate Limits, Cache, and Cost

---
