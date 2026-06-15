# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Mode:** brownfield retrospective initialization
**Current focus:** Phase 2.1 - Health-Aware Multi-Provider Routing

## Overview

VeloxMesh is being built as vertical gateway slices. Phase 1 established the runnable data-plane walking skeleton. Phase 2 turns that skeleton into a real routing layer, then prepares for native provider adapters.

| Phase | Name | Status | Plans | Requirements |
|-------|------|--------|-------|--------------|
| 1 | Gateway Walking Skeleton | Complete | 1/1 complete | GW-01..05, CHAT-01..05, PROV-01..03, ROUTE-01..02, OBS-01..02 |
| 2.1 | Health-Aware Multi-Provider Routing | Planned | 0/1 complete | PROV-04..05, ROUTE-03..06, OBS-03 |
| 2.2 | Go Version Baseline for Official Provider SDKs | Proposed | 0/0 | OPS-01..02 |
| 2.3 | Native Anthropic and Gemini Provider Adapters | Proposed | 0/0 | PROV-06, NPROV-01..03 |
| 3 | Durable Control State | Future | 0/0 | CTRL-01..03 |
| 4 | Streaming, Rate Limits, Cache, and Cost | Future | 0/0 | STRM-01, RATE-01, CACHE-01, COST-01, CB-01 |

## Phase Details

### Phase 1: Gateway Walking Skeleton

**Goal:** Create the runnable Go gateway foundation and prove the client-to-provider call chain.
**Status:** Complete
**Primary artifacts:**
- `.planning/phases/01-gateway-walking-skeleton/01-CONTEXT.md`
- `.planning/phases/01-gateway-walking-skeleton/01-01-PLAN.md`
- `.planning/phases/01-gateway-walking-skeleton/01-01-SUMMARY.md`

**Success Criteria:**
1. `GET /healthz` returns liveness.
2. `GET /readyz` checks static provider readiness.
3. `POST /v1/chat/completions` accepts OpenAI-compatible non-streaming chat requests.
4. Provider calls pass through routing and admission boundaries.
5. Responses and errors are normalized enough for Phase 1 tests and local debugging.

### Phase 2.1: Health-Aware Multi-Provider Routing

**Goal:** Extend the single-provider gateway into an in-memory health-aware routing layer for multiple OpenAI-compatible providers.
**Status:** Planned
**Primary artifacts:**
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-01-PLAN.md`

**Success Criteria:**
1. Static config supports multiple OpenAI-compatible provider definitions.
2. Provider registry lists and resolves multiple providers with stable ordering.
3. In-memory health store tracks latency, pending requests, failures, and health status.
4. Router supports `round-robin`, `least-latency`, and `X-Route-To` override.
5. Router skips unhealthy providers and returns structured errors when none are routable.
6. `/readyz` and `/v1/models` reflect multi-provider state.
7. Unit and integration tests prove routing behavior with fake upstream providers.

### Phase 2.2: Go Version Baseline for Official Provider SDKs

**Goal:** Confirm the active Go version baseline supports official provider SDK adoption, especially Anthropic's Go SDK.
**Status:** Proposed

**Success Criteria:**
1. `go.mod`, README, CI, and local tooling agree on the Go version baseline.
2. The selected baseline satisfies Anthropic official SDK requirements.
3. `go test ./...` passes under the selected baseline.
4. Phase 2.3 can start without reopening toolchain questions.

### Phase 2.3: Native Anthropic and Gemini Provider Adapters

**Goal:** Add native provider adapters while preserving OpenAI-compatible downstream responses.
**Status:** Proposed

**Success Criteria:**
1. Config supports `anthropic` and `gemini` provider types.
2. Anthropic adapter uses the official Anthropic Go SDK unless a concrete blocker is documented.
3. Gemini adapter evaluates the official Google Gen AI Go SDK before implementation.
4. Native provider responses normalize into internal `LLMResponse`.
5. `/v1/chat/completions` remains OpenAI-compatible for downstream clients.
6. Tests prove request mapping, response normalization, error mapping, and routing integration.

### Phase 3: Durable Control State

**Goal:** Introduce persistent provider/API-key/config state and Redis-backed hot control state.
**Status:** Future

**Success Criteria:**
1. PostgreSQL schema stores provider, API key, routing, and usage records.
2. Redis stores health and hot control state where appropriate.
3. Admin API manages provider config without process restart.

### Phase 4: Streaming, Rate Limits, Cache, and Cost

**Goal:** Add the advanced gateway features from the architecture after routing and adapters are stable.
**Status:** Future

**Success Criteria:**
1. SSE streaming proxy works with provider adapters.
2. Rate limits and admission controls protect providers.
3. Semantic cache and cost governance are opt-in and observable.
4. Circuit breaker and fallback-chain behavior is tested.

## Notes

- Phase 2.1 is planned but not reflected in current source code yet.
- Current source code still has single-provider env config and static routing.
- Native provider adapters should not be implemented before the Go baseline is settled.

---
*Roadmap created: 2026-06-15*
*Last updated: 2026-06-15 after retrospective project initialization*
