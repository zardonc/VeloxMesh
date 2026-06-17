# Phase 2 Follow-up Roadmap: Provider and Adapter Foundation

**Created:** 2026-06-16
**Scope:** Remaining Phase 2 provider/adapter foundation work after 02-05
**Status:** Proposed roadmap refresh

## Current Reality

The checked-in `.planning/ROADMAP.md` is stale. It still describes Phase 2.1 as planned and only lists Phase 2.1 through Phase 2.3, while current implementation artifacts show:

- 02-01 multi-provider health-aware routing has a plan.
- 02-02 Go baseline has been verified.
- 02-03 native Anthropic/Gemini adapters are complete.
- 02-04 provider reliability/error contract is complete.
- 02-05 retry/fallback execution is complete and verified 12/12.
- 02-06 active health probing has context and should be the next implementation step.

Phase 2 should now be treated as the provider/adapter foundation milestone. The remaining tasks should finish the provider runtime, adapter contract, config shape, model metadata, and conformance testing needed before moving to durable control state, streaming, rate limiting, cache, or cost governance.

## Guiding Decisions

- Finish provider- and adapter-related foundation work inside Phase 2.
- Split the remaining work into small decimal phases: 02-06, 02-07, 02-08, 02-09, and 02-10.
- Keep Phase 2 in-process and static-config based unless a specific step says otherwise.
- Do not introduce Redis, PostgreSQL, Admin API, Admin Console UI, runtime CRUD, full circuit breaker state machine, streaming, rate limiting, semantic cache, or cost governance in Phase 2.
- When a later Admin Console is expected to call an operation, expose a clean internal service boundary and reusable config structs now, but do not build the console/API yet.

## Recommended Phase Sequence

### Phase 02-06: Active Provider Health Probing and Recovery

**Goal:** Add configurable in-process provider health checks so provider health can recover without waiting for live client traffic.

**Why now:** 02-05 fallback depends on health state, but current health state is only updated by real requests. A provider can become unhealthy and stay unroutable until traffic or restart changes it.

**Must build:**

- Health-check config structs and JSON fields:
  - `health_check.enabled`
  - `health_check.interval`
  - `health_check.timeout`
  - `health_check.initial_delay`
  - `health_check.failure_threshold`
  - `health_check.success_threshold`
  - `health_check.stale_after`
  - `health_check.max_concurrency`
- Optional per-provider overrides for immediately useful fields, such as enabled, interval, timeout, and thresholds.
- In-process prober service with deterministic methods:
  - `ProbeOnce(ctx)`
  - `ProbeProvider(ctx, providerID)`
  - `Start(ctx)` or equivalent context-bound ticker loop.
- Probe result integration with `health.Store` so success can recover an unhealthy provider.
- Probe metadata in health snapshots only as needed for readiness/debugging.
- `/readyz` secret-safe probe visibility.
- Tests for config defaults, probe success/failure, timeout/cancellation, recovery, readiness, and routing after recovery.

**Must not build:**

- Admin Console or Admin API endpoint.
- Redis-backed health.
- Full circuit breaker states.
- Expensive synthetic chat/model probes.

**Exit criteria:**

- A provider marked unhealthy by request failures can become routable again after successful health checks.
- Health-check parameters are structured and reusable for future Admin Console calls.

### Phase 02-07: Provider Adapter Capability Contract

**Goal:** Make every provider adapter describe what it supports in a provider-neutral way.

**Why next:** The gateway now has OpenAI-compatible, Anthropic, and Gemini adapters. Before adding more features, routing and future clients need a stable way to know provider capabilities without provider-specific branches.

**Must build:**

- Extend adapter contract or registry metadata with provider capabilities:
  - provider type.
  - supported operations, initially `chat_completions`.
  - supported modalities, initially text input/output.
  - streaming support flag, initially false unless implemented later.
  - tool/function calling support flag, initially false.
  - max request/body constraints if available from static config.
  - generation parameter support, such as temperature and max tokens.
- Keep SDK-native details inside adapter packages.
- Expose capability metadata through internal registry APIs.
- Ensure `/v1/models` or readiness can optionally include safe capability metadata only where compatible.
- Unit tests proving OpenAI-compatible, Anthropic, and Gemini adapters report consistent metadata.

**Must not build:**

- Streaming implementation.
- Tool/function calling implementation.
- Multimodal implementation.
- Live model discovery.

**Exit criteria:**

- The gateway can ask adapters what they support without importing provider-specific packages outside app wiring.

### Phase 02-08: Provider Configuration Schema and Secret-Safe Validation

**Goal:** Harden static provider configuration into a stable schema that can later become the Admin API/Admin Console contract.

**Why next:** Phase 2 has grown config fields organically. Before durable control state, provider config should be explicit, validated, documented, and safe.

