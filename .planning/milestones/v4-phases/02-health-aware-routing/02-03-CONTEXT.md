# Phase 2.3: Native Anthropic and Gemini Provider Adapters - Context

**Gathered:** 2026-06-15
**Status:** Ready for planning after Phase 2.2 verification

<domain>
## Phase Boundary

Phase 2.3 extends the Phase 2.1 health-aware multi-provider routing system with native Anthropic and Gemini provider adapters. Phase 2.2 exists first to confirm the Go version/toolchain baseline needed for official SDK adoption.

This phase is complete when the gateway can accept the existing `/v1/chat/completions` OpenAI-compatible request, route to `anthropic` or `gemini` providers through provider-specific adapters, translate requests into each provider's native API shape, normalize provider responses back into `llm.LLMResponse`, and return the same OpenAI-compatible response body that downstream clients already consume.

</domain>

<decisions>
## Implementation Decisions

### Provider Adapter Scope
- **D-01:** Add first-class provider types `anthropic` and `gemini` to static provider config. Do not treat them as OpenAI-compatible endpoints.
- **D-02:** Implement provider-specific adapters under dedicated packages:
  - `internal/providers/anthropic`
  - `internal/providers/gemini`
- **D-03:** The existing `providers.ProviderAdapter` contract remains the integration boundary. `Complete(ctx, *llm.LLMRequest) (*llm.LLMResponse, error)` must return normalized internal output regardless of provider-native response shape.
- **D-04:** Health-aware routing, registry lookup, readiness, `/v1/models`, `X-Route-To`, metrics, and gateway service orchestration should continue to work without special handler logic for Anthropic or Gemini.

### Anthropic SDK Preference
- **D-05:** Use Anthropic's official Go SDK for the Anthropic adapter where practical.
- **D-06:** Phase 2.2 must verify the Go baseline first. Current `go.mod` declares `go 1.26.1`, satisfying the Anthropic SDK's documented Go 1.24+ requirement.
- **D-07:** Do not fall back to a hand-written Anthropic HTTP adapter unless implementation discovers a concrete blocker. If a blocker appears, document the evidence and ask before changing direction.
- **D-08:** Keep the SDK contained inside the Anthropic adapter package. Do not let SDK-native types leak into `internal/llm`, routing, gateway service, or HTTP handlers.

### Gemini SDK Policy
- **D-09:** Evaluate the official Google Gen AI Go SDK, `google.golang.org/genai`, for the Gemini adapter.
- **D-10:** Direct Gemini SDK usage is acceptable if it keeps mapping correct and does not hide gateway-critical behavior. If local REST mapping is simpler and more transparent, that is still acceptable for Gemini.
- **D-11:** Regardless of SDK choice, Gemini adapter output must normalize into the same internal `llm.LLMResponse` shape.

### OpenAI-Compatible Normalization
- **D-12:** This phase must implement normalization of LLM provider outputs. This is required because downstream clients only handle OpenAI-compatible API responses.
- **D-13:** The HTTP handler must continue to emit OpenAI-compatible `ChatCompletionResponse`. Provider adapters must normalize native Anthropic/Gemini responses into the existing internal `llm.LLMResponse` / `llm.Choice` shape before data returns to the handler.
- **D-14:** Initial normalization scope is text-only, non-streaming chat completions:
  - one assistant message per provider response
  - OpenAI-compatible `choices[].index`
  - OpenAI-compatible `choices[].message.role = "assistant"`
  - OpenAI-compatible `choices[].message.content`
  - OpenAI-compatible `choices[].finish_reason`
- **D-15:** Finish reason mapping must be explicit and tested. Recommended baseline:
  - Anthropic `end_turn` -> `stop`
  - Anthropic `max_tokens` -> `length`
  - Anthropic `stop_sequence` -> `stop`
  - Gemini normal stop/stop sequence -> `stop`
  - Gemini max token exhaustion -> `length`
  - provider safety/block reasons -> provider-specific gateway error or a conservative `content_filter` only if represented safely in the current type model.
- **D-16:** Usage/token normalization is optional in this phase because `llm.LLMResponse` does not currently expose usage fields. If minimal provider usage metadata is easy to capture, add internal fields only when it does not expand the public contract or require cost accounting.

