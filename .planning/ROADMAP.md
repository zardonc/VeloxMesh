# Roadmap: VeloxMesh

**Created:** 2026-06-15
**Mode:** brownfield retrospective initialization
**Current focus:** Phase 4 - Streaming, Rate Limits, Cache, and Cost

## Overview

VeloxMesh is being built as vertical gateway slices. Phase 1 established the runnable Go/Chi OpenAI-compatible data-plane skeleton. Phase 2 completed the provider and adapter foundation milestone: multi-provider routing, provider health, native adapters, retry/fallback, active probing, adapter capability metadata, config hardening, model eligibility, and adapter conformance. Phase 3 completed durable control state: database-backed provider configuration, Admin provider CRUD, runtime reload, audit/idempotency, Redis hot state, and Redis config-change notifications.

## Gateway Runtime Modes

VeloxMesh should keep two startup modes explicit:

- **Lite mode**: SQLite-only startup for local or small deployments. It should require no PostgreSQL or Redis middleware, keep the core OpenAI-compatible gateway path and durable local provider configuration working, and clearly disable or degrade distributed features that need PostgreSQL/Redis semantics.
- **Full mode**: PostgreSQL + Redis startup for complete gateway functionality. It is the production-capable path for distributed durable state, hot-state coordination, config-change propagation, quota/cost features, and full Phase 4 behavior. Full mode expects PostgreSQL and Redis to be deployed before gateway startup, using Docker Compose or equivalent local middleware deployment.

Planning rule: every Phase 4 plan must state whether each feature works in lite mode, requires full mode, or degrades/fails closed without PostgreSQL/Redis.

| Phase | Name | Status | Plans | Requirements |
|-------|------|--------|-------|--------------|
| 1 | Gateway Walking Skeleton | Complete | 1/1 complete | GW-01..05, CHAT-01..05, PROV-01..03, ROUTE-01..02, OBS-01..02 |
| 2.1 | Health-Aware Multi-Provider Routing | Complete | 1/1 complete | PROV-04..05, ROUTE-03..06, OBS-03 |
| 2.2 | Go Version Baseline for Official Provider SDKs | Complete | 1/1 complete | OPS-01..02 |
| 2.3 | Native Anthropic and Gemini Provider Adapters | Complete | 1/1 complete | PROV-06, NPROV-01..03 |
| 2.4 | Provider Reliability and Error Contract | Complete | 1/1 complete | provider error taxonomy, adapter hardening, readiness semantics |
| 2.5 | Provider Retry and Fallback Execution | Complete | 1/1 complete | retryability policy, fallback execution, attempt observability |
| 2.6 | Active Provider Health Probing and Recovery | Complete | 1/1 complete | active probing, probe-driven recovery, readiness probe visibility |
| 2.7 | Provider Adapter Capability Contract | Complete | 2/2 complete | provider-neutral adapter capability metadata |
| 2.8 | Provider Configuration Schema and Secret-Safe Validation | Complete | 1/1 complete | static config schema hardening |
| 2.9 | Provider Model Catalog and Routing Eligibility | Complete | 1/1 complete | model/provider capability eligibility |
| 2.10 | Adapter Conformance Test Harness | Complete | 1/1 complete | reusable adapter contract tests |
| 3 | Durable Control State | Complete | 7/7 complete | CTRL-01..03 |
| 4 | Streaming, Rate Limits, Cache, and Cost | Testing (04-11) | 12/12 complete | STRM-01, RATE-01, CACHE-01, COST-01, CB-01 |

## Phase Details

### Phase 1: Gateway Walking Skeleton

**Goal:** Create the runnable Go gateway foundation and prove the client-to-provider call chain.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/01-gateway-walking-skeleton/01-CONTEXT.md`
- `.planning/phases/01-gateway-walking-skeleton/01-01-PLAN.md`
- `.planning/phases/01-gateway-walking-skeleton/01-01-SUMMARY.md`
- `.planning/phases/01-gateway-walking-skeleton/01-VERIFICATION.md`

**Success Criteria:**

1. `GET /healthz` returns liveness.
2. `GET /readyz` checks static provider readiness.
3. `POST /v1/chat/completions` accepts OpenAI-compatible non-streaming chat requests.
4. Provider calls pass through routing and admission boundaries.
5. Responses and errors are normalized enough for Phase 1 tests and local debugging.

### Phase 2.1: Health-Aware Multi-Provider Routing

**Goal:** Extend the single-provider gateway into an in-memory health-aware routing layer for multiple OpenAI-compatible providers.
**Status:** Complete
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
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-02-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-02-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-02-UAT.md`

