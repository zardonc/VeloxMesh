# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Updated:** 2026-10-17
**Current focus:** Phase 30 — Request Budget and Safe Failover is planned and ready for implementation. Local main already contains PR #31 (merge 16a1c65); Phase 27/28 are verified and Phase 29 was integrated under the accepted candidate budget. Phase 29 production readiness remains partial; production cache stays disabled with an empty allowlist.

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
**Plans:** 2/3 plans executed; Phase 29 remains incomplete

**Latest execution:** [2026-10-08 expanded verification](debug/phase29-comprehensive-20261008/REPORT.md). The ordered Redis writer no longer queues subsequent same-key publications behind delayed replies; version rejection, errors and default 50ms deadlines remain. All 411 source hashes match the committed code: 613 backend PASS / 2 SKIP / 0 FAIL, 83 affected race PASS, 7 real-Redis PASS, 24 unique live cache/gateway PASS. Two local embedding model lifecycles were already recorded on 2026-10-03.

**Accepted integration contract (user decision, 2026-10-08):** semantic miss/low-hit P95 must satisfy both ≤1.25×off and delta≤40ms against each matched before/after off endpoint; exact miss delta≤10ms; hits P95≤60ms and ≤0.5×each off; residual P95≤10/P99≤15ms. Current 36-window, 3,600-request, three-block verification passes all nine scenario and interval checks. Original 1.05 still fails. Historical Formal07 (144 windows/43,200 requests) used an older binary and remains separate. This decision permits default-off code integration, not production activation or automatic Phase 29 completion.

Plans:
**Wave 1**

- [x] 29-01-PLAN.md

**Wave 2** *(offline implementation verified; numeric release approval deferred)*

- [x] 29-02-PLAN.md

**Wave 3** *(integration accepted; production open: original 1.05, independent semantic quality/seed correctness, empty-collection reclamation, historical primary EOF, longer/higher-load capacity and activation decision)*

- [ ] 29-03-PLAN.md

### Phase 30: Request Budget and Safe Failover

**Goal:** Bound the entire gateway request across cache, routing, admission, all provider attempts and streaming completion; complete connection-stage limits and constrain existing fallback to demonstrably safe failures without replaying ambiguous executions.
**Requirements:** TIMEOUT-F01, BUDGET-01, CONNECT-01, FAILOVER-01, BUDGET-OBS-01, BUDGET-UAT-01
**Depends on:** Phase 27/28 contracts and the integrated Phase 29 implementation. Phase 29 production activation is not a prerequisite; production remains default-off.
**Canonical refs:** `.planning/phases/30-request-budget-and-safe-failover/30-CONTEXT.md`; `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md` (external junction; 2026-10-17 update).

**Success criteria:**

1. A configured ingress deadline covers all request stages and attempts; earlier client deadlines win and no attempt starts after exhaustion.
2. Connection and per-attempt stage limits consume remaining budget; locked SDK implicit retries and actual invocation counts are verified.
3. Unknown upstream execution, client cancellation and started streams never replay; tool traffic retains its protocol and safety boundary.
4. Errors and cancellation release permits promptly and preserve once-only terminal/Usage settlement across ordinary, buffered and Fusion paths.
5. Phase-end real local HTTP/SSE fault acceptance emits redacted, repeatable evidence; absent new configuration preserves existing timeout defaults.

**Plans:** 0/8 plans executed; planning passed independent review.
**Estimate:** 4–7 engineering days, excluding review waits; acceptance evidence determines completion.

Plans:

**Wave 1**

- [ ] 30-01-PLAN.md — Optional ingress budget and a real HTTP tracer.

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 30-02-PLAN.md — Ordinary-request guards and bounded once-only finalization.

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 30-03-PLAN.md — Connection limits and locked SDK retry audit.

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 30-04-PLAN.md — Enforce SDK wire-attempt limits.

**Wave 5** *(blocked on Wave 4 completion)*

- [ ] 30-05-PLAN.md — Conservative cross-provider failover.

**Wave 6** *(blocked on Wave 5 completion)*

- [ ] 30-06-PLAN.md — Ordinary and buffered streaming lifetime.

**Wave 7** *(blocked on Wave 6 completion)*