**Must build:**

- Extract config structs for:
  - provider identity and type.
  - base URL and auth reference.
  - models and default model.
  - timeout and health-check overrides.
  - retry/fallback settings.
  - capability overrides if needed.
- Validate:
  - duplicate provider IDs.
  - unknown provider types.
  - missing/invalid base URL.
  - missing models/default model mismatch.
  - invalid durations.
  - invalid health-check thresholds.
  - fallback attempts vs provider count.
- Avoid logging or returning API keys.
- Add example multi-provider config covering OpenAI-compatible, Anthropic, and Gemini.
- Update README config section.
- Tests for config validation and backwards compatibility.

**Must not build:**

- Database-backed provider config.
- Secret manager integration.
- Runtime config hot reload.
- Admin API CRUD.

**Exit criteria:**

- Static config is clear enough to serve as the seed for a later provider-management API.

### Phase 02-09: Provider Model Catalog and Routing Eligibility

**Goal:** Build an internal model catalog that maps models to providers and provider capabilities.

**Why next:** Routing currently uses provider model lists, but future adapter routing needs clearer model/provider eligibility, especially when providers share model names or expose different capabilities.

**Must build:**

- Internal catalog derived from static provider config and adapter capability metadata.
- Stable APIs for:
  - list all models.
  - list providers for a requested model.
  - check whether a provider supports a requested operation/capability.
  - choose default model per provider where relevant.
- `/v1/models` remains OpenAI-compatible while internal catalog can carry richer metadata.
- Routing uses catalog eligibility before provider selection if that reduces duplicated checks.
- Tests for shared model names, provider-specific models, unknown models, and capability-ineligible providers.

**Must not build:**

- Live model discovery from provider APIs.
- Cost-aware model selection.
- Model substitution/degradation.
- Embeddings or images APIs.

**Exit criteria:**

- Provider selection is grounded in a single model/provider eligibility source rather than scattered model checks.

### Phase 02-10: Adapter Conformance Test Harness

**Goal:** Create reusable conformance tests that every current and future provider adapter must pass.

**Why last in this provider foundation slice:** After capabilities, config, health checks, and model catalog are stable, conformance tests lock down the adapter contract so future providers do not regress the gateway.

**Must build:**

- Shared test helpers for adapter behavior:
  - request mapping sanity.
  - response normalization into `llm.LLMResponse`.
  - finish reason mapping.
  - structured provider error categories.
  - health-check behavior.
  - no secret/raw prompt leakage in returned errors.
- Apply harness to OpenAI-compatible, Anthropic, and Gemini adapters.
- Keep provider-specific tests for SDK-specific details where useful.
- Add a short adapter authoring guide in README or docs.

**Must not build:**

- New provider adapters just to prove the harness.
- External provider calls.
- Golden tests that require real API credentials.

**Exit criteria:**

- A future provider adapter has a clear test contract before it can be registered.

## Suggested ROADMAP.md Refresh

Update the Phase 2 table and details to reflect the actual state:

| Phase | Name | Status |
|-------|------|--------|
| 2.1 | Health-Aware Multi-Provider Routing | Complete |
| 2.2 | Go Version Baseline for Official Provider SDKs | Complete |
| 2.3 | Native Anthropic and Gemini Provider Adapters | Complete |
| 2.4 | Provider Reliability and Error Contract | Complete |
| 2.5 | Provider Retry and Fallback Execution | Complete |
| 2.6 | Active Provider Health Probing and Recovery | Next |
| 2.7 | Provider Adapter Capability Contract | Proposed |
| 2.8 | Provider Configuration Schema and Secret-Safe Validation | Proposed |
| 2.9 | Provider Model Catalog and Routing Eligibility | Proposed |
| 2.10 | Adapter Conformance Test Harness | Proposed |

Phase 3 should remain durable control state. Phase 4 should remain streaming/rate/cache/cost work.

## Deferred Beyond Phase 2

- Admin Console UI.
- Admin API provider CRUD.
- PostgreSQL provider/config state.
- Redis-backed health and distributed circuit breaker state.
- Runtime config hot reload.
- Full circuit breaker state machine.
- SSE streaming proxy.
- Tool/function calling normalization.
- Multimodal APIs.
- Embeddings API.
- Rate limiting and admission queues.
- Semantic cache.
- Cost accounting/governance.
- Prometheus/OpenTelemetry exporters.

## Next Step

Plan and implement **02-06 Active Provider Health Probing and Recovery** first.

Run:

```text
$gsd-plan-phase 02-06
```

If the GSD phase tool cannot resolve `02-06`, update `.planning/ROADMAP.md` first using the suggested refresh above or add the decimal phase through the GSD phase workflow.