**Success Criteria:**

1. `go.mod`, README, CI, and local tooling agree on the Go version baseline.
2. The selected baseline satisfies Anthropic official SDK requirements.
3. `go test ./...` passes under the selected baseline.
4. Phase 2.3 can start without reopening toolchain questions.

### Phase 2.3: Native Anthropic and Gemini Provider Adapters

**Goal:** Add native provider adapters while preserving OpenAI-compatible downstream responses.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-03-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-03-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-03-UAT.md`

**Success Criteria:**

1. Config supports `anthropic` and `gemini` provider types.
2. Anthropic adapter uses the official Anthropic Go SDK unless a concrete blocker is documented.
3. Gemini adapter evaluates the official Google Gen AI Go SDK before implementation.
4. Native provider responses normalize into internal `LLMResponse`.
5. `/v1/chat/completions` remains OpenAI-compatible for downstream clients.
6. Tests prove request mapping, response normalization, error mapping, and routing integration.

### Phase 2.4: Provider Reliability and Error Contract

**Goal:** Standardize provider error classification, improve provider health semantics, and ensure adapters honor the shared request/response contract.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-04-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-04-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-04-UAT.md`

**Success Criteria:**

1. Provider errors use shared structured categories.
2. OpenAI-compatible, Anthropic, and Gemini adapters map retryable and non-retryable failures consistently.
3. Health impact semantics are predictable across provider categories.
4. Readiness remains secret-safe and avoids expensive provider calls.

### Phase 2.5: Provider Retry and Fallback Execution

**Goal:** Let non-streaming chat requests survive transient provider failures by trying another eligible provider when safe.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-05-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-05-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-05-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-05-UAT.md`

**Success Criteria:**

1. Retryability is based on Phase 2.4 provider error categories.
2. Fallback attempts exclude already failed providers.
3. `X-Route-To` remains a strict override and disables fallback.
4. Attempt metadata is exposed through safe response headers.
5. Tests prove fallback success, exhaustion, non-retryable errors, and strict override behavior.

### Phase 2.6: Active Provider Health Probing and Recovery

**Goal:** Add configurable in-process provider health checks so provider health can recover without waiting for live client traffic.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-06-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-06-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-06-UAT.md`

**Success Criteria:**

1. Provider health checks are structured in static config.
2. `ProbeProvider(ctx, providerID)` and `ProbeOnce(ctx)` exist for deterministic internal use.
3. Probe results can degrade and recover provider health.
4. `/readyz` exposes secret-safe probe state.
5. Probe lifecycle is context-bound and test-safe.

### Phase 2.7: Provider Adapter Capability Contract

**Goal:** Make every provider adapter describe supported operations, modalities, parameters, and future feature flags in a provider-neutral way.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-07-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-07-01-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-07-02-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-07-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-07-UAT.md`
- `.planning/phases/02-health-aware-routing/02-07-VALIDATION.md`
- `.planning/phases/02-health-aware-routing/02-07-SECURITY.md`

**Success Criteria:**

1. The adapter contract exposes provider-neutral capability metadata without leaking SDK-native details.
2. OpenAI-compatible, Anthropic, and Gemini adapters report consistent capability metadata.
3. Registry APIs can list and resolve capabilities without importing provider-specific packages outside app wiring.
4. Readiness or internal model surfaces can include safe capability metadata only where compatible.
5. Tests prove capability metadata is stable and secret-safe.

### Phase 2.8: Provider Configuration Schema and Secret-Safe Validation

**Goal:** Harden static provider configuration into a stable schema that can later become the Admin API/Admin Console contract.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-08-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-08-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-08-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-08-UAT.md`

**Success Criteria:**

1. Provider config structs cover identity, type, base URL, auth reference, models, defaults, timeout, health overrides, retry/fallback settings, and capability overrides if needed.
2. Validation rejects duplicate provider IDs, unknown provider types, invalid URLs, invalid durations, invalid thresholds, and model/default mismatches.
3. Validation and docs remain secret-safe.
4. Existing backward-compatible env config still loads.

### Phase 2.9: Provider Model Catalog and Routing Eligibility

