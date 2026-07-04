# Milestones

## v7.4 Gateway Scheduler (Shipped: 2026-07-04)

**Delivered:** Completed the optional Gateway Scheduler path while preserving the OpenAI-compatible data-plane contract and FIFO fallback behavior.

**Phases completed:** 14-16 (10 plans total)

**Key accomplishments:**

- Added disabled-by-default scheduler integration with gRPC `BatchScoreTasks`, 15ms timeout, circuit-breaker protection, and FIFO fallback.
- Added Redis ZSET queueing with task-id-only storage plus an in-memory single-node fallback.
- Added trusted priority resolution, tenant priority/ quota downgrade behavior, static virtual deadline scoring, and heuristic scheduler service health/metrics.
- Added safe opt-in scheduler training feedback, offline `uv` training/export/evaluate/publish tooling, and startup-loaded ONNX scheduler mode.
- Added heuristic/ONNX weighted rollout, prediction-quality rollups, sanitized comparison metrics, alerts, and authenticated runtime rollback controls.

**Known deferred work:**

- BFF/Admin Console UI remains deferred.
- Optional Qdrant semantic-neighbor features, anomaly detection, and tenant SLA waiting-time promotion remain future scheduler enhancements.

**What's next:** Confirm the next milestone scope: scheduler hardening/enhancements, BFF/Admin Console, or another gateway priority.

---

## v7.3 PostgreSQL Compatibility (Shipped: 2026-07-03)

**Delivered:** Added the PostgreSQL-compatible Plan 4 deployment path without changing the OpenAI-compatible data-plane contract.

**Phases completed:** 13 (4 plans total)

**Key accomplishments:**

- Added PostgreSQL + pgvector deployment documentation, templates, and readiness behavior.
- Closed PostgreSQL control-state compatibility gaps for active gateway capabilities.
- Added pgvector semantic-cache/vector adapter behavior behind the storage boundary.
- Added SQLite-to-PostgreSQL migration guidance/tooling and Plan 4 smoke verification.

**Known deferred work:**

- BFF/Admin Console UI remains Phase 11 scope.
- Full `LimitRule` unification across all scopes remains deferred.

**What's next:** Define and plan v7.4 Gateway Scheduler.

---

## v7.2 Multi-Node Coordination (Shipped: 2026-07-03)

**Phases completed:** 1 phases, 5 plans, 0 tasks

**Key accomplishments:**

- (none recorded)

---

## v7.1 Advanced Routing & Observability (Shipped: 2026-07-01)

**Delivered:** Completed the Composite Score Router and production-grade OpenTelemetry/Prometheus observability for routing decisions.

**Phases completed:** 10 (5 plans total)

**Key accomplishments:**

- Added composite routing across latency, pending requests, error rates, cost, and health signals.
- Added z-score normalization for routing signals.
- Added OpenTelemetry traces for TTFT, TPOT, end-to-end latency, and cache-hit behavior.
- Added Prometheus histogram metrics for routing and request timing.

**Known deferred work:**

- Phase 11: BFF Layer & Admin Console.
- Phase 12: Multi-Node Coordination.
- Phase 13: PostgreSQL Extension.
- Full `LimitRule` unification across future scopes.

**Archived:**

- `.planning/milestones/v7.1-ROADMAP.md`
- `.planning/milestones/v7.1-REQUIREMENTS.md`

**What's next:** Define the next milestone around Phase 12 multi-node coordination.

---

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
