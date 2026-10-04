---
gsd_state_version: "1.0"
milestone: v7.9
milestone_name: Phases
current_phase: 29
current_phase_name: Semantic Cache Latency Hardening
status: executing
stopped_at: Gemini terminal-tail and empty-scope readiness repaired; stable semantic performance, lifecycle and business safety pending
last_updated: "2026-10-04"
last_activity: 2026-10-04
last_activity_desc: 599 backend passes and real Gemini/cache readiness regressions; isolated path costs measured, no production performance approval
state_head: 9606669104617ea84c32f4959697fdb50f33f653
progress:
  total_phases: 3
  completed_phases: 2
  total_plans: 16
  completed_plans: 15
  percent: 93
---

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-09-25)

**Core value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

**Current focus:** Phase 29 — Semantic Cache Latency Hardening

## Current Implementation State

- Phase 1-10 (v7.0, v7.1) are implemented and verified.
- Phase 12 multi-node coordination (v7.2) is fully implemented, verified, and shipped.
- Phase 13 PostgreSQL Compatibility (v7.3) is fully implemented, verified, and shipped.
- Phase 14 Scheduler Queue Foundation (v7.4) is implemented, verified, and shipped.
- Phase 15 Training Feedback and ONNX Path (v7.4) is implemented, verified, and shipped.
- Phase 16 A/B Rollout and Prediction Quality (v7.4) is implemented, verified, and shipped.
- Phase 17 Semantic Neighbor Feature Aggregates (v7.5) is implemented and verified.
- Phase 18 Anomaly and OOD Conservative Scoring (v7.5) is implemented and verified, including production-shape ONNX artifact and Python worker/Scheduler call-chain coverage.
- Phase 19 SLA Waiting-Time Promotion (v7.5) is implemented and verified with sanitized promotion metrics, logs, and audit evidence.
- Phase 20 Config Unification + Scheduler Core Hardening (v7.6) is implemented and verified.
- Phase 21 Observability, Admin APIs & Tooling (v7.6) is implemented and verified.
- Phase 22 Documentation, .env.example & UAT (v7.6) is implemented and verified.
- Phase 23 Scheduler Queue Hardening (v7.7) is implemented, verified, and shipped.
- Phase 24 Plan 3 Vector Compatibility (v7.7) is implemented, verified, and shipped.
- Phase 25 Runbooks and Verification (v7.7) is implemented, verified, and shipped.
- Phase 26 Scheduler Scoring Backpressure Hardening (v7.8) is implemented, verified, and shipped.
- Phase 27 Stream Terminal and Settlement Consistency (v7.9) is implemented and verified.
- Phase 28 Tool Calling Protocol Completion (v7.9) is implemented and verified.

## Completed

- Phase 1-10 features (Routing, Observability, etc.)
- Phase 12: Multi-Node Coordination
- Phase 13: PostgreSQL Compatibility
- Phase 14: Scheduler Queue Foundation
- Phase 15: Training Feedback and ONNX Path
- Phase 16: A/B Rollout and Prediction Quality
- Phase 17: Semantic Neighbor Feature Aggregates
- Phase 18: Anomaly and OOD Conservative Scoring
- Phase 19: SLA Waiting-Time Promotion
- Phase 20: Config Unification + Scheduler Core Hardening
- Phase 21: Observability, Admin APIs & Tooling
- Phase 22: Documentation, .env.example & UAT
- Phase 23: Scheduler Queue Hardening
- Phase 24: Plan 3 Vector Compatibility
- Phase 25: Runbooks and Verification
- Phase 26: Scheduler Scoring Backpressure Hardening
- Phase 27: Stream Terminal and Settlement Consistency (v7.9) verified
- Phase 28: Tool Calling Protocol Completion (v7.9) verified

## Planned Next

1. Continue Phase 29 with stable semantic-path measurement, persistent collection ownership/reclamation and business-approved answer policy; preserve original performance evidence and production-disabled boundaries.

