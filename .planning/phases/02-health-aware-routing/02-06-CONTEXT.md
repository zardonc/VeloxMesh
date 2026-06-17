# Phase 2.6: Active Provider Health Probing and Recovery - Context

**Gathered:** 2026-06-16
**Status:** Ready for planning; ROADMAP should be updated to include Phase 2.6 as the next provider-foundation step

<domain>
## Phase Boundary

Phase 2.6 should add an active provider health probing and recovery loop on top of the existing in-memory health-aware routing layer.

Phase 2.1 introduced multi-provider health-aware routing, Phase 2.3 added native Anthropic and Gemini adapters, Phase 2.4 standardized provider error categories and health impact, and Phase 2.5 added request-level fallback execution. The remaining reliability gap is that provider health is currently updated only by live traffic. Once a provider becomes unhealthy, the gateway has no active background mechanism to re-check it, recover it, or make readiness reflect probe freshness.

This phase should keep the implementation small and in-process. It should not introduce Redis, PostgreSQL, Admin API, runtime health policy CRUD, Prometheus/OpenTelemetry exporters, full circuit breaker state machines, streaming retry, hedged requests, rate limiting, cost governance, or semantic cache.

</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.6 should implement active provider health probing and recovery, not a broad observability or platform-management phase.
- **D-02:** The goal is to let unhealthy/degraded providers recover without waiting for user traffic to hit them again.
- **D-03:** The implementation should remain an in-process background service using existing provider adapters and the existing in-memory `health.Store`.
- **D-04:** Keep the existing request-path health updates from Phase 2.5. Active probes should complement live traffic updates, not replace them.

### Probe Behavior
- **D-05:** Add a lightweight health prober that periodically calls each adapter's `HealthCheck(ctx)` method.
- **D-06:** Health-check behavior must be extracted into explicit configuration structs and JSON config fields so a later Admin Console/Admin API can call the same prober boundary without reworking internals.
- **D-07:** Health probes must use short timeouts and must not perform expensive generation/model calls. Each adapter's `HealthCheck` should remain cheap and secret-safe.
- **D-08:** Probe failures should affect health state enough to keep broken providers out of routing, but should not log secrets, raw prompts, auth headers, or raw provider bodies.
- **D-09:** Probe successes should be able to recover degraded/unhealthy providers by clearing consecutive failures through the existing health-store success path or a small explicit probe-result method.

### Health Check Configuration
- **D-10:** Add a top-level health-check config shape, for example `health_check` or `health_probe`, with fields such as:
  - `enabled`
  - `interval`
  - `timeout`
  - `initial_delay`
  - `failure_threshold`
  - `success_threshold`
  - `stale_after`
  - `max_concurrency`
- **D-11:** Add optional per-provider override fields only where they are immediately useful, for example `health_check_enabled`, `health_check_interval`, and `health_check_timeout` on provider config. Do not add dynamic policy storage in this phase.
- **D-12:** Defaults should be conservative and safe for local development:
  - enabled when more than one provider is configured.
  - interval around 30 seconds.
  - timeout around 2 seconds.
  - failure threshold aligned with the existing unhealthy threshold unless explicitly configured.
  - success threshold default 1 for simple recovery.
- **D-13:** The prober should expose a deterministic internal method such as `ProbeOnce(ctx)` and preferably `ProbeProvider(ctx, providerID)` so a future Admin Console can trigger checks through a thin API layer without duplicating probe logic.
- **D-14:** Config parsing should preserve backwards compatibility: existing configs without health-check fields should continue to load and behave sensibly.

### Health State Semantics
- **D-15:** Preserve the current simple health statuses: `healthy`, `degraded`, and `unhealthy`.
- **D-16:** Do not build a full Closed/Open/HalfOpen circuit breaker in this phase. If a minimal "probe can recover unhealthy provider" path resembles half-open behavior internally, keep it as health-store semantics rather than a new breaker abstraction.
- **D-17:** Add probe metadata to health snapshots only if needed for readiness/debugging, such as `last_probe_at`, `last_probe_error`, `last_probe_success`, or `last_probe_duration`.
- **D-18:** Unknown providers and providers with stale or failing probes should remain non-routable only when their current health status is unhealthy. Do not make cold-start readiness fail just because no probe has run yet unless config explicitly requires probes.

