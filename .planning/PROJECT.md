# VeloxMesh

## What This Is

VeloxMesh is a lightweight AI gateway for routing, governing, and observing LLM traffic across multiple providers. The current repository focuses on the gateway binary: a Go/Chi OpenAI-compatible data-plane API with provider adapters, streaming support, durable provider control state, credit quotas, usage settlement, semantic caching, and Redis-backed hot-state coordination where configured.

The gateway is intended to remain a unified OpenAI-compatible entry point for downstream clients while provider adapters translate to each upstream provider's native protocol where needed.

## Core Value

Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## Current State

**v7.3 PostgreSQL Compatibility** has been shipped. PostgreSQL control-state compatibility, pgvector semantic cache support, SQLite-to-PostgreSQL migration, and real-provider Plan 4 smoke verification are live as an opt-in deployment path. The current milestone adds an optional task Scheduler without changing the OpenAI-compatible data-plane contract.

## Current Milestone: v7.4 Gateway Scheduler

**Goal:** Add an optional stateless Scheduler that scores queued gateway tasks while the gateway keeps ownership of intake, queueing, execution, and fallback behavior.

**Target features:**
- Optional gRPC Scheduler service with `BatchScoreTasks`, health, metrics, and FIFO fallback when disabled or unhealthy.
- Redis ZSET queue backend with single-node in-memory fallback and circuit-breaker protected scheduler calls.
- Priority resolver, quota enforcement, static virtual deadline scoring, and cold-start heuristic scoring.
- Training-sample feedback loop, scheduler observability, and ONNX/LightGBM model path for later A/B comparison.

<details>
<summary>Archived Milestone: v7.2 Multi-Node Coordination</summary>

**Goal:** Enable Plan 2 multi-node deployment for the gateway without changing the OpenAI-compatible data-plane contract.

**Target features:**
- Redis-backed node coordination and leader election.
- SQLite relational WAL replication with write fencing.
- Cluster health, recovery, graceful shutdown, and chaos verification.

</details>

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
- ✓ Phase 13: PostgreSQL Compatibility — v7.3


### Active

- [ ] SCH-01: Gateway can run with Scheduler disabled and fall back to FIFO queue scoring without startup failure.
- [ ] SCH-02: Gateway can call a stateless Scheduler over gRPC with a 15ms timeout and circuit-breaker fallback.
- [ ] PRIO-01: Gateway resolves priority only from trusted structured inputs and never from prompt text.
- [ ] SCORE-01: Scheduler can compute static virtual deadline scores from predicted latency, priority multiplier, and uncertainty.
- [ ] FEED-01: Gateway can record scheduler training samples without storing raw prompts or provider secrets.
- [ ] ML-01: Operators can introduce ONNX/LightGBM scheduler models through versioned artifacts and A/B routing.

### Deferred to Future Milestones

- BFF Layer & Admin Console (Phase 11)
- Full `LimitRule` unification across all scopes outside the PostgreSQL-compatible Plan 4 path

### Long-Term / Architectural Goals

- **Heuristic Rules System**: User-configurable pluggable rules for compression, input/output processing. Must pre-allocate extension points during early phases (Phase 5/6) to avoid major refactoring.

## Context

- Source architecture: `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md`.
- Scheduler implementation reference: `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\Gateway-Scheduler-Implementation.md`.
- Operational resource lookup: test-environment components are configured in `.env`, including the test environment address; provider credentials and model resources for real-provider UAT are configured in `.env.local`. Prefer non-Gemini provider resources for routine real-provider checks because Gemini entries may carry usage-limit notes and should be reserved for Gemini-specific scenarios.
- The original gateway design is Go-first. TypeScript/Node gateway plans were superseded.
- Current code includes Phase 1 through Phase 9: Go/Chi OpenAI-compatible data plane, multi-provider health-aware routing, native Anthropic/Gemini adapters, durable SQLite/PostgreSQL provider control state, versioned Admin provider CRUD, runtime reload, SSE streaming, rate limiting, semantic caching, usage tracking, SQLite-first Plan 1 foundation, configurable semantic pipeline, Redis hot-state primitives, Redis-backed admission direction, and Redis VSS fallback for Qdrant degradation. Architecture v2.1 makes SQLite the authoritative relational path, Redis Stack part of the Plan 1/2 runtime for hot cache/rate/config coordination, and Qdrant the primary vector and semantic-cache store. PostgreSQL remains a later adapter extension; LanceDB is retained only for edge builds.
- Downstream clients should continue to see OpenAI-compatible responses.

## Constraints

- **Tech stack**: Gateway is Go with Chi and standard `net/http` boundaries — matches the architecture and low-latency goal.
- **Client contract**: Data-plane clients consume OpenAI-compatible JSON over HTTP — provider-native responses must be normalized before returning to clients.
- **Provider isolation**: Provider-specific request/response details stay behind adapter packages.
- **Latency**: Optional systems such as semantic cache, storage, and admin features should not pollute the base forwarding path.
- **Security**: Do not log API keys, authorization headers, raw prompts, or sensitive provider payloads.
- **Scheduler optionality**: Scheduler must be disabled by default and must degrade to FIFO without breaking gateway startup or request forwarding.
- **Scheduler latency**: Scheduler gRPC calls have a 15ms budget; failures, timeouts, or open breakers must fall back rather than retry inline.
- **Priority safety**: Priority may come only from trusted config, headers, or structured fields; prompt text must never influence priority.
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
| Phase 13 follows Phase 12 | PostgreSQL/pgvector can now use the finalized multi-node write and recovery boundaries | ✓ Good |
| Plan 4 uses PostgreSQL + pgvector as an extension path | SQLite + Qdrant remain the default Plans 1/2 path; PostgreSQL compatibility is for enterprise deployments that need concurrent writes and relational/vector joins | ✓ Good |
| Full LimitRule unification is deferred | Phase 9 shipped the minimal API-key/upstream-account direction; broader scope unification belongs in a future hardening phase | Deferred |
| Scheduler is a stateless scoring oracle | Gateway keeps queue ownership, task storage, execution, and fallback behavior; Scheduler only returns scores and prediction metadata | Active |
| Static virtual deadline is the scheduler score | One Redis score write avoids dynamic ZSET re-ranking and gives aging through enqueue time | Active |
| Cold start uses heuristic scoring before ONNX | Rules can ship with no training data; model prediction is introduced only after samples and validation exist | Active |

## Evolution

After each phase:
1. Move completed active requirements to Validated when implementation and verification pass.
2. Update Active with the next planned slice.
3. Record new key decisions when provider, routing, storage, or API-contract choices are locked.
4. Keep `What This Is` honest if the repository expands beyond the gateway binary.

---
*Last updated: 2026-07-03 after starting v7.4 Gateway Scheduler*
