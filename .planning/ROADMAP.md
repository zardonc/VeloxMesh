# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-03
**Current focus:** v7.4 Gateway Scheduler

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-10 established the runnable Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, and observability.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.4 adds an optional stateless Scheduler that scores queued work while the gateway keeps ownership of intake, queue storage, execution, and fallback behavior.

## Milestones

- ✅ **v7.0 Plan 1 Foundation** - Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- ✅ **v7.1 Advanced Routing & Observability** - Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- ✅ **v7.2 Multi-Node Coordination** - Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- ✅ **v7.3 PostgreSQL Compatibility** - Phase 13 (shipped 2026-07-03)
- ◆ **v7.4 Gateway Scheduler** - Phases 14-16 (active)
- ○ **Future milestones** - BFF/Admin Console
- ✅ **v5** - Phases 5-6 (shipped 2026-06-29)
- ✅ **v4** - Phases 1-4 (shipped 2026-06-23)

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Shipped in v7.3 |

## v7.4 Gateway Scheduler

**Goal:** Add an optional stateless Scheduler that scores queued gateway tasks without changing the OpenAI-compatible data-plane contract.
**Requirements:** SCH-01, SCH-02, SCH-03, SCH-04, PRIO-01, PRIO-02, SCORE-01, SCORE-02, FEED-01, OBS-01, OBS-02, ML-01, ML-02, ML-03

### Phases

- [ ] Phase 14: Scheduler Queue Foundation
- [ ] Phase 15: Training Feedback and ONNX Path
- [ ] Phase 16: A/B Rollout and Prediction Quality

### Phase 14: Scheduler Queue Foundation

**Goal:** Build the cold-start Scheduler path: optional service integration, queue backend, fallback behavior, heuristic scoring, priority safety, and core observability.
**Priority:** P2
**Depends on:** Phase 13
**Requirements:** SCH-01, SCH-02, SCH-03, SCH-04, PRIO-01, PRIO-02, SCORE-01, SCORE-02, OBS-01
**Plans:** 2/4 plans executed

Success criteria:

1. Gateway starts and forwards normally when Scheduler is disabled, unavailable, timing out, or breaker-open.
2. `scheduler.v1` gRPC scoring is wired with 15ms timeout, no inline retry, and FIFO fallback.
3. Redis ZSET queue operations and in-memory single-node fallback share one `QueueBackend` boundary.
4. Heuristic Scheduler returns static virtual deadline scores with configured task classification, latency estimates, priority multipliers, and uncertainty penalty.
5. Priority is resolved only from trusted structured inputs, max-priority/quota policy is enforced, and core scheduler/queue metrics are exposed.

Candidate plan slices:

- **14-01 Proto, config, and client fallback**: define `scheduler.v1`, add disabled-by-default config, gRPC client, timeout, and breaker-safe fallback.
- **14-02 Queue backend**: implement Redis ZSET queue operations, in-memory heap fallback, and queue depth behavior.
- **14-03 Heuristic Scheduler**: implement structured/rule classification, score calculator, gRPC service, health, and metrics.
- **14-04 Priority and observability**: implement trusted priority resolution, quota downgrade audit, logs, and Prometheus metrics.

### Phase 15: Training Feedback and ONNX Path

**Goal:** Record safe training data and establish the versioned ONNX model path without making ML required for cold start.
**Priority:** P2
**Depends on:** Phase 14
**Requirements:** FEED-01, ML-01, ML-02
**Plans:** 3 plans

Success criteria:

1. Gateway writes enqueue feature snapshots and completion labels without raw prompts, authorization headers, API keys, or provider secrets.
2. Offline tooling can export completed samples, train/evaluate a P70 predictor, validate ONNX parity, and publish a versioned artifact directory.
3. ONNX Scheduler loads model artifacts once at startup and serves predicted latency, confidence, and scheduler version through the same scoring interface.

### Phase 16: A/B Rollout and Prediction Quality

**Goal:** Let operators compare heuristic and ONNX scoring safely, then rollback or keep the model path based on observed prediction quality.
**Priority:** P2
**Depends on:** Phase 15
**Requirements:** OBS-02, ML-03
**Plans:** 3 plans

Success criteria:

1. Gateway can route scheduler calls to heuristic and ONNX backends by configuration.
2. Operators can compare wait time, call latency, scheduler errors, prediction MAPE, and task-type quality by scheduler version.
3. ONNX rollout can be disabled or rolled back to heuristic/FIFO without changing the data-plane API.

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
*Roadmap refreshed: 2026-07-03 after starting v7.4 Gateway Scheduler*