### Readiness and Visibility
- **D-19:** `/readyz` should expose a secret-safe provider readiness summary that includes active-probe information when available.
- **D-20:** Readiness should still return 200 when at least one provider is healthy or degraded and routeable.
- **D-21:** Readiness should return 503 when no provider is healthy or degraded.
- **D-22:** Logs/metrics should record probe outcomes at the existing abstraction level where practical, but dedicated Prometheus/OpenTelemetry exporters remain deferred.

### Lifecycle and App Wiring
- **D-23:** The prober should start when the app starts and stop cleanly when its context is canceled.
- **D-24:** Avoid goroutine leaks in tests by giving the prober an explicit `Start(ctx)`/`Stop` or context-driven lifecycle.
- **D-25:** Tests should use fake adapters or fake upstream servers and must not call real LLM providers.

### the agent's Discretion
The planner/executor may choose whether to place the prober under `internal/health`, `internal/gateway`, or a small new package such as `internal/health/prober.go`. Prefer the placement that avoids import cycles and keeps provider adapters independent from routing internals.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Context
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md` — Phase 2 routing, health, and deferred scope.
- `.planning/phases/02-health-aware-routing/02-04-CONTEXT.md` — Provider reliability/error-contract decisions.
- `.planning/phases/02-health-aware-routing/02-04-UAT.md` — Verification of shared provider error and health behavior.
- `.planning/phases/02-health-aware-routing/02-05-CONTEXT.md` — Provider retry/fallback execution decisions.
- `.planning/phases/02-health-aware-routing/02-05-SUMMARY.md` — Completed fallback behavior summary.
- `.planning/phases/02-health-aware-routing/02-05-UAT.md` — Confirms Phase 2.5 passes 12/12 checks with no gaps.

### Gateway Architecture
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` — Source architecture. Relevant sections: Provider Health Tracking, Routing Engine, Request Processing Pipeline, Observability & Telemetry.

### Current Code Integration Points
- `internal/health/store.go` — In-memory health snapshots and failure/success counters.
- `internal/providers/adapter.go` — Existing `HealthCheck(ctx)` contract.
- `internal/providers/openai/adapter.go` — OpenAI-compatible adapter health-check behavior.
- `internal/providers/anthropic/adapter.go` — Anthropic adapter health-check behavior.
- `internal/providers/gemini/adapter.go` — Gemini adapter health-check behavior.
- `internal/providers/registry.go` — Provider listing needed by a prober.
- `internal/routing/router.go` — Routing excludes providers with unhealthy snapshots.
- `internal/gateway/service.go` — Live request-path health updates and fallback attempts.
- `internal/http/handlers/health.go` — `/readyz` response and provider readiness summary.
- `internal/app/app.go` — App wiring location for starting an in-process prober.
- `internal/config/config.go` — Static config location for probe settings.
- `internal/observability/metrics.go` — Existing metrics abstraction if probe outcome recording is added.
- `tests/integration/health_test.go` — Existing health/readiness integration patterns.
- `internal/health/store_test.go` — Existing health-store unit coverage.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `providers.ProviderAdapter.HealthCheck(ctx)` already exists and returns a lightweight `providers.HealthStatus`.
- `health.Store` already tracks success/failure counts, EWMA latency, pending requests, and healthy/degraded/unhealthy status.
- `routing.HealthAwareRouter` already avoids providers whose snapshot status is `unhealthy`.
- `gateway.Service` already records health updates for every live provider attempt.
- `/readyz` already summarizes healthy/degraded/unhealthy provider counts from `HealthStore`.

### Established Patterns
- Health state is in-memory for Phase 2.x.
- Static JSON config is the current control surface.
- Provider adapters own provider-specific upstream behavior.
- HTTP handlers should stay provider-agnostic and secret-safe.
- Tests should use fake providers and local `httptest.Server` instances, never real provider credentials.