### Request Mapping
- **D-17:** `llm.LLMRequest` remains OpenAI-compatible at the gateway boundary. Each native adapter owns translation from OpenAI-style messages into provider-native request payloads or SDK-native request types.
- **D-18:** Anthropic request mapping should convert:
  - OpenAI `system` messages into Anthropic system content.
  - OpenAI `user` and `assistant` messages into Anthropic Messages API message params.
  - OpenAI `max_tokens`, once carried through `LLMRequest`, into Anthropic `max_tokens`.
  - OpenAI model name into the provider-configured or requested Anthropic model.
- **D-19:** Gemini request mapping should convert OpenAI messages into Gemini `contents` / `parts` with role mapping that preserves conversation order. System instruction handling should follow the current official Gemini API/SDK behavior verified during implementation.
- **D-20:** Existing `LLMRequest` currently drops `Temperature` and `MaxTokens` from `ChatCompletionRequest`; the planner should include a small internal type extension so native adapters can forward supported generation parameters. Keep this focused and backwards-compatible.

### Error Handling
- **D-21:** Provider-native errors must be mapped into structured gateway/provider errors without leaking API keys, request bodies, raw prompts, or sensitive provider payloads.
- **D-22:** Anthropic SDK errors should be unwrapped or classified enough to distinguish authentication, rate limit, invalid model/request, and transient upstream failures where the SDK exposes that information.
- **D-23:** Provider safety blocks, invalid model errors, authentication errors, rate limits, and transient upstream failures should be distinguishable enough for tests and future retry/fallback phases, but this phase does not need full retry execution.
- **D-24:** The gateway should still return OpenAI-compatible success responses. Error bodies may continue using the existing gateway structured error format unless a later phase explicitly standardizes OpenAI-compatible error envelopes.

### the agent's Discretion
The planner/executor may choose the exact helper placement for mapping and normalization. Acceptable options include provider-local mapping helpers or a small shared `internal/llm` normalization helper. Prefer provider-local helpers if sharing would create premature abstraction. Anthropic SDK-specific helpers should stay provider-local.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Context
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md` - Phase 2 locked decisions for health-aware routing, static provider config, registry behavior, and deferred scope.
- `.planning/phases/02-health-aware-routing/02-01-PLAN.md` - Completed Phase 2.1 implementation plan and package boundaries.
- `.planning/phases/02-health-aware-routing/02-02-CONTEXT.md` - Go baseline phase that must be completed before adopting Anthropic official SDK.

### Gateway Architecture
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` - Source architecture. Relevant sections: Provider Adapter System, Request Processing Pipeline, API Design, Provider Health Tracking.

### Current Code Integration Points
- `internal/config/config.go` - Provider config currently validates only `openai-compatible`; Phase 2.3 must add `anthropic` and `gemini`.
- `internal/app/app.go` - Adapter construction currently wires only OpenAI-compatible adapters.
- `internal/providers/adapter.go` - Provider adapter interface used by routing and gateway service.
- `internal/providers/openai/adapter.go` - Existing adapter pattern and OpenAI-compatible response decode.
- `internal/providers/registry.go` - Multi-provider registry that should accept native adapters without handler changes.
- `internal/routing/router.go` - Health-aware routing must remain provider-type agnostic.
- `internal/gateway/service.go` - Provider call, health updates, and response metadata wiring.
- `internal/llm/types.go` - Internal normalized request/response types and OpenAI-compatible response shape.
- `internal/http/handlers/chat.go` - Client-facing `/v1/chat/completions` handler; should remain OpenAI-compatible.
- `internal/errors/errors.go` - Structured gateway error pattern.
- `go.mod` - Go baseline must satisfy Anthropic official SDK requirements before this phase starts.

### Provider References
- `https://github.com/anthropics/anthropic-sdk-go` - Official Anthropic Go SDK reference. Current README requires Go 1.24+.
- `https://platform.claude.com/docs/en/api/messages` - Anthropic Messages API reference for request/response shape and content blocks.
- `https://github.com/googleapis/go-genai` - Official Google Gen AI Go SDK repository.
- `https://pkg.go.dev/google.golang.org/genai` - Go package documentation for Gemini SDK.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `providers.ProviderAdapter`: The central extension point. Anthropic and Gemini adapters should implement this interface.
- `llm.LLMRequest`: Existing internal request boundary for normalized OpenAI-compatible input.
- `llm.LLMResponse` and `llm.Choice`: Existing internal response boundary that the HTTP handler converts into OpenAI-compatible JSON.
- `health.Store`: Existing health tracking remains per provider ID and does not need provider-type awareness.
- `routing.HealthAwareRouter`: Already selects adapters without caring about concrete adapter type.

