# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-03
**Current focus:** v7.4 Gateway Scheduler

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-10 established the runnable Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, and observability.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.4 adds an optional Scheduler that scores queued work while the gateway remains the owner of intake, queue storage, execution, and fallback behavior.

## Milestones

- [x] **v7.0 Plan 1 Foundation** - Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- [x] **v7.1 Advanced Routing & Observability** - Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- [x] **v7.2 Multi-Node Coordination** - Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- [x] **v7.3 PostgreSQL Compatibility** - Phase 13 (shipped 2026-07-03)
- [ ] **v7.4 Gateway Scheduler** - Phases 14-16 (active)
- [x] **v5** - Phases 5-6 (shipped 2026-06-29)
- [x] **v4** - Phases 1-4 (shipped 2026-06-23)
- [ ] **Future milestones** - BFF/Admin Console

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Shipped in v7.3 |

## Scheduler Runtime Modes

| Mode | Components | Behavior |
| --- | --- | --- |
| Disabled | Gateway only | FIFO score from current timestamp; no Scheduler dependency. |
| Heuristic | Gateway + heuristic Scheduler | Structured/rule classification and configured latency tables. |
| ONNX | Gateway + ONNX Scheduler | Versioned model artifacts, startup-time model load, prediction quality monitoring, and A/B routing. |

<details>
<summary>v7.3 PostgreSQL Compatibility (Phase 13) - SHIPPED 2026-07-03</summary>

- [x] Phase 13: PostgreSQL Compatibility (4/4 plans) - completed 2026-07-03

</details>

<details open>
<summary>v7.4 Gateway Scheduler (Phases 14-16) - ACTIVE</summary>

**Goal:** Add an optional stateless Scheduler that scores queued gateway tasks while the gateway keeps ownership of intake, queueing, execution, and fallback behavior.

| Phase | Name | Requirements | Success Criteria |
| --- | --- | --- | --- |
| 14 | Scheduler Queue Foundation | SCH-01, SCH-02, SCH-03, SCH-04, PRIO-01, PRIO-02, SCORE-01, SCORE-02, OBS-01 | Disabled/FIFO path, gRPC fallback, queue backend, heuristic scoring, priority safety, and core metrics work. |
| 15 | Training Feedback and ONNX Path | FEED-01, ML-01, ML-02 | Training samples are captured safely and ONNX model artifacts can be trained, loaded, and served. |
| 16 | A/B Rollout and Prediction Quality | OBS-02, ML-03 | Gateway can compare heuristic/ONNX backends and rollback through routing config. |

</details>

## v7.4 Gateway Scheduler

**Goal:** Add an optional stateless Scheduler that scores queued gateway tasks without changing the OpenAI-compatible data-plane contract.
**Requirements:** SCH-01, SCH-02, SCH-03, SCH-04, PRIO-01, PRIO-02, SCORE-01, SCORE-02, FEED-01, OBS-01, OBS-02, ML-01, ML-02, ML-03

### Phase 14: Scheduler Queue Foundation

**Goal:** Build the cold-start Scheduler path: optional service integration, queue backend, fallback behavior, heuristic scoring, priority safety, and core observability.
**Priority:** P2
**Depends on:** Phase 13
**Requirements:** SCH-01, SCH-02, SCH-03, SCH-04, PRIO-01, PRIO-02, SCORE-01, SCORE-02, OBS-01

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

Success criteria:

1. Gateway writes enqueue feature snapshots and completion labels without raw prompts, authorization headers, API keys, or provider secrets.
2. Training sample storage records scheduler type, scheduler version, prediction values, actual duration, output tokens, and completion timestamps.
3. Offline tooling can export completed samples, train/evaluate a P70 predictor, validate ONNX parity, and publish a versioned artifact directory.
4. ONNX Scheduler loads model artifacts once at startup and serves predicted latency, confidence, and scheduler version through the same scoring interface.

Candidate plan slices:

- **15-01 Training sample schema and collector**: add safe sample storage, completion updates, and retention-friendly indexes.
- **15-02 Export and training scripts**: add sample export, feature schema, P70 model training, ONNX export, and parity check.
- **15-03 ONNX Scheduler**: add startup model load, prediction response metadata, and failure fallback to conservative scoring.

### Phase 16: A/B Rollout and Prediction Quality

**Goal:** Let operators compare heuristic and ONNX scoring safely, then rollback or keep the model path based on observed prediction quality.
**Priority:** P2
**Depends on:** Phase 15
**Requirements:** OBS-02, ML-03

Success criteria:

1. Gateway can route scheduler calls to heuristic and ONNX backends by configuration.
2. Operators can compare wait time, call latency, scheduler errors, prediction MAPE, and task-type quality by scheduler version.
3. ONNX rollout can be disabled or rolled back to heuristic/FIFO without changing the data-plane API.
4. Documentation explains the cold-start, heuristic, ONNX, A/B, and rollback paths.

Candidate plan slices:

- **16-01 Scheduler routing**: add single, weighted, or primary-fallback backend selection.
- **16-02 Prediction quality metrics**: compute MAPE by task type, scheduler type, and scheduler version.
- **16-03 Rollout docs and verification**: document deployment, A/B observation, rollback, and smoke checks.

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** - JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.
- **Scheduler enhancements** - optional Qdrant semantic-neighbor features, anomaly detection, and SLA waiting-time promotion after the core Scheduler path is stable.

## Gateway Runtime Modes

VeloxMesh supports progressive deployment tiers:

- **Plan 1 (Standalone Enhanced)**: SQLite + Redis Stack + Qdrant.
- **Plan 2 (Multi-Node)**: Multi App + Redis Stack + SQLite + Qdrant.
- **Plan 3 (Edge)**: SQLite + LanceDB behind `-tags lancedb`, Linux/macOS only.
- **Plan 4 (Extension)**: Redis Stack + PostgreSQL + pgvector.

## Notes

- Scheduler is optional and disabled by default.
- Gateway remains the source of truth for queue ownership, task state, execution, and fallback behavior.
- Scheduler must not receive raw prompts, provider secrets, API keys, or authorization headers.
- Static virtual deadline scoring is preferred over dynamic score updates to avoid Redis write amplification.
- Source code committed to git must not contain hardcoded configuration. Configuration must come from local environment variables, configuration files, or the database.

---
*Roadmap refreshed: 2026-07-03 after starting v7.4 Gateway Scheduler*
