# Phase 1 Context: Go Gateway Walking Skeleton

<domain>
VeloxMesh is the AI Gateway portion of the Velox architecture. The gateway is the unified entry point for LLM traffic and is optimized for low-latency request forwarding, routing, governance, and observability.

Phase 1 focuses on a Go gateway walking skeleton: a runnable Go 1.22+ Chi service that accepts an OpenAI-compatible chat request, passes through the intended data-plane boundaries, calls one OpenAI-compatible provider, and returns a normalized response.
</domain>

<canonical_refs>
- `README.md` — repository-level project description.
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` — source-of-truth architecture document for the gateway.
- `.planning/phases/01-gateway-walking-skeleton/01-01-PLAN.md` — executable Phase 1 Go implementation plan.
</canonical_refs>

<decisions>

## D-01: Gateway implementation language is Go

The AI gateway must be implemented with Go 1.22+ and Chi router. TypeScript, Node.js, Fastify, Hono, and Vitest are not valid gateway implementation choices.

## D-02: Process model is a single Go binary

Control-plane and data-plane boundaries should be represented as logical Go package boundaries inside one process, not separate services in Phase 1.

## D-03: Phase 1 proves the data-plane call chain

The first phase must prove:

client -> Chi router -> request id/logging/auth middleware -> request normalization -> gateway service -> routing boundary -> admission boundary -> provider adapter -> upstream LLM provider -> normalized response -> client.

## D-04: Public API target is OpenAI-compatible JSON over HTTP

The first endpoint is `POST /v1/chat/completions`. Phase 1 supports non-streaming chat completions only. SSE streaming remains a later phase.

## D-05: Health endpoint names follow the architecture document

Use `GET /healthz` for liveness and `GET /readyz` for readiness.

## D-06: Provider abstraction follows the architecture direction

Provider adapters expose Go interfaces with methods equivalent to:

- `ID() string`
- `Models() []string`
- `Complete(ctx, *LLMRequest) (*LLMResponse, error)`
- `HealthCheck(ctx) HealthStatus`

`Stream(...)` can be designed later with the SSE phase; Phase 1 should not implement fake streaming.

## D-07: Phase 1 keeps future boundaries without implementing full systems

Routing and admission control should exist as interfaces/packages in Phase 1, but their implementation can be static/pass-through. This preserves architecture alignment without pulling advanced routing, queues, Redis, or PostgreSQL into the first slice.

## D-08: Phase 1 uses static bootstrap config instead of PG/Redis

The architecture eventually uses PostgreSQL for durable config/API keys and Redis for auth cache, rate limiting, provider health, and exact cache. Phase 1 may use environment/config-file provider settings and one development API key to complete the call chain.

## D-09: Observability starts with slog

Use Go `slog` structured JSON logs in Phase 1. Prometheus and OpenTelemetry should be added later after the call chain is stable.

## D-10: Scope remains narrow

Phase 1 excludes PostgreSQL, Redis, admin API, streaming, rate limiting, semantic cache, composite routing, circuit breakers, fallback chain, cost governance, usage aggregation, Docker Compose, and Admin Console.

</decisions>

<code_context>
The repository currently contains only project documentation and planning files. No gateway implementation exists yet.

The prior Phase 1 plan incorrectly specified a TypeScript backend. It has been superseded by the Go-based plan in `.planning/phases/01-gateway-walking-skeleton/01-01-PLAN.md`.
</code_context>

<gray_areas_resolved>
- Stack: Go 1.22+ with Chi.
- API style: OpenAI-compatible JSON over HTTP.
- Liveness/readiness names: `/healthz` and `/readyz`.
- Logging: Go `slog` structured JSON.
- First provider: OpenAI-compatible adapter.
- First auth approach: static bootstrap/dev API key, replacing later with PG/Redis-backed API key verification.
- First routing approach: static/single-provider routing boundary.
- First admission approach: pass-through boundary with priority parsing.
</gray_areas_resolved>

<deferred>
- PostgreSQL 16 + pgvector schema and migrations.
- Redis state/cache split.
- Auth cache and durable API key management.
- Sliding-window rate limiting.
- Full admission control queues and concurrency gates.
- Latency-aware routing, composite scoring, and heuristic rules.
- Circuit breaker and fallback behavior.
- SSE streaming proxy with byte-level passthrough.
- Semantic cache and Python embedding sidecar.
- Admin API and Admin Console.
- Prometheus metrics endpoint and OpenTelemetry tracing.
- Usage aggregation, cost governance, and model degradation.
- Docker Compose development stack.
</deferred>
