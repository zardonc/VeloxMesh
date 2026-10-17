---
gsd_state_version: "1.0"
milestone: v7.9
milestone_name: Phases
current_phase: 30
current_phase_name: Request Budget and Safe Failover
status: executing
stopped_at: Phase 31 planning complete; 9 plans passed independent review; blocked on Phase 30 implementation/verification; next execute Phase 30
last_updated: "2026-10-17T16:21:50.684Z"
last_activity: 2026-10-17
last_activity_desc: Phase 31 planning complete; 9 reviewed plans; execution blocked on Phase 30; no implementation started
state_head: 4a2e2bf620eedf0114bfdc65e3e43ff7a69e2721
progress:
  total_phases: 5
  completed_phases: 2
  total_plans: 37
  completed_plans: 15
  percent: 40
integration_status: accepted_candidate_budget
production_release_approved: false
---

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-10-17)

**Core value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

**Current focus:** Phase 30 — Request Budget and Safe Failover; Phase 29 production gates remain open

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

1. Execute the reviewed Phase 30 plans in dependency order on `codex/roadmap-next-phase-20261017`. This documentation session does not start implementation.
2. Phase 31 has 9 reviewed plans for Console prerequisites. Execute them only after Phase 30 implementation and verification pass; Console UI is deferred.
3. Keep Phase 29 production gates open, production caching off and the allowlist empty; local `main` already contains the PR #31 merge.

## Useful Commands

- `$gsd-progress` - review completed milestone status.
- `$gsd-new-milestone` - start the next milestone.
- `go test -timeout 60s ./...` - run the current Go test suite.

## Current Position

Phase: 30 (Request Budget and Safe Failover) — READY TO EXECUTE
Plan: Not started (0 of 8)
Status: Ready to execute
Last activity: 2026-10-17 — Phase 31 planning complete; 9 reviewed plans; execution blocked on Phase 30; no implementation started

## Operator Next Steps

- Use the [Phase 30 context](phases/30-request-budget-and-safe-failover/30-CONTEXT.md) and reviewed execution plans for the next implementation step.
- [Phase 31 context](phases/31-operator-diagnostics-foundation/31-CONTEXT.md) and [planning checks](phases/31-operator-diagnostics-foundation/31-PLANNING-CHECKS.md) record its 9-plan scope, static review and unmet Phase 30 prerequisite.
- PR #31 is already merged in local `main`; no PR submission or deployment is part of this documentation session.
- Historical 2026-10-08 validation: 613 backend PASS / 2 SKIP / 0 FAIL; 83 affected race, 7 real-Redis and 24 unique live cache/gateway PASS; 36 windows and 3,600 successful requests satisfy nine candidate checks. Current source matches the 411-file manifest after line-ending normalization.
- Earlier report-update checks: `go vet -tags phase29preflight ./...` and `go build ./...` exited zero; they were not rerun for Phase 31 planning. Current Phase 31 checks are plan structure/coverage and independent static review only. Historical E2E/performance results are not reruns; older Formal07 retains its own binary identity.
- Keep production cache disabled and allowlist empty. No production rollout is included.

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
- Phase 30 added: Request Budget and Safe Failover selected; local main contains PR31; Phase29 production stays partial
- Phase 31 added: Operator Diagnostics Foundation; advance planning only, execution follows verified Phase30

## Session

**Last session:** 2026-10-17T16:21:50.451Z
**Stopped at:** Phase 31 planning complete; 9 plans passed independent review; blocked on Phase 30 implementation/verification; next execute Phase 30
**Resume file:** .planning/phases/30-request-budget-and-safe-failover/30-01-PLAN.md

## Decisions

- [Phase 29, 2026-10-08]: User accepts candidate budgets for code integration: semantic miss/low-hit ≤1.25×each off AND delta≤40ms; exact miss delta≤10ms; hits ≤60ms AND ≤0.5×each off; residual P95≤10/P99≤15ms. Original 1.05 failure remains recorded; production enablement is not approved.
- [Phase 29, 2026-10-08]: Ordered Redis publication avoids redundant same-key serialization; real default50ms/order/recovery checks pass. Two outdated live test premises were corrected without weakening assertions. Current source is committed and matches the latest 411-file manifest.

Earlier decisions below are retained with their original scope; candidate-budget approval above supersedes earlier numeric approval deferrals for integration only.

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
- [Phase 30]: Owner selects request-wide budget and conservative failover: ambiguous execution timeout ends without replay; started output is never replayed
- [Phase 31]: Owner selects Console prerequisite diagnostics; UI later; background probes non-generating, generation checks require explicit administrator action
- [Phase 31]: Default-off bounded node-local redacted metadata and trusted internal IDs; Phase30 implementation and verification remain execution prerequisite

### Remaining Production Gates

- Original 1.05 target still fails; no hardware physical limit has been established. Current candidate performance passes within the measured workload, with only three blocks and off drift −11.31% to +5.18%.
- Independent semantic quality and seed facts, empty-collection reclamation, longer/higher-load and cold-start capacity remain open. Two local model lifecycles do not qualify every model or the GPU alternative.
- Historical primary EOF and external provider reliability remain unresolved; successful later runs do not close intermittent failures.
- Production FAQ publisher, atomic version-switch procedure, deployment stop deadline and activation approval remain separate from code integration.
