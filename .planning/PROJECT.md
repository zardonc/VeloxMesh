# VeloxMesh

## What This Is

VeloxMesh is a lightweight AI gateway for routing, governing, and observing LLM traffic across multiple providers. The current repository focuses on the gateway binary: a Go/Chi OpenAI-compatible data-plane API with provider adapters, routing boundaries, admission boundaries, and a path toward health-aware multi-provider routing.

The gateway is intended to remain a unified OpenAI-compatible entry point for downstream clients while provider adapters translate to each upstream provider's native protocol where needed.

## Core Value

Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## Requirements

### Validated

- ✓ Go/Chi gateway walking skeleton exists with `cmd/gateway/main.go`, app wiring, middleware, health endpoints, chat endpoint, provider adapter boundary, routing boundary, admission boundary, and integration tests — Phase 1.
- ✓ OpenAI-compatible non-streaming `POST /v1/chat/completions` request/response types exist — Phase 1.
- ✓ Static development API key auth exists for data-plane endpoints — Phase 1.
- ✓ `/healthz`, `/readyz`, and `/v1/models` endpoints exist — Phase 1.

### Active

- [ ] Begin durable control-state design for provider/API-key/config persistence.

### Out of Scope

- PostgreSQL-backed provider/API-key/config persistence — deferred until the gateway routing layer is stable.
- Redis-backed health, auth cache, rate limiting, and semantic cache — deferred until in-memory behavior is proven.
- Admin API and Admin Console — deferred; current control surface is static config/env.
- SSE streaming proxy — deferred; current chat endpoint is non-streaming only.
- Tool/function calling and multimodal provider normalization — deferred until text chat adapters are working.
- Cost governance, usage aggregation, and model degradation — deferred until provider routing and observability foundations exist.

## Context

- Source architecture: `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md`.
- The original gateway design is Go-first. TypeScript/Node gateway plans were superseded.
- Current code is a Phase 1 walking skeleton. It uses Go `1.26.1` in `go.mod`, Chi router, static environment config, a single OpenAI-compatible adapter, and basic integration tests.
- Phase 2.1 is planned in `.planning/phases/02-health-aware-routing/02-01-PLAN.md` but the current code has not yet implemented `internal/health`, multi-provider config, or health-aware routing.
- Native Anthropic/Gemini adapters are planned after the Go version baseline is verified. Downstream clients should continue to see OpenAI-compatible responses.

## Constraints

- **Tech stack**: Gateway is Go with Chi and standard `net/http` boundaries — matches the architecture and low-latency goal.
- **Client contract**: Data-plane clients consume OpenAI-compatible JSON over HTTP — provider-native responses must be normalized before returning to clients.
- **Provider isolation**: Provider-specific request/response details stay behind adapter packages.
- **Latency**: Optional systems such as semantic cache, storage, and admin features should not pollute the base forwarding path.
- **Security**: Do not log API keys, authorization headers, raw prompts, or sensitive provider payloads.
- **Current config**: Static env/config is acceptable until provider CRUD and durable config are intentionally added.
- **Temporary transitional measures**: When a solution is explicitly introduced as a temporary transitional measure during a development phase, its goal is only to meet the current phase's requirements. Do not spend excessive time optimizing, refining, or designing it for long-term maintainability unless it is expected to remain in use in future phases.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Gateway is implemented in Go with Chi | Low-latency, stdlib-compatible, architecture-aligned gateway path | ✓ Good |
| Public data plane is OpenAI-compatible | Keeps downstream clients simple and provider-agnostic | ✓ Good |
| Provider-specific behavior lives behind adapters | Allows Anthropic/Gemini/Gemini-native formats without changing handlers | ✓ Good |
| Phase 1 uses static dev auth and env config | Proves the call chain without pulling in PostgreSQL/Redis early | ✓ Good |
| Phase 2 should use in-memory/static control surfaces before Redis/Admin API | Builds routing value before persistence/control-plane scope | ✓ Good |
| Anthropic adapter should prefer official SDK after Go baseline verification | User preference; reduces provider mapping risk if SDK fits | ✓ Good |
| Static JSON multi-provider config is transitional | It satisfies Phase 2 provider/routing requirements but is expected to be replaced by runtime Admin Console/database configuration in a future phase | Temporary |

## Evolution

After each phase:
1. Move completed active requirements to Validated when implementation and verification pass.
2. Update Active with the next planned slice.
3. Record new key decisions when provider, routing, storage, or API-contract choices are locked.
4. Keep `What This Is` honest if the repository expands beyond the gateway binary.

---
*Last updated: 2026-06-17 after Phase 2 completion metadata reconciliation*
