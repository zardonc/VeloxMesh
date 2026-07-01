# Phase 10: Advanced Routing & Observability - Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement the opt-in Composite Score Router for normalized multi-signal provider selection and add request-level OpenTelemetry/Prometheus observability for routing, latency, cache behavior, and provider outcomes. This phase clarifies routing quality and operator visibility only; BFF/Admin UI, multi-node coordination, PostgreSQL extension work, and full LimitRule unification remain deferred.

</domain>

<decisions>
## Implementation Decisions

### Composite Score Policy
- **D-01:** Add a balanced `composite-score` routing strategy that considers latency, pending requests, error rate, cost, and health status.
- **D-02:** Cost is only a tie-breaker between otherwise similar candidates. Reliability, speed, and load stay ahead of savings.
- **D-03:** Unhealthy providers are hard excluded, matching current router behavior. Degraded providers remain eligible but receive a score penalty.
- **D-04:** Use guarded z-score normalization only when candidate count and variance are sufficient. For tiny or flat candidate pools, fall back to min/max or neutral scoring.

### Cold Start and Fallback
- **D-05:** Use existing round-robin behavior until a provider/model has enough successful live request samples to be scored confidently.
- **D-06:** If every candidate's composite score is below the configured threshold, trigger the existing provider fallback chain before returning failure.
- **D-07:** Stale latency, error, and load metrics become neutral instead of helping or hurting. Health status still applies.
- **D-08:** "Warm enough" means a small minimum count of successful live requests per provider/model. Planners should choose a conservative default and make it configurable.

### Observability Surface
- **D-09:** Traces should capture the request lifecycle: routing decision, selected provider, strategy, score summary, TTFT, TPOT, E2E latency, cache hit/miss, provider outcome, and fallback reason where applicable.
- **D-10:** Prometheus labels must stay low-cardinality: `provider`, `model`, `strategy`, `status`, `cache_result`, and `error_category`. Do not use request, user, API-key, or raw identity labels.
- **D-11:** Score breakdown visibility is trace-attribute only for the selected provider. Do not emit per-candidate metrics by default.
- **D-12:** Apply strict sanitization to all observability data. Do not record raw prompts, auth headers, API keys, sensitive provider payloads, or user identifiers. Request IDs are allowed only when generated and safe.

### Configuration and Rollout
- **D-13:** Introduce composite routing as an opt-in `composite-score` strategy beside existing `round-robin` and `least-latency`; do not change default runtime behavior.
- **D-14:** Store routing strategy, weights, thresholds, warm-up count, stale window, preset name, and optional overrides in SQLite-backed routing config with hot reload.
- **D-15:** Expose a conservative default preset plus optional explicit overrides for advanced operators.
- **D-16:** Invalid or risky routing config must be rejected at validation. Runtime should retain the last known good routing config rather than clamping silently or accepting unsafe values.

### the agent's Discretion
- Keep the first implementation small: one opt-in strategy, one conservative preset, and the narrowest config schema needed to support safe overrides.
- Reuse `internal/routing.HealthAwareRouter`, `health.Store` snapshots, existing fallback behavior, and the current observability interface shape rather than introducing a parallel routing engine.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` — Phase 10 goal, milestone scope, and deferred Phase 11-13 boundaries.
- `.planning/PROJECT.md` — project constraints, core value, security/logging boundaries, and current v7.1 milestone focus.
- `.planning/REQUIREMENTS.md` — ROUT/OBS requirements for Phase 10.
- `.planning/phases/07-adapter-interfaces-sqlite-foundation/07-CONTEXT.md` — SQLite authoritative state, Redis hot-state boundary, Qdrant/degraded-path decisions.
- `.planning/phases/08-semantic-pipeline/08-CONTEXT.md` — no raw prompt/sensitive logging rule and gateway integration context.
- `.planning/phases/09-redis-stack-qdrant-fallback-integration/09-CONTEXT.md` — Redis hot-state/non-authoritative boundary, existing config reload direction, and fallback-chain context.

### Existing Code
- `internal/routing/router.go` — existing `HealthAwareRouter`, strategy switch, round-robin fallback, health filtering, combo routing, and `RoutingDecision`.
- `internal/health/store.go` — in-memory health snapshots with EWMA latency, pending requests, total successes/failures, and provider status.
- `internal/health/redis_store.go` — Redis-backed health snapshot behavior and local fallback.
- `internal/observability/metrics.go` — current metrics interface and stub implementation to extend or replace.
- `internal/controlstate/repository.go` — existing `RoutingRepository` seam for durable routing config.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `HealthAwareRouter.SelectExcluding` already centralizes provider eligibility, health filtering, route overrides, combo routing, and strategy selection.
- `health.ProviderSnapshot` already carries most scoring inputs: EWMA latency, pending requests, consecutive failures, total successes, total failures, probe state, and status.
- `RoutingRepository` already exists and should be the durable SQLite-backed place to extend routing config.
- `observability.Metrics` already has request outcome and routing strategy hooks, but it is currently stubbed and needs real Prometheus/OpenTelemetry implementations.

### Established Patterns
- Unknown or unhealthy providers are excluded from normal routing.
- Redis may accelerate hot health/config state but must not become durable truth.
- Gateway observability must avoid raw prompts, auth headers, API keys, and sensitive payloads.
- Provider-specific details stay behind adapters; routing should consume provider-neutral capabilities and health snapshots.

### Integration Points
- Composite scoring should attach inside the existing routing strategy switch, not bypass `Router` or provider registry behavior.
- Score config belongs in existing runtime/control-state loading and hot reload paths.
- Request lifecycle observability should connect through gateway service handling, streaming handling, semantic-cache hit/miss paths, provider outcome recording, and fallback execution.
- Prometheus histograms should be designed with low-cardinality labels only.

</code_context>

<specifics>
## Specific Ideas

- Conservative preset should optimize for stable production behavior over aggressive cost savings.
- Cost should only break near-ties; it should not cause a slow or unreliable provider to win.
- The selected-provider score summary can be attached to traces, but per-candidate score details should not become default metrics.
- Keep `round-robin` and `least-latency` available exactly as existing strategies during Phase 10 rollout.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within Phase 10 scope.

</deferred>

---

*Phase: 10-Advanced Routing & Observability*
*Context gathered: 2026-06-30*