## Useful Commands

- `$gsd-progress` - review completed milestone status.
- `$gsd-new-milestone` - start the next milestone.
- `go test -timeout 60s ./...` - run the current Go test suite.

## Current Position

Phase: 29 (Semantic Cache Latency Hardening) — EXECUTING
Plan: 3 of 3
Status: Functional regressions pass; stable performance, collection lifecycle and business semantic safety remain open
Last activity: 2026-10-04 — Gemini terminal-tail and empty-scope readiness repaired; 599 backend passes; exact and semantic paths diagnosed with drift retained

## Operator Next Steps

- Review [further correction](phases/29-semantic-cache-latency-hardening/29-FURTHER-CORRECTION-20261004.md): real Gemini tail faults reject with zero settlement; wire parameters match for one successful pair; fresh scope readiness and PostgreSQL pass. Exact miss avoids embedding, while semantic miss still has about 21ms read cost and drifting baseline.
- Use [hardware-first evidence](phases/29-semantic-cache-latency-hardening/29-CONTINUED-INVESTIGATION-20261004.md) to avoid repeating VM expansion as the first remedy: fresh volumes pass recovery, but original stable 8 RPS ratios failed. Persistent collection ownership/reclamation and business seed/semantic approval remain open.
- Review [RAM-upgrade controlled results](phases/29-semantic-cache-latency-hardening/29-MEMORY-UPGRADE-RESULTS-20261004.md): 4800 formal requests and 69 selected real checks pass, but original P95, four drifting brackets and paging remain open; 347 Qdrant collections are inventoried.
- Review [recovery investigation](phases/29-semantic-cache-latency-hardening/29-RECOVERY-INVESTIGATION-20261004.md): no new formal windows; Qdrant-only startup pressure, sparse files and missing collection expiry motivate a controlled layout/lifecycle comparison.
- Resume 29-03 with observed recovery completion, fixed collection-state experiments and primary queue/prefill/decode correlation; evaluate business semantic safety independently. Two distinct model lifecycles are already recorded; numeric/production gates remain separate.
- Deployment remains separate; no deployment authorization was received.

## Deferred Items

Items acknowledged at v7.0 close:

| Category | Item | Status |
| --- | --- | --- |
| UAT | Phase 05 UAT report uses legacy status format and records older Phase 5 provider-specific gaps | Deferred from prior shipped milestone |

## Performance Metrics

| Phase | Plan | Duration | Notes |
|-------|------|----------|-------|
| Phase 14 P14-04 | 1h | 3 tasks | 17 files |
| Phase 15 P15-01 | 52min | 3 tasks | 21 files |
| Phase 15 P15-02 | 10min | 3 tasks | 15 files |
| Phase 15 P15-03 | 11min | 3 tasks | 12 files |
| Phase 17 P01 | 35 min | 2 tasks | 10 files |
| Phase 17 P02 | 40 min | 3 tasks | 12 files |
| Phase 17 P03 | 55min | 3 tasks | 22 files |
| Phase 18 P04 | 35 min | 3 tasks | 34 files |
| Phase 19 P01 | 19 min | 2 tasks | 4 files |
| Phase 19 P02 | 18 min | 3 tasks | 17 files |
| Phase 19 P03 | 16 min | 3 tasks | 6 files |
| Phase 22 P01 | 20 min | 4 tasks | 8 files |
**Per-Plan Metrics:**

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| Phase 28 P01 | 17min | 3 tasks | 7 files |
| Phase 28 P02 | 37m | 3 tasks | 8 files |
| Phase 28-tool-calling-protocol-completion P03 | 16min | 3 tasks | 4 files |
| Phase 29-semantic-cache-latency-hardening P01 | 1h | 2 tasks | 12 files |

## Accumulated Context

### Roadmap Evolution

- Phase 28 added: Tool Calling Protocol Completion
- Phase 29 added: Semantic Cache Latency Hardening