**Goal:** Build an internal model catalog that maps models to providers and provider capabilities.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-09-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-09-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-09-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-09-UAT.md`
- `.planning/phases/02-health-aware-routing/02-09-SECURITY.md`

**Success Criteria:**

1. Catalog derives model/provider eligibility from static config and adapter capability metadata.
2. Routing can ask one source whether a provider supports a requested model and operation.
3. `/v1/models` remains OpenAI-compatible while internal metadata can be richer.
4. Tests cover shared model names, provider-specific models, unknown models, and capability-ineligible providers.

### Phase 2.10: Adapter Conformance Test Harness

**Goal:** Create reusable conformance tests that every current and future provider adapter must pass.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-10-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-10-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-10-UAT.md`
- `.planning/phases/02-health-aware-routing/02-10-SECURITY.md`

**Success Criteria:**

1. Shared test helpers cover request mapping sanity, response normalization, finish reason mapping, structured error categories, health-check behavior, and secret-safe errors.
2. OpenAI-compatible, Anthropic, and Gemini adapters run through the harness.
3. Provider-specific tests remain where SDK details matter.
4. A future adapter has a clear test contract before registration.

### Phase 3: Durable Control State

**Goal:** Introduce persistent provider/API-key/config state and Redis-backed hot control state.
**Status:** Complete
**Plans:** 7 plans complete
Plans:
**Wave 1**

- [x] 03-01-PLAN.md - Durable control-state contracts, schema migrations, validation, and encrypted provider secrets.

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 03-02-PLAN.md - PostgreSQL/SQLite repositories, backend config, and local-dev static seed semantics.

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 03-03-PLAN.md - Runtime provider loading, actionable missing-config errors, disabled-provider filtering, and reload without restart.

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 03-04-PLAN.md - Versioned Admin provider CRUD API, dedicated admin bearer auth, transactional runtime activation, and redacted DTOs.

**Wave 5** *(blocked on Wave 4 completion)*

- [x] 03-05-PLAN.md - Provider test-connection action, idempotency keys, audit events, and audit retention.

**Wave 6** *(blocked on Wave 5 completion)*

- [x] 03-06-PLAN.md - Optional Redis health/probe hot state, data-plane auth cache, namespace/TTL handling, and local degradation.

**Wave 7** *(blocked on Wave 6 completion)*

- [x] 03-07-PLAN.md - Redis config-change pub/sub notifications and no-Redis consistency documentation.

**Primary artifacts:**

- `.planning/phases/03-durable-control-state/03-CONTEXT.md`
- `.planning/phases/03-durable-control-state/03-01-PLAN.md`
- `.planning/phases/03-durable-control-state/03-01-UAT.md`
- `.planning/phases/03-durable-control-state/03-02-PLAN.md`
- `.planning/phases/03-durable-control-state/03-02-UAT.md`
- `.planning/phases/03-durable-control-state/03-03-PLAN.md`
- `.planning/phases/03-durable-control-state/03-03-UAT.md`
- `.planning/phases/03-durable-control-state/03-04-PLAN.md`
- `.planning/phases/03-durable-control-state/03-04-UAT.md`
- `.planning/phases/03-durable-control-state/03-05-PLAN.md`
- `.planning/phases/03-durable-control-state/03-05-UAT.md`
- `.planning/phases/03-durable-control-state/03-06-PLAN.md`
- `.planning/phases/03-durable-control-state/03-06-UAT.md`
- `.planning/phases/03-durable-control-state/03-07-PLAN.md`
- `.planning/phases/03-durable-control-state/03-07-SUMMARY.md`
- `.planning/phases/03-durable-control-state/03-07-UAT.md`

**Success Criteria:**


**Success Criteria:**

1. Config supports `anthropic` and `gemini` provider types.
2. Anthropic adapter uses the official Anthropic Go SDK unless a concrete blocker is documented.
3. Gemini adapter evaluates the official Google Gen AI Go SDK before implementation.
4. Native provider responses normalize into internal `LLMResponse`.
5. `/v1/chat/completions` remains OpenAI-compatible for downstream clients.
6. Tests prove request mapping, response normalization, error mapping, and routing integration.

### Phase 2.4: Provider Reliability and Error Contract

**Goal:** Standardize provider error classification, improve provider health semantics, and ensure adapters honor the shared request/response contract.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-04-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-04-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-04-UAT.md`

**Success Criteria:**

1. Provider errors use shared structured categories.
2. OpenAI-compatible, Anthropic, and Gemini adapters map retryable and non-retryable failures consistently.
3. Health impact semantics are predictable across provider categories.
4. Readiness remains secret-safe and avoids expensive provider calls.

### Phase 2.5: Provider Retry and Fallback Execution

**Goal:** Let non-streaming chat requests survive transient provider failures by trying another eligible provider when safe.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-05-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-05-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-05-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-05-UAT.md`

