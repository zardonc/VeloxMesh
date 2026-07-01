# Requirements

## Current Status

Milestone v7.1 Advanced Routing & Observability is being planned.

Scope is Phase 10 only. BFF/Admin, multi-node coordination, and PostgreSQL extension work are deferred to later milestones.

## Completed in v7.0

- [x] Phase 7: Adapter Interfaces & SQLite Foundation.
- [x] Phase 8: Semantic Pipeline.
- [x] Phase 9: Redis Stack + Qdrant Fallback Integration.

See `.planning/milestones/v7.0-REQUIREMENTS.md` for the archived requirement outcomes.

## v7.1 Requirements

### Routing

- [ ] **ROUT-01**: Gateway can route provider requests through a Composite Score Router.
- [ ] **ROUT-02**: Gateway can score routing candidates using latency, pending requests, error rates, cost, and health bonus signals.
- [ ] **ROUT-03**: Gateway can normalize routing signals with z-score normalization before weighting.

### Observability

- [ ] **OBS-01**: Operators can inspect OpenTelemetry traces for TTFT, TPOT, end-to-end latency, and cache-hit behavior.
- [ ] **OBS-02**: Operators can scrape Prometheus histogram metrics for routing and request timing.

## Deferred to Future Milestones

- Phase 11: BFF Layer & Admin Console.
- Phase 12: Multi-Node Coordination.
- Phase 13: PostgreSQL Extension.
- Full `LimitRule` unification across future scopes.

## Traceability

| Area | Status | Notes |
| --- | --- | --- |
| v7.0 Plan 1 Foundation | Complete | Archived in `.planning/milestones/v7.0-REQUIREMENTS.md`. |
| v7.1 Advanced Routing & Observability | Planning | Phase 10 only. |
