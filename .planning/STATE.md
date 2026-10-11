---
gsd_state_version: "1.0"
milestone: v7.9
milestone_name: Phases
current_phase: 29
current_phase_name: Semantic Cache Latency Hardening
status: executing
stopped_at: Candidate budget accepted for all-commit integration; documentation synchronized for PR; production cache off
last_updated: "2026-10-08"
last_activity: 2026-10-08
last_activity_desc: 411 source hashes match; 613 backend passes, 83 race passes and 3600 successful measured requests; production partial
state_head: 214422cb4ca0d40e455ce3dd7468323dbd092005
integration_status: accepted_candidate_budget
production_release_approved: false
progress:
  total_phases: 3
  completed_phases: 2
  total_plans: 16
  completed_plans: 15
  percent: 93
---

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-10-08)

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

1. Submit all current branch commits and synchronized documentation in a PR to main under the accepted candidate budget. Keep production caching off and Phase 29 production work partial.

## Useful Commands

- `$gsd-progress` - review completed milestone status.
- `$gsd-new-milestone` - start the next milestone.
- `go test -timeout 60s ./...` - run the current Go test suite.

## Current Position

Phase: 29 (Semantic Cache Latency Hardening) — EXECUTING
Plan: 3 of 3
Status: Application and candidate-budget verification pass for integration; production gates remain open
Last activity: 2026-10-08 — user accepts candidate budget, all branch commits and planning history; latest 411-source manifest verified against committed code

## Operator Next Steps

- Use [current verification](phases/29-semantic-cache-latency-hardening/29-VERIFICATION.md) and the [consolidated review](REVIEW-20261008.md) for PR review; earlier measurements retain their dates and source identities.
- Preserve all 68 existing commits when submitting the PR; documentation adds a new commit without filtering planning history.
- Latest validation: 613 backend PASS / 2 SKIP / 0 FAIL; 83 affected race, 7 real-Redis and 24 unique live cache/gateway tests pass. Performance: 36 windows, 3,600 successful requests, nine candidate scenario/interval checks pass.
- Treat the three-block current-binary run separately from the older Formal07; do not rerun large batches solely for documentation changes.
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

## Session

**Last session:** 2026-10-08
**Stopped at:** All-commit PR preparation under accepted candidate budget; documentation synchronized, production partial
**Resume file:** 29-03-PLAN.md

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

### Remaining Production Gates

- Original 1.05 target still fails; no hardware physical limit has been established. Current candidate performance passes within the measured workload, with only three blocks and off drift −11.31% to +5.18%.
- Independent semantic quality and seed facts, empty-collection reclamation, longer/higher-load and cold-start capacity remain open. Two local model lifecycles do not qualify every model or the GPU alternative.
- Historical primary EOF and external provider reliability remain unresolved; successful later runs do not close intermittent failures.
- Production FAQ publisher, atomic version-switch procedure, deployment stop deadline and activation approval remain separate from code integration.
