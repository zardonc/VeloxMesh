# VeloxMesh

## What This Is

VeloxMesh is a lightweight AI gateway for routing, governing, and observing LLM traffic across multiple providers. The current repository focuses on the gateway binary: a Go/Chi OpenAI-compatible data-plane API with provider adapters, streaming support, durable provider control state, credit quotas, usage settlement, semantic caching, and Redis-backed hot-state coordination where configured.

The gateway is intended to remain a unified OpenAI-compatible entry point for downstream clients while provider adapters translate to each upstream provider's native protocol where needed.

## Core Value

Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## Current State

**v7.1 Advanced Routing & Observability** has been shipped. The Composite Score Router and OpenTelemetry/Prometheus integrations are live.

## Current Milestone: v7.2 Multi-Node Coordination

**Goal:** Enable Plan 2 multi-node deployment for the gateway without changing the OpenAI-compatible data-plane contract.

**Target features:**
- Redis-backed node coordination and leader election.
- SQLite relational WAL replication with write fencing.
- Cluster health, recovery, graceful shutdown, and chaos verification.

## Next Milestone Goals

Phase 13 PostgreSQL Extension remains deferred until Phase 12 proves the multi-node coordination boundary.

<details>
<summary>Archived Milestone: v7.1 Advanced Routing & Observability</summary>

**Goal:** Ship the Composite Score Router and production-grade observability for routing decisions.

**Target features:**
- Composite Score Router using latency, pending requests, error rates, costs, and health bonuses.
- Z-score normalization for routing signals.
- OpenTelemetry traces for TTFT, TPOT, E2E latency, and cache hits.
- Prometheus histogram metrics for routing and request timing.

</details>

## Requirements

### Validated

- ✓ Go/Chi gateway walking skeleton exists with `cmd/gateway/main.go`, app wiring, middleware, health endpoints, chat endpoint, provider adapter boundary, routing boundary, admission boundary, and integration tests — Phase 1.
- ✓ OpenAI-compatible non-streaming `POST /v1/chat/completions` request/response types exist — Phase 1.
- ✓ Static development API key auth exists for data-plane endpoints — Phase 1.
- ✓ `/healthz`, `/readyz`, and `/v1/models` endpoints exist — Phase 1.
- ✓ STRM-01: Gateway supports SSE streaming proxy — Phase 4.
- ✓ RATE-01: Gateway enforces rate limits — Phase 4.
- ✓ CACHE-01: Gateway supports semantic cache — Phase 4.
- ✓ COST-01: Gateway tracks usage and cost — Phase 4.
- ✓ CB-01: Gateway supports circuit breaker and fallback-chain behavior — Phase 4.
- ✓ Phase 5: Tool/Function Calling and Multimodal capabilities
- ✓ Phase 6: Model Combo Feature (RR, Fusion, capability-based routing)
- ✓ Phase 7: Adapter Interfaces & SQLite Foundation (v7 architecture refactor)
- ✓ Phase 8: Semantic Pipeline — v7.0
- ✓ Phase 9: Redis Stack + Qdrant Fallback Integration — v7.0
- ✓ Phase 10: Advanced Routing & Observability — v7.1


### Active

- [ ] Phase 12: Multi-Node Coordination — v7.2

### Deferred to Future Milestones

- BFF Layer & Admin Console (Phase 11)
- PostgreSQL Extension (Phase 13)
- Full `LimitRule` unification across all scopes

### Long-Term / Architectural Goals

- **Heuristic Rules System**: User-configurable pluggable rules for compression, input/output processing. Must pre-allocate extension points during early phases (Phase 5/6) to avoid major refactoring.

## Context

- Source architecture: `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md`.
- The original gateway design is Go-first. TypeScript/Node gateway plans were superseded.
- Current code includes Phase 1 through Phase 9: Go/Chi OpenAI-compatible data plane, multi-provider health-aware routing, native Anthropic/Gemini adapters, durable SQLite/PostgreSQL provider control state, versioned Admin provider CRUD, runtime reload, SSE streaming, rate limiting, semantic caching, usage tracking, SQLite-first Plan 1 foundation, configurable semantic pipeline, Redis hot-state primitives, Redis-backed admission direction, and Redis VSS fallback for Qdrant degradation. Architecture v2.1 makes SQLite the authoritative relational path, Redis Stack part of the Plan 1/2 runtime for hot cache/rate/config coordination, and Qdrant the primary vector and semantic-cache store. PostgreSQL remains a later adapter extension; LanceDB is retained only for edge builds.
- Downstream clients should continue to see OpenAI-compatible responses.

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
| Phase 1 uses static dev auth and env config | Proves the call chain without pulling in durable storage/Redis early | ✓ Good |
| Phase 2 should use in-memory/static control surfaces before Redis/Admin API | Builds routing value before persistence/control-plane scope | ✓ Good |
| Anthropic adapter should prefer official SDK after Go baseline verification | User preference; reduces provider mapping risk if SDK fits | ✓ Good |
| Static JSON multi-provider config is transitional | It satisfies Phase 2 provider/routing requirements but durable provider configuration is now the intended source of truth after Phase 3 | Temporary |
| Durable provider configuration is database-backed | Phase 3 introduced SQLite/PostgreSQL repositories plus Admin provider APIs and runtime reload; SQLite is now the primary v2.1 relational path | ✓ Good |
| Redis hot state is optional | Phase 3 Redis support coordinates health/probe/auth-cache/config-change hot state while no-Redis mode remains local/single-instance for reload consistency | ✓ Good |
| Phase 4 implemented SSE streaming and semantic cache natively | Fulfills advanced gateway functionality | ✓ Good |
| Qdrant replaces LanceDB on the main vector path | LanceDB blocked development and is not cross-platform enough for the default runtime; Qdrant provides official Go/gRPC integration, persistence, and cluster options | Active |
| LanceDB remains edge-only | Embedded vector storage still has value for zero-external-dependency Linux/macOS edge deployments, but it must be isolated behind `-tags lancedb` | Deferred |
| Redis is hot state, not source of truth | SQLite remains authoritative for user/account/security/billing state while Redis accelerates cache, rate, config, and aggregation paths | ✓ Good |
| Redis VSS is fallback-only | Qdrant remains primary; Redis VSS activates only for degraded Qdrant paths | ✓ Good |
| Phase 12 skips BFF/Admin Console work | Multi-node runtime coordination can ship before Phase 11; topology UI stays deferred | Active |
| Phase 13 waits for Phase 12 | PostgreSQL/pgvector should follow the finalized multi-node write and recovery boundaries | Deferred |
| Full LimitRule unification is deferred | Phase 9 shipped the minimal API-key/upstream-account direction; broader scope unification belongs in a future hardening phase | Deferred |

## Evolution

After each phase:
1. Move completed active requirements to Validated when implementation and verification pass.
2. Update Active with the next planned slice.
3. Record new key decisions when provider, routing, storage, or API-contract choices are locked.
4. Keep `What This Is` honest if the repository expands beyond the gateway binary.

---
*Last updated: 2026-07-02 after starting v7.2 Multi-Node Coordination*
