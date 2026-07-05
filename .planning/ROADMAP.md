# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-07-05
**Current focus:** v7.5 Scheduler Enhancements

## Overview

VeloxMesh is being built as vertical gateway slices. Phases 1-16 established the runnable Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, observability, multi-node coordination, PostgreSQL compatibility, and the optional Gateway Scheduler path.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.5 completes the scheduler enhancement items deferred from v7.4: safe semantic-neighbor aggregate features, anomaly/OOD conservative scoring, and SLA waiting-time promotion.

## Milestones

- [ ] **v7.5 Scheduler Enhancements** - Phases 17-19 (active)
- [x] **v7.4 Gateway Scheduler** - Phases 14-16 (shipped 2026-07-04; archive: `.planning/milestones/v7.4-ROADMAP.md`)
- [x] **v7.3 PostgreSQL Compatibility** - Phase 13 (shipped 2026-07-03)
- [x] **v7.2 Multi-Node Coordination** - Phase 12 (shipped 2026-07-03; archive: `.planning/milestones/v7.2-ROADMAP.md`)
- [x] **v7.1 Advanced Routing & Observability** - Phase 10 (shipped 2026-07-01; archive: `.planning/milestones/v7.1-ROADMAP.md`)
- [x] **v7.0 Plan 1 Foundation** - Phases 7-9 (shipped 2026-06-30; archive: `.planning/milestones/v7.0-ROADMAP.md`)
- [x] **v5** - Phases 5-6 (shipped 2026-06-29)
- [x] **v4** - Phases 1-4 (shipped 2026-06-23)
- [ ] **Future milestones** - BFF/Admin Console or another gateway priority

## Deployment Tiers

| Tier | Components | Priority | Status |
| --- | --- | --- | --- |
| **Plan 1**: Standalone Enhanced | App + Redis Stack + SQLite + Qdrant | P0 | Shipped in v7.0 |
| **Plan 2**: Multi-Node | Multi App + Redis Stack + SQLite + Qdrant | P1 | Shipped in v7.2 |
| **Plan 3**: Edge | App + SQLite + LanceDB (`-tags lancedb`, Linux/macOS only) | P3 | Future |
| **Plan 4**: Extension | App + Redis Stack + PostgreSQL + pgvector | P3 | Shipped in v7.3 |

## v7.5 Scheduler Enhancements

**Goal:** Complete the missing scheduler enhancement path without changing the OpenAI-compatible data-plane contract or moving queue ownership out of Gateway.
**Requirements:** QDR-01, QDR-02, QDR-03, QDR-04, ANOM-01, ANOM-02, ANOM-03, ANOM-04, SLA-01, SLA-02, SLA-03, SLA-04

### Phases

- [x] Phase 17: Semantic Neighbor Feature Aggregates (completed 2026-07-05)
- [x] Phase 18: Anomaly and OOD Conservative Scoring (completed 2026-07-05)
- [ ] Phase 19: SLA Waiting-Time Promotion

### Phase 17: Semantic Neighbor Feature Aggregates

**Goal:** Add optional Qdrant/vector-backed semantic-neighbor aggregate features while keeping raw text, embeddings, and semantic-cache payloads out of Scheduler.
**Priority:** P2
**Depends on:** Phase 16
**Requirements:** QDR-01, QDR-02, QDR-03, QDR-04
**Plans:** 3/3 plans complete

Success criteria:

1. Gateway can collect bounded semantic-neighbor aggregate stats only when explicitly enabled and vector/embedding dependencies are configured.
2. Scheduler feature payloads and training samples include only numeric or enum aggregate fields with safe defaults when semantic lookup is unavailable.
3. Semantic enrichment has a tight timeout budget and falls back without blocking data-plane forwarding or scheduler scoring.
4. Metrics expose semantic enrichment attempts, timeouts, errors, and fallback reasons with sanitized labels.

Candidate plan slices:

- **17-01 Feature contract and config**: add bounded semantic aggregate fields, disabled-by-default config, and safe defaults.
- **17-02 Gateway semantic collector**: reuse existing vector/embedding boundaries to collect aggregate stats under timeout and degradation rules.
- **17-03 Training and ONNX feature path**: carry semantic aggregate fields through export, training, artifact metadata, and scheduler scoring defaults.

### Phase 18: Anomaly and OOD Conservative Scoring

**Goal:** Let ONNX Scheduler recognize unfamiliar tasks and respond conservatively through confidence and uncertainty signals.
**Priority:** P2
**Depends on:** Phase 17
**Requirements:** ANOM-01, ANOM-02, ANOM-03, ANOM-04
**Plans:** 4/4 plans complete

Success criteria:

1. Offline tooling computes anomaly/OOD thresholds from safe scheduler samples and publishes them in versioned artifacts.
2. ONNX Scheduler validates anomaly metadata at startup and reports clear degraded or fallback behavior.
3. Out-of-distribution tasks produce lower confidence, higher uncertainty, and more conservative virtual-deadline scores.
4. Quality rollups and metrics compare anomaly rate, MAPE, and fallback behavior by scheduler version and task type.
5. Verification uses the same production-shape ONNX model artifact and Python worker/Scheduler call chain that production will ship; only training data volume may differ.

Candidate plan slices:

- **18-01 Artifact thresholds**: compute and publish anomaly/OOD metadata from safe training samples.
- **18-02 Runtime conservatism**: apply anomaly signals in ONNX scoring without changing the scheduler RPC contract.
- **18-03 Quality evidence**: add rollup/metric coverage and tests for anomaly flags, fallback, and prediction quality.
- **18-04 Predictor boundary and real ONNX Runtime**: route Scheduler ONNX mode through the model-neutral predictor boundary, long-lived Python ONNX Runtime worker, and production-shape feature-driven ONNX artifact.

### Phase 19: SLA Waiting-Time Promotion

**Goal:** Promote tasks that exceed tenant waiting thresholds while preserving trusted priority, quota, and Gateway-owned queue behavior.
**Priority:** P2
**Depends on:** Phase 18
**Requirements:** SLA-01, SLA-02, SLA-03, SLA-04
**Plans:** 1/3 plans executed

Success criteria:

1. Operators can configure tenant, model, and request-kind waiting thresholds with promotion disabled by default.
2. Gateway can reorder Redis ZSET and in-memory queued tasks when thresholds are exceeded.
3. Promotion respects trusted priority, tenant max-priority, and high-priority quota limits.
4. Promotion decisions emit sanitized audit, log, and metric records without raw prompts or secrets.

Candidate plan slices:

- **19-01 Promotion policy config**: add disabled-by-default SLA threshold config and validation.
- **19-02 Queue reordering support**: extend queue backends or runner flow to update Redis ZSET and memory queue scores safely.
- **19-03 Promotion audit and metrics**: add tests and operator evidence for quota-safe promotion behavior.

## Future Milestones

- **Phase 11: BFF Layer & Admin Console** - JWT authentication, role-based access control, session management, and Admin Console foundation. Depends on Phase 7.
- **Scheduler automation** - optional automatic ONNX rollout changes after explicit operator opt-in.

## Notes

- Scheduler is optional and disabled by default.
- Gateway remains the source of truth for queue ownership, task state, execution, promotion, and fallback behavior.
- Scheduler must not receive raw prompts, embeddings, semantic-cache payloads, provider secrets, API keys, or authorization headers.
- Static virtual deadline scoring remains the default; v7.5 only adds bounded, policy-driven SLA promotion.
- Source code committed to git must not contain hardcoded configuration.

---
*Roadmap refreshed: 2026-07-05 after confirming Phase 18 production-shape ONNX verification*