### Integration Points
- Add health probe config to `internal/config/config.go`.
- Extract health-check parameters into named config structs rather than loose fields so the shape can later back an Admin Console/Admin API request/response contract.
- Add a prober service that iterates over `providers.Registry.List()` or receives adapters directly.
- Update `health.Store` only if current `EndRequest` semantics are insufficient for probe recovery metadata.
- Wire prober startup in `internal/app/app.go` without changing request handlers.
- Extend `/readyz` to include probe freshness/status when the store exposes it.
- Add unit tests for probe success, probe failure, recovery from unhealthy, timeout handling, and lifecycle cancellation.
- Add integration tests proving routing can recover a previously unhealthy provider after a successful probe.

</code_context>

<specifics>
## Specific Ideas

- User selected direction **1: active health probing** for Phase 2.6.
- Recommended phase name: **Active Provider Health Probing and Recovery**.
- Recommended default behavior:
  - probes enabled by default when more than one provider is configured.
  - interval default around 30 seconds.
  - timeout default around 2 seconds.
  - health-check parameters live in config structs and are suitable for later Admin Console invocation.
  - tests should use short manually-triggered probe runs instead of sleeping for real intervals.
- The planner should prefer a deterministic `ProbeOnce(ctx)` API for tests plus a ticker-based `Start(ctx)` loop for runtime.
- The planner should also prefer `ProbeProvider(ctx, providerID)` if it can be added cleanly; this gives the future Admin Console a direct "check this provider now" boundary without adding the console in this phase.

</specifics>

<must_build>
## Must Build In Phase 2.6

- Static health probe config with conservative defaults and validation, extracted into reusable config structs.
- Optional per-provider health-check overrides that do not require runtime policy storage.
- In-process provider health prober with `ProbeOnce(ctx)` and context-driven background loop.
- Internal single-provider probe entry point suitable for future Admin Console/Admin API use.
- Integration with existing `providers.ProviderAdapter.HealthCheck(ctx)`.
- Health-store updates for probe success/failure so unhealthy providers can recover.
- Secret-safe readiness output that includes probe state/freshness if implemented.
- App startup wiring for the prober without goroutine leaks.
- Tests for probe success, probe failure, timeout/cancellation, unhealthy-provider recovery, readiness summary, and routing after recovery.
- `gofmt -l .`, `go vet ./...`, and `go test ./...`.

</must_build>

<deferred>
## Deferred Ideas

- Redis-backed shared provider health.
- PostgreSQL provider state or Admin API provider management.
- Runtime health-policy CRUD or hot reload.
- Full circuit breaker state machine with explicit Closed/Open/HalfOpen states.
- Prometheus `/metrics` endpoint or OpenTelemetry exporters.
- Synthetic chat/model generation probes.
- Provider model discovery from live upstream APIs.
- Streaming retry/replay handling.
- Hedged requests or speculative parallel provider calls.
- Rate limiting, admission queues, semantic cache, and cost governance.

</deferred>

<success_criteria>
## Success Criteria

- The gateway periodically probes configured providers using cheap adapter health checks.
- Health-check parameters are configurable and structured enough to be reused by a future Admin Console/Admin API.
- A provider marked unhealthy by live traffic can recover after successful health probes.
- Routing automatically includes recovered providers once their health state becomes routeable.
- `/readyz` remains secret-safe and accurately reflects whether any provider is routeable.
- Probe lifecycle is context-bound and does not leak goroutines in tests.
- Tests prove probe-driven failure, recovery, readiness, and routing behavior without real LLM provider calls.
- No Redis, PostgreSQL, Admin API, full circuit breaker, streaming retry, Prometheus/OpenTelemetry exporter, rate limiting, semantic cache, or cost governance is introduced in this phase.
</success_criteria>

---

*Phase: 2.6-Active Provider Health Probing and Recovery*
*Context gathered: 2026-06-16*