### Established Patterns
- Adapter construction happens during app startup from static config.
- Provider adapters own outbound provider behavior and return normalized internal LLM responses.
- Gateway service owns health updates and metadata attachment after provider call.
- HTTP handlers should validate client input and emit OpenAI-compatible output, not branch on provider-native formats.
- Secrets must not be logged or surfaced in readiness/models responses.
- SDK-native types should remain inside provider packages.

### Integration Points
- Extend `ProviderConfig.Type` validation to allow `anthropic` and `gemini`.
- Extend app wiring to instantiate the new adapters by provider type.
- Add Anthropic SDK dependency after Phase 2.2 verifies the Go baseline.
- Add request parameter propagation from `ChatCompletionRequest` to `LLMRequest` for at least `temperature` and `max_tokens` if supported.
- Add adapter tests using SDK-testable seams, provider-local mapping helpers, or fake HTTP transport where practical.
- Add integration tests proving `/v1/chat/completions` returns the same OpenAI-compatible shape when routed to Anthropic/Gemini fake providers.

</code_context>

<specifics>
## Specific Ideas

- User explicitly wants provider-specific adapters for Anthropic and Gemini instead of pretending every provider is OpenAI-compatible.
- User explicitly asked to confirm normalization. Decision: normalization is required because downstream clients only process OpenAI-compatible interfaces.
- User prefers official Anthropic SDK implementation, so the Anthropic adapter should start from `github.com/anthropics/anthropic-sdk-go`.
- Phase 2.2 exists to remove Go version uncertainty before this phase starts.

</specifics>

<must_build>
## Must Build In Phase 2.3

- Add provider config support for `anthropic` and `gemini`.
- Add native Anthropic adapter using the official Anthropic Go SDK unless a concrete blocker is documented.
- Add native Gemini adapter, evaluating official `google.golang.org/genai` first.
- Implement OpenAI request -> provider-native request/SDK mapping in each adapter.
- Implement provider-native response -> OpenAI-compatible internal normalization.
- Preserve OpenAI-compatible `/v1/chat/completions` response contract.
- Preserve provider-agnostic routing, health tracking, readiness, and model aggregation.
- Add structured error mapping for provider auth/rate-limit/invalid-model/upstream failures.
- Add unit tests for Anthropic mapping, Gemini mapping, finish reason normalization, provider errors, and config validation.
- Add integration tests with fake provider servers or SDK seams proving client-facing OpenAI-compatible responses.

</must_build>

<deferred>
## Deferred Ideas

- SSE streaming support for Anthropic and Gemini.
- Tool/function calling normalization.
- Multimodal input/output normalization.
- Embeddings APIs.
- Native Ollama adapter.
- Generic HTTP adapter.
- Retry/fallback-chain execution.
- Circuit breaker state machine beyond existing health status.
- Cost accounting and token usage aggregation.
- Provider model discovery from live upstream APIs.
- PostgreSQL-backed provider CRUD and Admin API.
- Redis-backed provider health.

</deferred>

<success_criteria>
## Success Criteria

- Config can define OpenAI-compatible, Anthropic, and Gemini providers side by side.
- Gateway can route a normal OpenAI-compatible chat completion request to Anthropic and Gemini adapters.
- Anthropic adapter uses the official Anthropic Go SDK unless a documented blocker prevents it.
- Anthropic/Gemini adapters translate requests into provider-native payloads or SDK-native request types.
- Anthropic/Gemini adapters normalize native responses into `llm.LLMResponse`.
- `/v1/chat/completions` returns OpenAI-compatible `chat.completion` JSON for all supported provider types.
- Existing health-aware routing and `X-Route-To` work with native providers.
- Tests prove request mapping, response normalization, error mapping, and end-to-end client contract.
- No streaming, tools, multimodal, cost governance, Redis, PostgreSQL, Admin API, or generic provider mapping is added in this phase.
</success_criteria>

---

*Phase: 2.3-Native Anthropic and Gemini Provider Adapters*
*Context gathered: 2026-06-15*
