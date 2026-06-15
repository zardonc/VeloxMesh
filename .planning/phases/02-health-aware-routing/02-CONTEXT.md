# Phase 2 Context: Health-Aware Multi-Provider Routing

<domain>
Phase 1 completed the Go gateway walking skeleton: a Chi-based OpenAI-compatible data plane, static auth, request normalization, routing/admission boundaries, one OpenAI-compatible provider adapter, `/v1/chat/completions`, `/v1/models`, `/healthz`, and `/readyz`.

Phase 2 should turn the gateway from a single-provider proxy into a real gateway routing layer. The core capability is health-aware multi-provider routing with observable provider state, while keeping persistence, Redis, rate limiting, streaming, and admin APIs out of scope.
</domain>

<canonical_refs>
- `README.md` — current project setup and Phase 1 usage.
- `.planning/phases/01-gateway-walking-skeleton/01-CONTEXT.md` — locked Phase 1 decisions and deferred items.
- `.planning/phases/01-gateway-walking-skeleton/01-01-PLAN.md` — implemented Phase 1 package boundaries.
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` — source-of-truth gateway architecture.
- `internal/gateway/service.go` — current request orchestration.
- `internal/routing/router.go` — current static router extension point.
- `internal/providers/registry.go` — current provider registry.
- `internal/providers/adapter.go` — provider adapter contract.
- `internal/admission/controller.go` — current pass-through admission boundary.
- `internal/observability/metrics.go` — current metrics abstraction.
- `internal/http/handlers/models.go` — model aggregation endpoint.
</canonical_refs>

<decisions>

## D-01: Phase 2 main goal is multi-provider routing

The second phase should focus on routing value, not storage or admin surfaces. It should support multiple configured providers and select among them using provider health and lightweight routing strategies.

## D-02: Keep providers OpenAI-compatible in Phase 2

Phase 2 should support multiple OpenAI-compatible provider instances through config, such as OpenAI, OpenRouter, local vLLM, or Ollama-compatible OpenAI endpoints if they speak the same API shape.

Native Anthropic, Gemini, Ollama, and GenericHTTP adapters are deferred. The architecture supports them, but they should not be required for the second phase.

## D-03: Provider config remains static for now

Do not add PostgreSQL-backed provider CRUD or Admin API in Phase 2. Provider definitions should come from env/config file so the routing layer can be built and tested without database migration work.

## D-04: Add health tracking without Redis first

The architecture eventually stores provider health in Redis. Phase 2 should implement an in-memory `HealthStore` interface and use it to track:

- EWMA latency
- pending count
- error count/rate or consecutive failures
- health status: healthy, degraded, unhealthy
- last error / last checked timestamp

This keeps the data model aligned with the architecture while deferring Redis integration.

## D-05: Routing strategies for Phase 2 are round-robin and least-latency

Phase 2 should implement:

- `round-robin` across healthy providers
- `least-latency` using EWMA latency
- `override` via `X-Route-To`

Composite-score routing, cost-aware routing, heuristic YAML rules, and fallback-chain retries are deferred.

## D-06: Health must affect routing

The router should avoid providers marked unhealthy. If all providers are unhealthy, return a structured 503 or 502 gateway error instead of silently routing to a bad provider.

## D-07: Request result updates health state

Gateway service should update provider health after each provider call:

- increment pending before the call and decrement after completion
- update EWMA latency on success
- record failure on upstream/network error
- recover degraded/unhealthy state after successful checks or successful calls according to the chosen simple policy.

## D-08: Readiness should include provider availability

`GET /readyz` should report readiness based on config validity and provider availability. It can return a summarized provider health payload without exposing secrets.

## D-09: `/v1/models` should aggregate across all configured providers

Phase 1 already has `/v1/models`. Phase 2 should make it reflect all configured providers and include enough metadata to debug routing, while preserving OpenAI-compatible response shape.

## D-10: Metrics and logs should expose routing decisions

Phase 2 should add structured logs and metrics for:

- selected provider
- routing strategy
- route override
- provider health status
- EWMA latency
- pending count
- provider call outcome.

Prometheus and OpenTelemetry exporters remain deferred unless added as very thin adapters to the existing metrics interface.

## D-11: Prefer official provider implementations when useful

For multi-provider implementation, check whether the official provider SDK or maintained reference implementation already solves the required request/response mapping, auth headers, error parsing, model metadata, or streaming primitives.

The implementation may directly depend on an official SDK when the dependency is small, stable, and does not add unnecessary hot-path overhead. If the full SDK is too heavy or pulls unrelated features into the gateway, extract only the needed behavior by studying the official implementation and re-implementing the minimal compatible subset inside the adapter.

Do not hand-roll provider behavior before checking official SDK/reference behavior, and do not add a full SDK dependency simply for one trivial mapping if a small local adapter is clearer.

</decisions>

<must_build>

## 1. Multi-provider static configuration

- Replace single `OPENAI_PRIMARY_*`-only config with a multi-provider config shape.
- Support at least two provider definitions in tests.
- Keep one-provider configs backward compatible if practical.
- Validate duplicate provider IDs, missing base URLs, missing models, and invalid default provider.

## 2. Provider registry expansion

- Register multiple OpenAI-compatible adapters.
- Expose provider lookup, default provider lookup, all provider IDs, all models, and provider health metadata hooks.
- Preserve `X-Route-To` behavior.

## 3. In-memory provider health store

- Add `internal/health` or similar package.
- Track EWMA latency, pending count, failure count, and status.
- Make the store concurrency-safe.
- Keep an interface so Redis can replace or back it later.

## 4. Health-aware routing strategies

- Implement `round-robin`.
- Implement `least-latency`.
- Route only to healthy/degraded providers unless override explicitly requests a known provider.
- Define override behavior clearly: either allow override to degraded providers but reject unhealthy providers, or allow override with an explicit warning header. Recommended: reject unhealthy override unless a later admin/debug bypass is added.

## 5. Gateway health updates

- Update health before and after provider calls.
- Ensure pending counts are decremented on all paths.
- Record latency and failures without logging secrets or raw prompts.

## 6. Readiness and models improvements

- `/readyz` should include provider readiness summary.
- `/v1/models` should aggregate configured models across all providers.
- Optional but useful: include non-OpenAI debug metadata only in logs or readiness, not in the OpenAI-compatible models response.

## 7. Tests

- Unit tests for config parsing.
- Unit tests for health store concurrency behavior.
- Unit tests for round-robin and least-latency selection.
- Integration tests with two `httptest.Server` upstream providers.
- Failure tests proving unhealthy provider avoidance.
- Tests proving `X-Route-To` still works.

</must_build>

<deferred>
- PostgreSQL provider table and migrations.
- Redis-backed health state.
- Admin API provider CRUD.
- Circuit breaker state machine.
- Retry and fallback-chain execution.
- Composite-score routing.
- Cost-aware routing.
- YAML heuristic pre-routing rules.
- Rate limiting.
- Full admission control queues.
- SSE streaming proxy.
- Semantic cache.
- Native Anthropic/Gemini/Ollama adapters.
- OpenTelemetry and Prometheus exporters.
- Usage/cost aggregation.
- Docker Compose stack.
</deferred>

<gray_areas_for_planning>

## Provider config format

Recommended default: use a small YAML config file for provider list plus env vars for secrets. Environment-only multi-provider config becomes awkward quickly. The planner should choose the simplest implementation that works on Windows and in CI.

## Official SDK reuse policy

Before implementing each provider adapter or provider-specific mapping, evaluate the official SDK/reference implementation:

- Direct SDK dependency is acceptable when it materially reduces correctness risk and stays lightweight.
- Partial extraction is preferred when the SDK is large, slow to initialize, hides too much transport behavior, or conflicts with the gateway's low-latency hot path.
- Local reimplementation is acceptable for simple OpenAI-compatible adapters, but it should still be checked against official request/response/error behavior.

## Health status thresholds

Recommended default:

- healthy: no recent failures
- degraded: one or more recent failures but still routable
- unhealthy: consecutive failures exceed threshold or health check fails

Keep thresholds configurable but static in Phase 2.

## Strategy default

Recommended default: `least-latency` when there are latency samples, with round-robin fallback during cold start.

## Override semantics

Recommended default: `X-Route-To` can select a known healthy/degraded provider, but cannot force an unhealthy provider.

</gray_areas_for_planning>

<success_criteria>
- Gateway can be configured with multiple OpenAI-compatible providers.
- `/v1/chat/completions` can route to different providers without handler changes.
- Router avoids unhealthy providers.
- Provider health changes after successful and failed calls.
- `/readyz` reflects provider readiness.
- `/v1/models` aggregates models across providers.
- Tests prove routing behavior with at least two fake upstream providers.
- No PostgreSQL, Redis, Admin API, streaming, semantic cache, or cost governance is introduced in Phase 2.
</success_criteria>
