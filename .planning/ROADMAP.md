# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-09-25
**Current focus:** Phase 29 semantic-cache latency hardening planning; v7.9 verified, no deployment recorded

## Overview

VeloxMesh is being built as vertical gateway slices. The current gateway includes the Go/Chi OpenAI-compatible data plane, provider adapters, durable control state, streaming, rate limits, semantic caching, Redis/Qdrant Plan 1 infrastructure, advanced routing, observability, multi-node coordination, PostgreSQL compatibility, and the optional Gateway Scheduler path.

The architecture uses SQLite + Redis Stack + Qdrant for the main Plans 1/2 path, with PostgreSQL + pgvector available as the Plan 4 extension path. v7.7 shipped Scheduler queue default hardening, Redis node-scoped queueing, FallbackQueue recovery reads, and Plan 3 single-node LanceDB/Qdrant vector compatibility. Phase 26 hardened synchronous Scheduler scoring, process-local admission limits, task context propagation, admin rollout reporting, predictor breaker config, and multilingual feature extraction.

## Milestones

- [x] **v7.9 Gateway Protocol Correctness** - Phases 27-28 verified 2026-09-25; no deployment recorded
- [x] **v7.8 Scheduler Scoring Backpressure Hardening** - Phase 26 (shipped 2026-07-10)
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
- [ ] **Future milestones** - Semantic-cache latency, staged timeouts, BFF/Admin Console, or Scheduler automation

## Completed v7.9 Phases

| Phase | Name | Goal | Requirements | Status |
|-------|------|------|--------------|--------|
| 27 | Stream Terminal and Settlement Consistency | Unify stream terminal classification and exactly-once finalization across ordinary, buffered, and Fusion paths without adding hot-path I/O. | TERM-01..08 | Verified |
| 28 | Tool Calling Protocol Completion | Complete strict OpenAI-compatible tool protocol handling across validation, routing, gateway, provider adapters, streaming state, and observability. | TOOL-F01 | Complete    |

### Phase 27: Stream Terminal and Settlement Consistency

**Goal:** Make every streaming request end with one authoritative terminal outcome that drives client output, provider health, circuit breaker, observability, admission release, and usage settlement consistently.

**Depends on:** Phase 4 streaming/usage foundations, Phase 6 Fusion streaming, and Phase 10 observability.

**Requirements:** TERM-01, TERM-02, TERM-03, TERM-04, TERM-05, TERM-06, TERM-07, TERM-08.

**Estimated implementation:** 6.5 engineer-days (critical path: 5.5 engineer-days), excluding review latency and real-provider soak time.

**Success criteria:**

1. Done and clean EOF complete successfully; provider errors retain their mapped category; policy rejection and client cancellation do not damage provider health.
2. Every terminal side effect is observable exactly once across ordinary, buffered, and Fusion streams.
3. Only successful completed streams settle client usage; missing final Usage is recorded as `missing_usage`; failure and cancellation never debit the client.
4. SSE output contains one terminal sequence, and downstream write failure stops forwarding and releases resources.
5. Focused benchmarks or allocation-aware tests show no new external I/O and no material per-chunk regression.

**Plans:**

1. `27-01` P0 — Unified terminal model and exactly-once finalizer contract (Wave 1, 1.0 day).
2. `27-02` P0 — Ordinary, buffered, and Fusion lifecycle closure through that contract (Wave 2, 1.5 days).
3. `27-03` P0 — Request cancellation and checked SSE output (Wave 3, 1.0 day).
4. `27-04` P0 — Outcome-gated Usage settlement and observable persistence failure (Wave 3, 1.0 day).
5. `27-06` P1 — Fusion consistency verification only; no aggregation, routing, or Judge change (Wave 4, 0.5 day).
6. `27-05` P0 — Cross-layer terminal regression matrix and phase-gate evidence (Wave 5, 1.5 days).

### Phase 28: Tool Calling Protocol Completion

**Goal:** Complete the `/v1/chat/completions` tool-calling protocol end to end so OpenAI-compatible, Anthropic, and Gemini preserve one strict public contract across validated requests, capability-aware routing, non-stream and stream responses, and multi-turn tool-result continuation while Phase 27 remains the sole terminal and Usage-settlement owner.
**Requirements:** TOOL-F01
**Depends on:** Phase 27
**Plans:** 7/7 plans complete

**Verification:** Passed 2026-09-25; see `.planning/phases/28-tool-calling-protocol-completion/28-VERIFICATION.md`.

Plans:

- [x] 28-01-PLAN.md
- [x] 28-02-PLAN.md
- [x] 28-03-PLAN.md
- [x] 28-04-PLAN.md
- [x] 28-05-PLAN.md
- [x] 28-06-PLAN.md
- [x] 28-07-PLAN.md

- [x] `28-01-PLAN.md` — Define and enforce the normalized public tool protocol at the HTTP boundary
- [x] `28-02-PLAN.md` — Add capability-aware routing, explicit OPT-OUT, Fusion rejection, and fallback safety
- [x] `28-03-PLAN.md` — Build the shared streaming shadow state machine and cross-provider contract harness
- [x] `28-04-PLAN.md` — Complete OpenAI-compatible non-stream and streaming tool adaptation
- [x] `28-05-PLAN.md` — Complete Anthropic non-stream and streaming tool adaptation
- [x] `28-06-PLAN.md` — Complete Gemini non-stream and streaming tool adaptation
- [x] `28-07-PLAN.md` — Gateway Integration, Observability, and Phase Gate

**Cross-cutting constraints:**

- D-29 through D-33: Pinned SDK behavior is rechecked, deterministic shared/provider fixtures are the evidence, all Go test commands use a 60-second timeout, and no dependency is upgraded.

## Planned Next Phase

### Phase 29: Semantic Cache Latency Hardening

**Goal:** Make explicitly allowlisted semantic answer reuse safe and latency-bounded for non-streaming requests, with configurable embedding models, version/tenant isolation, asynchronous writes, and fail-open cache bypass that preserves primary forwarding.
**Requirements:** CACHE-F01
**Depends on:** Phase 28
**Plans:** 0/3 plans executed

Plans:
**Wave 1**

- [ ] 29-01-PLAN.md

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 29-02-PLAN.md

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 29-03-PLAN.md

## Shipped v7.8 Phase

| Phase | Name | Goal | Requirements | Status |
|-------|------|------|--------------|--------|
| 26 | Scheduler Scoring Backpressure Hardening | 3/3 | Shipped | 2026-07-10 |

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
- **Semantic-cache latency hardening** - Configurable embedding path, short read budget, bounded asynchronous write path, and failure isolation.
- **Stage timeout and cancellation hardening** - Connect, first-byte, stream-idle, and total-duration budgets with explicit retry eligibility.
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
*Roadmap refreshed: 2026-09-25 - v7.9 Phase 28 verified; milestone complete, deployment not performed*
