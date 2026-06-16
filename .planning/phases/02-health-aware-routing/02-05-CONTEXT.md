# Phase 2.5: Provider Retry and Fallback Execution - Context

**Gathered:** 2026-06-15
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.5 should add a small, deterministic retry and fallback execution layer on top of the now-stable provider routing and error contract.

Phase 2.1 introduced health-aware multi-provider routing, Phase 2.3 added native Anthropic/Gemini adapters, and Phase 2.4 standardized provider error categories and health impact. The next useful slice is to let a single client request survive transient provider failure by trying another eligible provider when it is safe to do so.

This phase is not a broad resilience platform. It should not introduce Redis, PostgreSQL, Admin API, circuit breaker state machines, streaming retries, semantic cache, rate limiting, cost governance, or model degradation. It should build the smallest production-shaped fallback path for non-streaming `/v1/chat/completions`.

</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.5 should implement provider retry/fallback execution for non-streaming chat completions.
- **D-02:** The goal is request-level availability improvement: when one provider fails with a retryable provider error, the gateway should attempt another eligible provider before returning an error to the client.
- **D-03:** Keep this as an in-process data-plane feature using existing provider registry, router, health store, and structured provider errors.
- **D-04:** Do not implement a full circuit breaker state machine in this phase. The existing health store can still mark providers degraded/unhealthy; full Closed/Open/HalfOpen breaker behavior remains deferred.

### Retry Eligibility
- **D-05:** Retry only non-streaming requests. Streaming retry is deferred because partial SSE output cannot be safely replayed once bytes have reached the client.
- **D-06:** Retry only provider-side or transient categories from the Phase 2.4 contract:
  - `provider_rate_limit`
  - `provider_timeout`
  - `provider_unavailable`
  - `provider_bad_response`
  - `provider_error`
- **D-07:** Do not retry client/request/config categories:
  - `provider_invalid_request`
  - `provider_invalid_model`
  - `provider_auth_error`
  These should fail fast because retrying another provider is likely incorrect, unsafe, or hides configuration problems.
- **D-08:** Context cancellation and request deadline expiry should stop further attempts immediately.

### Fallback Provider Selection
- **D-09:** The fallback path should never retry the same provider twice for a single client request.
- **D-10:** Fallback candidates should come from the existing router/registry and must respect health state: healthy/degraded providers are eligible; unhealthy providers are not.
- **D-11:** Preserve `X-Route-To` semantics: if the client explicitly routes to a provider, do not silently fallback to another provider unless a future explicit opt-in header is introduced. In Phase 2.5, provider override remains strict.
- **D-12:** Prefer minimal router/API changes. If needed, add a method that selects while excluding previously attempted providers, or add a request-scoped exclusion field internal to routing. Avoid duplicating routing strategy logic inside `gateway.Service`.
- **D-13:** Fallback should preserve the original requested model. Do not perform model substitution or degradation in this phase.

### Attempt Limits and Backoff
- **D-14:** Add static config for retry/fallback behavior:
  - enabled/disabled, default enabled only when more than one provider is configured.
  - max attempts, default 2 total attempts.
  - optional small backoff or none by default; avoid adding sleeps that make tests flaky or increase latency unexpectedly.
- **D-15:** The max attempts should be bounded by the number of eligible providers. A request should not fan out across all providers unless configured and tested.
- **D-16:** Avoid automatic retry on the same provider in this phase. Same-provider retry can be added later once provider-specific idempotency and backoff policy are more mature.

### Response and Observability Contract
- **D-17:** Successful fallback responses should remain OpenAI-compatible and should expose the final provider via existing `X-Provider`.
- **D-18:** Add safe response/debug headers where useful:
  - `X-Retry-Attempts` or `X-Provider-Attempts`
  - optionally `X-Fallback-Used: true|false`
  Do not expose raw upstream errors or prompts.
- **D-19:** Metrics/logging should record each provider attempt and the final outcome, including provider id, model, strategy, error category, attempt index, and whether fallback was used.
- **D-20:** Health updates should occur per attempted provider. A failed first provider should record failure; the final successful provider should record success.

### the agent's Discretion
The planner/executor may decide the cleanest internal type for attempt history, for example `gateway.Attempt`, `routing.AttemptResult`, or fields on `llm.LLMResponse`. Prefer the option that keeps HTTP handlers provider-agnostic and avoids leaking routing internals into adapter packages.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Context
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md` — Phase 2 routing, health, and deferred scope.
- `.planning/phases/02-health-aware-routing/02-04-CONTEXT.md` — Provider reliability/error-contract decisions that retry logic depends on.
- `.planning/phases/02-health-aware-routing/02-04-PLAN.md` — Structured provider error and health semantics implementation plan.
- `.planning/phases/02-health-aware-routing/02-04-UAT.md` — Phase 2.4 verification record; confirms shared provider error contract and tests are passing.

### Gateway Architecture
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` — Source architecture. Relevant sections: Request Processing Pipeline, Routing Engine, Fallback Behavior, Provider Health Tracking, Admission Control, Observability & Telemetry.