## Session

**Last session:** 2026-10-04
**Stopped at:** Gemini terminal-tail and scope readiness repaired; stable semantic performance/lifecycle/business gates pending
**Resume file:** 29-03-PLAN.md

## Decisions

- [Phase 28]: Use nil *ToolChoice for omission and a closed mode union for explicit choices.
- [Phase 28]: Normalize tool protocol once at the HTTP boundary and pass ToolProtocolRequirements downstream.
- [Phase 28]: Treat schemas, arguments, and results as opaque data; reject invalid structure before routing.
- [Phase 28]: Use a fail-closed per-model ToolProtocolCapability snapshot with explicit choice mode support.
- [Phase 28]: Preflight normalized tool requirements in every router branch and reject Fusion before decision construction.
- [Phase 28]: Tool-call shadow state remains side-effect free; Phase 27 retains all terminal ownership.
- [Phase 28]: Arguments stay opaque until completion and are capped per unfinished call.
- [Phase 28]: The shared harness accepts first-turn tool requests and validates supplied tool-result history.
- [Phase 29]: Semantic cache eligibility is an opaque trusted FAQ profile keyed by database-authenticated API key ID, knowledge version, fixed prompt, generation settings, and embedding identity.
- [Phase 29]: User authorized offline Wave 2 without final numeric approval; enabled cache bounds must be explicit. Isolation candidates are 100 ms / four reads / two workers / queue 32 / 2 s write / 1 s close, pending sustainable-load validation.
- [Phase 29]: Production remains off with empty allowlist; no account credits were purchased. Native Gemini and real app/Qdrant paths are repaired.
- [Phase 29]: The user-provided local embedding model is selected through configuration and real dimension probing. Gateway cache input is eligible user text only; system prompt remains in authorization scope, with an input-format version preventing old vectors from matching.
- [Phase 29]: 2026-10-04 explicit disabled/exact/experimental-semantic profiles and layered Provider deadlines/shared attempt capacity implemented. Exact regression rejects 41 changed questions; final backend and real protected SSE/Fusion pass. Scenario budgets retain original 1.05 evidence and require both 1.25 ratio and +40ms; no production approval.
- [Phase 29]: Continued 2026-10-04 investigation separates health-store read failure from provider unhealthy through fail-closed 503 health_state_unavailable. Five routing modes, readable peer, recovery and eleven selected real regressions pass; final backend is 595 top-level passes, zero failures.
- [Phase 29]: RAM upgrade is verified at 7376MiB with the same four vCPUs. Six bracket blocks per scenario yield 4800 successful formal requests; candidate AND point estimates and residual pass, original 1.05 fails. Four drifting brackets, observed swap-in and 323 small semantic collections keep stable performance/lifecycle investigation open; no production approval.

### Blockers

- New empty/pending-scope readiness is repaired and real SQLite/Qdrant/PostgreSQL regressions pass. Persistent collection reclamation remains open; semantic reads still pay embedding cost. One wire-equivalent Gemini pair passes but does not close historical intermittent headers timeouts. See 29-FURTHER-CORRECTION-20261004.md.
- Remote SANS embedding quota remains a separate external limit. Two real local embedding model scope lifecycles now pass; independent business-approved semantic recall and true isolated model cold state remain unverified.
- Historical six-block ordinary residual P95 was 13–27ms and failed the combined candidate contract. Later fresh-volume hardware-first results had low steady resource utilization but failed original 1.05 ratios; the latest semantic bracket drifted -14.37%, preventing acceptance. Natural health-read stalls, historical upstream 400 and stable capacity/SLO confidence remain open. See 29-AVAILABILITY-INVESTIGATION-20261004.md and the newer investigation reports; previous failed evidence remains intact.
- Production FAQ publisher, atomic version-switch procedure and deployment stop deadline are unconfirmed; production enablement requires separate approval.
