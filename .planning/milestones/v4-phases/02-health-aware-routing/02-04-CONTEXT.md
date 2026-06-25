# Phase 2.4: Provider Reliability and Error Contract - Context

**Gathered:** 2026-06-15
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.1 built health-aware multi-provider routing, Phase 2.2 verified the Go/SDK baseline, and Phase 2.3 added native Anthropic and Gemini adapters with OpenAI-compatible normalization.

Phase 2.4 should harden the provider layer now that OpenAI-compatible, Anthropic, and Gemini providers all exist. The goal is to make provider calls more predictable, diagnosable, and operationally safe without adding new platform systems. This phase should standardize provider error classification, improve provider health semantics, make readiness reflect adapter-level checks, and ensure all adapters honor the shared request/response contract consistently.

</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.4 is a provider reliability and contract-hardening phase, not a new feature-surface phase.
- **D-02:** The main deliverable is a stable provider error/health contract across OpenAI-compatible, Anthropic, and Gemini adapters.
- **D-03:** Do not add Redis, PostgreSQL, Admin API, streaming, retries, fallback chains, semantic cache, rate limiting, or cost governance in this phase.

### Provider Error Contract
- **D-04:** Introduce a structured provider error classification that all adapters use. Recommended categories:
  - `provider_auth_error`
  - `provider_rate_limit`
  - `provider_invalid_request`
  - `provider_invalid_model`
  - `provider_timeout`
  - `provider_unavailable`
  - `provider_bad_response`
  - `provider_error`
- **D-05:** Provider errors should carry enough metadata for routing, health, logs, and HTTP mapping without exposing secrets or raw prompts.
- **D-06:** HTTP handlers should not need provider-specific error branches. They should map structured gateway/provider errors consistently.
- **D-07:** Existing OpenAI-compatible adapter currently returns plain `fmt.Errorf` errors; Phase 2.4 should bring it up to the same structured error level as Anthropic/Gemini.

### Health Semantics
- **D-08:** Health updates should distinguish transient provider failures from client/request errors when practical.
- **D-09:** Provider auth/config errors should make the provider unroutable or clearly unhealthy/degraded.
- **D-10:** Provider invalid request errors should not necessarily poison provider health if the failure is caused by bad client input.
- **D-11:** Readiness should include adapter-level health check results where possible, but health checks should remain lightweight and should not require expensive model calls.
- **D-12:** Health state remains in-memory. Redis-backed health remains deferred.

### Request/Response Contract Consistency
- **D-13:** All adapters should honor common fields from `llm.LLMRequest` where supported:
  - `Model`
  - `Messages`
  - `Temperature`
  - `MaxTokens`
  - `RequestID` where useful for observability.
- **D-14:** OpenAI-compatible adapter should forward `temperature` and `max_tokens` just as native adapters do.
- **D-15:** All adapters should normalize empty/malformed upstream responses into structured `provider_bad_response` errors instead of panics, empty successes, or opaque decode errors.
- **D-16:** Success responses must remain OpenAI-compatible at `/v1/chat/completions`.

### Observability
- **D-17:** Logs and metrics should expose provider id, provider type, model, routing strategy, provider error category, provider health status, and latency.
- **D-18:** Do not introduce Prometheus/OpenTelemetry exporters yet. Extend the existing metrics/logging abstractions only as far as needed for stable internal signals and tests.
- **D-19:** Do not log API keys, auth headers, raw prompts, or raw provider response bodies.

### the agent's Discretion
The planner/executor may decide whether the structured provider error type lives in `internal/errors`, `internal/providers`, or a small new package. Prefer the placement that avoids import cycles and keeps adapters easy to test.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Context
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md` - Phase 2 health-aware routing decisions and deferred scope.
- `.planning/phases/02-health-aware-routing/02-01-PLAN.md` - Multi-provider routing and in-memory health implementation plan.
- `.planning/phases/02-health-aware-routing/02-02-CONTEXT.md` - Go baseline and SDK adoption context.
- `.planning/phases/02-health-aware-routing/02-02-UAT.md` - Phase 2.2 verification record.
- `.planning/phases/02-health-aware-routing/02-03-CONTEXT.md` - Native provider adapter decisions.
- `.planning/phases/02-health-aware-routing/02-03-PLAN.md` - Native Anthropic/Gemini adapter implementation plan.
- `.planning/phases/02-health-aware-routing/02-03-UAT.md` - Phase 2.3 verification record showing adapter coverage is now complete.

### Gateway Architecture
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` - Source architecture. Relevant sections: Provider Adapter System, Provider Health Tracking, Routing Engine, Observability & Telemetry, API Design.