### Current Code Integration Points
- `internal/errors/errors.go` — Shared provider error categories and `AffectsProviderHealth`.
- `internal/gateway/service.go` — Current single-attempt provider call wrapper, health updates, and metrics recording.
- `internal/routing/router.go` — Health-aware provider selection and override behavior.
- `internal/providers/registry.go` — Provider lookup/listing needed for fallback candidate selection.
- `internal/health/store.go` — In-memory provider health snapshots and failure tracking.
- `internal/http/handlers/chat.go` — OpenAI-compatible chat endpoint and response headers.
- `internal/observability/metrics.go` — Existing metrics abstraction to extend for attempts/fallback.
- `internal/config/config.go` — Static config location for retry/fallback settings.
- `tests/integration/chat_test.go` — Multi-provider request integration patterns.
- `tests/integration/health_test.go` — Health transition patterns with fake providers.
- `internal/providers/openai/adapter_test.go` — Provider error category test examples.
- `internal/providers/anthropic/adapter_test.go` — Native provider error category test examples.
- `internal/providers/gemini/adapter_test.go` — Native provider error category test examples.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `errors.GatewayError` and provider category constants provide the retryability signal.
- `errors.AffectsProviderHealth` already separates health-impacting provider errors from client invalid requests.
- `routing.HealthAwareRouter` already filters unhealthy providers and supports least-latency/round-robin.
- `providers.Registry.List()` can provide candidate providers if router needs exclusion-aware selection.
- `gateway.Service.HandleChatCompletion` is the natural place to orchestrate attempts because it already owns routing, admission, provider calls, health updates, and metrics.
- Existing integration tests use `httptest.Server` fake providers and can be extended to simulate first-provider failure followed by second-provider success.

### Established Patterns
- HTTP handlers remain provider-agnostic.
- Provider adapters normalize native responses to `llm.LLMResponse`.
- Static config is the control surface for Phase 2.x work.
- Health updates happen around real provider calls.
- Tests must not call real external LLM providers.
- Deferred systems are kept out until explicitly scoped.

### Integration Points
- Add retry/fallback configuration in `internal/config/config.go`.
- Extend routing with an exclusion-aware selection path or a clean request-scoped candidate filter.
- Update `internal/gateway/service.go` to loop over attempts while preserving admission, health, and metrics correctness.
- Update `internal/http/handlers/chat.go` to expose safe attempt/fallback headers if attempt metadata is returned by service.
- Extend `internal/observability/metrics.go` with attempt/fallback recording hooks.
- Add unit tests for retryability policy and integration tests for fallback success/failure.

</code_context>

<specifics>
## Specific Ideas

- Recommended phase name: **Provider Retry and Fallback Execution**.
- Recommended first implementation target: non-streaming `/v1/chat/completions` only.
- Recommended default behavior:
  - no `X-Route-To`: allow fallback across healthy/degraded providers.
  - with `X-Route-To`: strict override, no silent fallback.
  - max attempts: 2 total.
  - retryable errors: rate limit, timeout, unavailable, bad response, generic provider error.
  - non-retryable errors: auth, invalid request, invalid model.
- Recommended tests:
  - first provider 500s, second succeeds.
  - first provider returns 429, second succeeds.
  - invalid request does not fallback.
  - auth error does not fallback.
  - `X-Route-To` failure does not fallback.
  - all eligible providers fail returns the last/aggregated structured error.
  - each attempted provider receives correct health update.

</specifics>

<must_build>
## Must Build In Phase 2.5

- Retryability policy based on Phase 2.4 provider error categories.
- Static retry/fallback config with conservative defaults.
- Request-scoped fallback execution for non-streaming chat completions.
- Exclusion-aware provider selection so one request does not hit the same provider repeatedly.
- Strict `X-Route-To` behavior with no silent fallback.
- Per-attempt health updates and provider diagnostics.
- Safe response headers/metadata showing attempt count and fallback use.
- Unit and integration tests for successful fallback, non-retryable errors, override behavior, all-failed behavior, and health updates.
- Run `gofmt`, `go vet ./...`, and `go test ./...`.

</must_build>

<deferred>
## Deferred Ideas

- Full circuit breaker state machine with Closed/Open/HalfOpen probes.
- Redis-backed health and distributed breaker state.
- Admin API/runtime-configurable retry policies.
- Same-provider retry with exponential backoff and jitter.
- Streaming/SSE retry and replay handling.
- Hedged requests/speculative parallel provider calls.
- Model degradation or automatic model substitution.
- Cost-aware fallback selection.
- Composite-score routing.
- Rate limiting and admission queues.
- Semantic cache.
- Provider-specific idempotency keys.

</deferred>

<success_criteria>
## Success Criteria

- A non-streaming chat request can fallback from a retryable failed provider to another healthy/degraded provider and return an OpenAI-compatible success response.
- Non-retryable provider errors fail fast and do not mask bad client input or provider configuration.
- `X-Route-To` remains strict and does not silently route to a different provider.
- Attempted providers are not repeated within one request.
- Health state is updated for every attempted provider.
- Response headers or metadata let operators know whether fallback occurred without leaking raw upstream details.
- Tests prove fallback success, fallback exhaustion, non-retryable behavior, override behavior, and health updates.
- No Redis, PostgreSQL, Admin API, streaming retry, semantic cache, rate limiting, cost governance, or full circuit breaker is introduced in this phase.
</success_criteria>

---

*Phase: 2.5-Provider Retry and Fallback Execution*
*Context gathered: 2026-06-15*