**Success Criteria:**

1. Retryability is based on Phase 2.4 provider error categories.
2. Fallback attempts exclude already failed providers.
3. `X-Route-To` remains a strict override and disables fallback.
4. Attempt metadata is exposed through safe response headers.
5. Tests prove fallback success, exhaustion, non-retryable errors, and strict override behavior.

### Phase 2.6: Active Provider Health Probing and Recovery

**Goal:** Add configurable in-process provider health checks so provider health can recover without waiting for live client traffic.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-06-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-06-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-06-UAT.md`

**Success Criteria:**

1. Provider health checks are structured in static config.
2. `ProbeProvider(ctx, providerID)` and `ProbeOnce(ctx)` exist for deterministic internal use.
3. Probe results can degrade and recover provider health.
4. `/readyz` exposes secret-safe probe state.
5. Probe lifecycle is context-bound and test-safe.

### Phase 2.7: Provider Adapter Capability Contract

**Goal:** Make every provider adapter describe supported operations, modalities, parameters, and future feature flags in a provider-neutral way.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-07-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-07-01-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-07-02-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-07-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-07-UAT.md`
- `.planning/phases/02-health-aware-routing/02-07-VALIDATION.md`
- `.planning/phases/02-health-aware-routing/02-07-SECURITY.md`

**Success Criteria:**

1. The adapter contract exposes provider-neutral capability metadata without leaking SDK-native details.
2. OpenAI-compatible, Anthropic, and Gemini adapters report consistent capability metadata.
3. Registry APIs can list and resolve capabilities without importing provider-specific packages outside app wiring.
4. Readiness or internal model surfaces can include safe capability metadata only where compatible.
5. Tests prove capability metadata is stable and secret-safe.

### Phase 2.8: Provider Configuration Schema and Secret-Safe Validation

**Goal:** Harden static provider configuration into a stable schema that can later become the Admin API/Admin Console contract.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-08-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-08-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-08-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-08-UAT.md`

**Success Criteria:**

1. Provider config structs cover identity, type, base URL, auth reference, models, defaults, timeout, health overrides, retry/fallback settings, and capability overrides if needed.
2. Validation rejects duplicate provider IDs, unknown provider types, invalid URLs, invalid durations, invalid thresholds, and model/default mismatches.
3. Validation and docs remain secret-safe.
4. Existing backward-compatible env config still loads.

### Phase 2.9: Provider Model Catalog and Routing Eligibility

**Goal:** Build an internal model catalog that maps models to providers and provider capabilities.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-09-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-09-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-09-SUMMARY.md`
- `.planning/phases/02-health-aware-routing/02-09-UAT.md`
- `.planning/phases/02-health-aware-routing/02-09-SECURITY.md`

**Success Criteria:**

1. Catalog derives model/provider eligibility from static config and adapter capability metadata.
2. Routing can ask one source whether a provider supports a requested model and operation.
3. `/v1/models` remains OpenAI-compatible while internal metadata can be richer.
4. Tests cover shared model names, provider-specific models, unknown models, and capability-ineligible providers.

### Phase 2.10: Adapter Conformance Test Harness

**Goal:** Create reusable conformance tests that every current and future provider adapter must pass.
**Status:** Complete
**Primary artifacts:**

- `.planning/phases/02-health-aware-routing/02-10-CONTEXT.md`
- `.planning/phases/02-health-aware-routing/02-10-PLAN.md`
- `.planning/phases/02-health-aware-routing/02-10-UAT.md`
- `.planning/phases/02-health-aware-routing/02-10-SECURITY.md`

**Success Criteria:**

1. Shared test helpers cover request mapping sanity, response normalization, finish reason mapping, structured error categories, health-check behavior, and secret-safe errors.
2. OpenAI-compatible, Anthropic, and Gemini adapters run through the harness.
3. Provider-specific tests remain where SDK details matter.
4. A future adapter has a clear test contract before registration.

### Phase 3: Durable Control State

**Goal:** Introduce persistent provider/API-key/config state and Redis-backed hot control state.
**Status:** Complete
**Plans:** 7 plans complete
Plans:
**Wave 1**

