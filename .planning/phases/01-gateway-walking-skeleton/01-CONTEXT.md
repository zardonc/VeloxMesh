# Phase 1 Context: Gateway Walking Skeleton

<domain>
VeloxMesh is a lightweight AI gateway and agent orchestration layer for routing, governing, and observing LLM traffic across multiple providers.

Phase 1 focuses only on the gateway walking skeleton: a runnable backend service that forwards a validated client chat request to one OpenAI-compatible LLM provider and returns a normalized response.
</domain>

<canonical_refs>
- `README.md` — project description and product intent.
- `.planning/phases/01-gateway-walking-skeleton/01-01-PLAN.md` — executable Phase 1 implementation plan.
</canonical_refs>

<decisions>

## D-01: First phase is an end-to-end gateway call chain

The first phase must prove the complete path:

client -> gateway HTTP API -> validation -> gateway core -> provider adapter -> LLM provider -> normalized response -> client.

## D-02: API compatibility target is OpenAI-style chat completions

The first public endpoint should be `POST /v1/chat/completions`, using a minimal OpenAI-compatible request and response shape.

## D-03: Only one provider adapter is required in Phase 1

Phase 1 implements only an `openai-compatible` provider adapter configured through `baseUrl`, `apiKey`, and default model.

## D-04: Provider abstractions must exist from the start

Even with one provider, routes should call gateway core, and gateway core should call a provider interface. Provider-specific details belong in `src/providers`.

## D-05: Scope must stay narrow

Phase 1 excludes routing policies, fallback, retries, streaming, authentication, rate limiting, caching, usage metering, agent orchestration, dashboard UI, and database persistence.

</decisions>

<code_context>
The repository currently contains only `README.md`, so Phase 1 is a greenfield backend scaffold.
</code_context>

<deferred>
- Multi-provider routing and fallback.
- Streaming/SSE.
- API authentication and tenant management.
- Rate limiting.
- Semantic caching.
- Usage metering and cost controls.
- Agent workflow orchestration.
- Dashboard or frontend UI.
- Database persistence.
- Distributed tracing integration.
</deferred>