- [ ] 30-07-PLAN.md — Fusion lifetime and cleanup.

**Wave 8** *(blocked on Wave 7 completion)*

- [ ] 30-08-PLAN.md — Low-cardinality observation and final repeatable HTTP/SSE acceptance.

### Phase 31: Operator Diagnostics Foundation

**Goal:** Provide trustworthy operator diagnostics before Console UI: bounded non-generating probes, explicit manual generation checks, gateway-defined TTFT/TTLT, and protected request-ID lookup of redacted metadata.
**Requirements:** DIAG-F01, PROBE-01, PROBE-ISO-01, TIMING-01, CORRELATION-01, DIAG-QUERY-01, DIAG-UAT-01
**Depends on:** Phase 30 implementation and verification, plus Phase 27/28 terminal/tool contracts. Phase 30 is planned, not implemented; Phase 31 is advance planning and must not skip that prerequisite.
**Canonical refs:** `.planning/phases/31-operator-diagnostics-foundation/31-CONTEXT.md`; `.planning/phases/31-operator-diagnostics-foundation/31-PATTERNS.md`; research report section 11.

**Success criteria:**

1. Background probes make no generation requests and report only the capability actually checked; light probe success never clears failures of real generation traffic.
2. Manual generation checks require an explicit authorized action, a registered provider/model, finite cost/work limits and separate probe accounting.
3. Gateway TTFT/TTLT measure successful client-facing semantic output from ingress; headers, empty/role/Usage/terminal frames and hidden Fusion output do not become token latency. Undefined timings remain nullable and E2E is separate.
4. Server-generated request identity survives all attempts; bounded, redacted node-local diagnostics are queryable only through authenticated and rate-limited Admin endpoints with clear disabled/expiry/node-scope behavior.
5. Real local HTTP/SSE fault acceptance verifies semantics, cost boundaries, redaction, bounded memory and data-plane isolation with repeatable evidence, preserving existing terminal/settlement behavior.

**Plans:** 0/9 plans executed; planning passed independent review, execution blocked on Phase 30 implementation/verification.
**Estimate:** 5–8 engineering days, excluding Phase 30 dependency and review waits; acceptance evidence determines completion.

Plans:

**Wave 1**

- [ ] 31-01-PLAN.md — Real non-generating Admin HTTP probe tracer.

**Wave 2** *(blocked on Wave 1 completion)*

- [ ] 31-02-PLAN.md — Native read probes and separate generation-health semantics.

**Wave 3** *(blocked on Wave 2 completion)*

- [ ] 31-03-PLAN.md — Independent bounded background probes and lifecycle.

**Wave 4** *(blocked on Wave 3 completion)*

- [ ] 31-04-PLAN.md — Explicit manual generation with durable audit and controlled dispatch.

**Wave 5** *(blocked on Wave 4 completion)*

- [ ] 31-05-PLAN.md — Trusted internal identity and compatible public request IDs.

**Wave 6** *(blocked on Wave 5 completion)*

- [ ] 31-06-PLAN.md — Semantic HTTP output timing through the real writer chain.

**Wave 7** *(blocked on Wave 6 completion)*

- [ ] 31-07-PLAN.md — Default-off bounded node-local capture and protected lookup.

**Wave 8** *(blocked on Wave 7 completion)*

- [ ] 31-08-PLAN.md — Finite attempt correlation and duplicate-safe external-ID query.

**Wave 9** *(blocked on Wave 8 completion)*

- [ ] 31-09-PLAN.md — Two fresh repeatable local HTTP/SSE acceptance runs.

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
- **Semantic-cache production gates** - Integrated implementation stays default-off; original latency target, business quality/seed facts, collection reclamation, capacity and activation approval remain open in Phase 29.
- **Console UI** - Follows Phase 31 diagnostics; reuse the protected Admin API and trustworthy probe/timing/request data rather than introducing a new gateway platform.
- **Model/account capability overrides** - Extend the existing catalog and tool filtering rather than rebuilding them.
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
*Roadmap refreshed: 2026-10-17 — PR #31 merge confirmed in local main; Phase 30 selected for implementation, Phase 31 planned next; Phase 29 production gates remain open.*