- [x] 03-01-PLAN.md - Durable control-state contracts, schema migrations, validation, and encrypted provider secrets.

**Wave 2** *(blocked on Wave 1 completion)*

- [x] 03-02-PLAN.md - PostgreSQL/SQLite repositories, backend config, and local-dev static seed semantics.

**Wave 3** *(blocked on Wave 2 completion)*

- [x] 03-03-PLAN.md - Runtime provider loading, actionable missing-config errors, disabled-provider filtering, and reload without restart.

**Wave 4** *(blocked on Wave 3 completion)*

- [x] 03-04-PLAN.md - Versioned Admin provider CRUD API, dedicated admin bearer auth, transactional runtime activation, and redacted DTOs.

**Wave 5** *(blocked on Wave 4 completion)*

- [x] 03-05-PLAN.md - Provider test-connection action, idempotency keys, audit events, and audit retention.

**Wave 6** *(blocked on Wave 5 completion)*

- [x] 03-06-PLAN.md - Optional Redis health/probe hot state, data-plane auth cache, namespace/TTL handling, and local degradation.

**Wave 7** *(blocked on Wave 6 completion)*

- [x] 03-07-PLAN.md - Redis config-change pub/sub notifications and no-Redis consistency documentation.

**Primary artifacts:**

- `.planning/phases/03-durable-control-state/03-CONTEXT.md`
- `.planning/phases/03-durable-control-state/03-01-PLAN.md`
- `.planning/phases/03-durable-control-state/03-01-UAT.md`
- `.planning/phases/03-durable-control-state/03-02-PLAN.md`
- `.planning/phases/03-durable-control-state/03-02-UAT.md`
- `.planning/phases/03-durable-control-state/03-03-PLAN.md`
- `.planning/phases/03-durable-control-state/03-03-UAT.md`
- `.planning/phases/03-durable-control-state/03-04-PLAN.md`
- `.planning/phases/03-durable-control-state/03-04-UAT.md`
- `.planning/phases/03-durable-control-state/03-05-PLAN.md`
- `.planning/phases/03-durable-control-state/03-05-UAT.md`
- `.planning/phases/03-durable-control-state/03-06-PLAN.md`
- `.planning/phases/03-durable-control-state/03-06-UAT.md`
- `.planning/phases/03-durable-control-state/03-07-PLAN.md`
- `.planning/phases/03-durable-control-state/03-07-SUMMARY.md`
- `.planning/phases/03-durable-control-state/03-07-UAT.md`

**Success Criteria:**

1. PostgreSQL schema stores provider, API key, routing, and usage records.
2. Redis stores provider health/probe hot state, auth-cache hot state, and config-change notifications where configured.
3. Admin API manages provider config without process restart.

### Phase 4: Streaming, Rate Limits, Cache, and Cost

**Goal:** Add advanced gateway features after routing and adapters are stable.
**Status:** Testing (04-11)

**Success Criteria:**

1. SSE streaming proxy works with provider adapters.
2. Rate limits and admission controls protect providers.
3. Semantic cache and cost governance are opt-in and observable.
4. Circuit breaker and fallback-chain behavior is tested.
5. Lite and full startup modes are documented and enforced by capability checks.

## Notes

- Phase 3 is complete. Phase 2 static-config and in-process control surfaces remain only as compatibility/local-development paths; durable control state is now the intended provider configuration path.
- Lite mode uses SQLite only and intentionally limited/basic features; full mode requires PostgreSQL + Redis predeployed through Docker Compose or equivalent and unlocks complete gateway behavior.
- When a solution is explicitly introduced as a temporary transitional measure during a development phase, its goal is only to meet the current phase's requirements. Do not spend excessive time optimizing, refining, or designing it for long-term maintainability unless it is expected to remain in use in future phases.
- Do not add streaming, semantic cache, rate limiting, or cost governance until their later phase is explicitly scoped.
- Native provider SDK details stay inside adapter packages; handlers and routing consume provider-neutral contracts.

## Local Development Resources

The local development environment has been verified and configured. The following resources are available and their specific connection details, models, and credentials can be found in the local `.env` and `.env.local` files:

- **Infrastructure**: 
  - PostgreSQL Database
  - Redis Cache
- **LLM Providers**:
  - `sanf` (OpenAI Primary)
  - `sans` (SANS Primary, with multiple models configured)

---
*Roadmap refreshed: 2026-06-19 after Phase 3 durable control state UAT completion*