### Current Code Integration Points
- `internal/errors/errors.go` - Existing structured gateway error type and routing errors.
- `internal/providers/adapter.go` - Provider adapter contract and current health status shape.
- `internal/providers/openai/adapter.go` - OpenAI-compatible adapter needing structured error and parameter-forwarding hardening.
- `internal/providers/anthropic/adapter.go` - Native Anthropic SDK adapter with provider error mapping.
- `internal/providers/gemini/adapter.go` - Native Gemini SDK adapter with provider error mapping.
- `internal/health/store.go` - In-memory health status and failure tracking.
- `internal/routing/router.go` - Health-aware provider selection and override behavior.
- `internal/gateway/service.go` - Provider call wrapper, health updates, metrics, and response metadata.
- `internal/http/handlers/chat.go` - Client-facing OpenAI-compatible chat endpoint and error mapping.
- `internal/http/handlers/health.go` - `/readyz` provider readiness summary.
- `internal/observability/metrics.go` - Existing metrics abstraction.
- `tests/integration/chat_test.go` - Multi-provider routing integration tests.
- `tests/integration/anthropic_test.go` - Anthropic integration coverage.
- `tests/integration/gemini_test.go` - Gemini integration coverage.
- `internal/providers/anthropic/adapter_test.go` - Anthropic adapter unit coverage.
- `internal/providers/gemini/adapter_test.go` - Gemini adapter unit coverage.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `errors.GatewayError`: Existing structured error envelope can be extended or reused for provider error categories.
- `health.Store`: Already records failures and success-driven recovery.
- `providers.ProviderAdapter`: Central adapter contract already works across OpenAI-compatible, Anthropic, and Gemini.
- `routing.HealthAwareRouter`: Already filters unhealthy providers and supports override behavior.
- Existing adapter tests: Anthropic/Gemini tests show a pattern for fake upstream/SDK HTTP behavior.

### Established Patterns
- Provider adapters normalize native outputs to `llm.LLMResponse`.
- HTTP handlers remain provider-agnostic and emit OpenAI-compatible success responses.
- Static config remains the control surface for this milestone.
- Health state is in-memory and updated around real provider calls.
- Tests use fake upstream servers rather than real external LLM providers.

### Integration Points
- Standardize adapter error returns so `gateway.Service` and `health.Store` can react consistently.
- Update OpenAI adapter to include `temperature` and `max_tokens` in outbound requests when provided.
- Update readiness to optionally consult `ProviderAdapter.HealthCheck` and summarize provider type/status without secrets.
- Add tests for OpenAI adapter error mapping and bad upstream response handling.
- Add tests proving provider invalid request errors do not incorrectly mark a provider unhealthy if that distinction is implemented.

</code_context>

<specifics>
## Specific Ideas

- The next stage should make the provider layer production-shaped before adding larger systems.
- The phase should close consistency gaps introduced by adding native adapters:
  - OpenAI-compatible adapter still uses plain errors and omits parameter forwarding.
  - Native adapters classify some errors, but there is not yet one shared provider error contract.
  - Readiness currently summarizes in-memory health but does not exercise adapter health semantics.

</specifics>

<must_build>
## Must Build In Phase 2.4

- Define or refine a shared provider error classification.
- Update OpenAI-compatible adapter to return structured provider errors.
- Ensure Anthropic and Gemini adapters use the shared categories consistently.
- Forward `Temperature` and `MaxTokens` from `LLMRequest` in the OpenAI-compatible adapter.
- Normalize empty/malformed upstream provider responses into structured errors.
- Improve readiness/provider health checks without adding expensive upstream calls.
- Add provider-type-aware, secret-safe logs/metrics hooks where current abstractions allow.
- Add tests for:
  - OpenAI adapter parameter forwarding.
  - OpenAI adapter non-2xx error mapping.
  - Anthropic/Gemini error category consistency.
  - malformed/empty provider responses.
  - readiness summary behavior across healthy/degraded/unhealthy provider states.
  - health impact of provider vs client/request errors if separated.
- Run `go test ./...`.

</must_build>

<deferred>
## Deferred Ideas

- SSE streaming support.
- Tool/function calling normalization.
- Multimodal input/output normalization.
- Embeddings APIs.
- Native Ollama adapter.
- Generic HTTP adapter.
- Retry/fallback-chain execution.
- Circuit breaker state machine.
- Redis-backed provider health.
- PostgreSQL-backed provider CRUD and Admin API.
- Rate limiting and admission queueing.
- Prometheus and OpenTelemetry exporters.
- Cost accounting and token usage aggregation.

</deferred>

<success_criteria>
## Success Criteria

- All provider adapters return consistent structured provider errors.
- OpenAI-compatible adapter forwards common generation parameters and is no longer the least-structured adapter.
- Provider health/readiness behavior is clearer and tested.
- Malformed upstream responses become controlled gateway/provider errors.
- Logs/metrics carry useful provider diagnostics without leaking secrets.
- `/v1/chat/completions` remains OpenAI-compatible for successful responses.
- Existing Anthropic/Gemini/OpenAI routing tests still pass.
- No Redis, PostgreSQL, Admin API, streaming, semantic cache, rate limiting, retry/fallback chain, or cost governance is introduced in this phase.
</success_criteria>

---

*Phase: 2.4-Provider Reliability and Error Contract*
*Context gathered: 2026-06-15*
