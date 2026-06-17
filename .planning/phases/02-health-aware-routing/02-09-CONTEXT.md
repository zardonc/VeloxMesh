# Phase 2.9: Provider Model Catalog and Routing Eligibility - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.9 should build an internal model catalog that maps configured models to eligible providers and provider-neutral capabilities.

Phase 2.1 introduced multi-provider routing and model aggregation. Phase 2.7 added provider capability metadata on adapters and registry APIs. Phase 2.8 hardened static provider configuration. The current runtime can list all configured models and route among healthy providers, but it does not have a single source of truth for whether a provider is eligible for a requested model and operation.

This phase should introduce catalog and eligibility behavior only. It must not add live model discovery, cost-aware routing, model substitution/degradation, embeddings/images APIs, streaming, tool calling, Admin API/Admin Console CRUD, PostgreSQL/Redis state, runtime hot reload, rate limiting, semantic cache, or cost governance.
</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.9 should create an internal model/provider catalog derived from static provider config and adapter capability metadata.
- **D-02:** The catalog should become the single internal source for model-to-provider and operation eligibility decisions.
- **D-03:** The catalog should not discover models from live provider APIs.
- **D-04:** The catalog should not substitute, alias, degrade, or rewrite client-requested model names.

### Catalog Shape
- **D-05:** Catalog entries should retain model ID, provider ID, provider type, default-model status where applicable, and provider-neutral capability metadata.
- **D-06:** Shared model names across providers should be represented as one model ID with multiple eligible providers, not duplicated externally in `/v1/models`.
- **D-07:** Provider-specific model names should remain available only for the providers that configured them.
- **D-08:** Catalog APIs should return defensive copies so callers cannot mutate internal provider/model metadata.
- **D-09:** Catalog construction should preserve stable provider ordering from the registry/config.

### Eligibility APIs
- **D-10:** The gateway should be able to list all OpenAI-compatible model IDs through the catalog.
- **D-11:** The gateway should be able to list eligible providers for a requested model.
- **D-12:** The gateway should be able to check whether a provider supports a requested operation/capability, initially `chat_completions`.
- **D-13:** Unknown requested models should produce a deterministic no-eligible-provider style failure before provider execution.
- **D-14:** Capability-ineligible providers should be excluded before health/latency strategy selection.

### Routing Integration
- **D-15:** Routing should use catalog eligibility before applying health filtering and routing strategy.
- **D-16:** `X-Route-To` strict override should still be strict, but it must fail if the chosen provider is not eligible for the requested model/operation.
- **D-17:** Fallback should exclude previously failed providers and catalog-ineligible providers using the same eligibility source.
- **D-18:** Routing strategy behavior (`round-robin`, `least-latency`, cold-start fallback) should remain unchanged after eligibility candidates are selected.

### Surface Compatibility
- **D-19:** `/v1/models` must remain OpenAI-compatible: `object: "list"` with model items, no incompatible provider metadata in the public response.
- **D-20:** Richer catalog metadata should stay internal unless a safe internal/debug surface already exists and remains compatible.
- **D-21:** Errors, logs, readiness, and tests must not expose API keys, authorization headers, raw prompts, raw upstream bodies, or sensitive provider payloads.

### Testing
- **D-22:** Tests should cover shared model names across providers.
- **D-23:** Tests should cover provider-specific model names.
- **D-24:** Tests should cover unknown model requests.
- **D-25:** Tests should cover capability-ineligible providers.
- **D-26:** Tests should prove `/v1/models` remains deduplicated and OpenAI-compatible.
- **D-27:** Full verification should include `gofmt -l .`, `go vet ./...`, and `go test ./...`.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and Phase Context
- `.planning/PROJECT.md` - Project-level purpose and constraints.
- `.planning/REQUIREMENTS.md` - Requirements registry.
- `.planning/ROADMAP.md` - Phase 2.9 goal and success criteria.
- `.planning/STATE.md` - Current milestone state.
- `.planning/phases/02-health-aware-routing/02-FOLLOWUP-ROADMAP.md` - Follow-up roadmap naming Phase 2.9 and its scope.
- `.planning/phases/02-health-aware-routing/02-07-CONTEXT.md` - Capability metadata decisions.
- `.planning/phases/02-health-aware-routing/02-07-01-PLAN.md` - Adapter capability contract plan.
- `.planning/phases/02-health-aware-routing/02-07-02-PLAN.md` - Capability exposure plan.
- `.planning/phases/02-health-aware-routing/02-08-CONTEXT.md` - Provider config schema decisions.
- `.planning/phases/02-health-aware-routing/02-08-PLAN.md` - Config schema and validation plan.
- `.planning/phases/02-health-aware-routing/02-08-SUMMARY.md` - Implemented config schema summary.
- `.planning/phases/02-health-aware-routing/02-08-UAT.md` - Phase 2.8 UAT results.

### Current Code Integration Points
- `internal/providers/registry.go` - Current registry model aggregation and capability APIs.
- `internal/providers/capabilities.go` - Provider-neutral capability types.
- `internal/providers/adapter.go` - Adapter contract.
- `internal/routing/router.go` - Health-aware provider selection and routing strategy.
- `internal/gateway/service.go` - Chat completion execution, fallback loop, model/capability surfaces.
- `internal/http/handlers/models.go` - OpenAI-compatible `/v1/models` response.
- `internal/http/handlers/chat.go` - Chat request parsing into `llm.LLMRequest`.
- `internal/llm/types.go` - Request model and operation context.
- `internal/config/config.go` - Static provider model config and defaults.
- `internal/providers/registry_test.go` - Registry/capability test patterns.
- `internal/routing/router_test.go` - Router behavior test patterns.
- `internal/gateway/service_test.go` - Gateway service test patterns.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `providers.ProviderAdapter.Models()` already returns configured provider model IDs.
- `providers.ProviderAdapter.Capabilities()` already returns provider-neutral operation/modality/feature metadata.
- `providers.Registry` preserves stable provider ordering through `ids`.
- `providers.Registry.GetAllModels()` already deduplicates model IDs in stable provider order.
- `providers.Registry.AllCapabilities()` returns provider IDs, model slices, and copied capabilities.
- `routing.HealthAwareRouter` already centralizes provider selection and strict override handling.
- `gateway.Service` already delegates model listing and capability listing to the router.
- `/v1/models` already emits an OpenAI-compatible model list.

### Gaps
- Model aggregation and provider eligibility are not represented as a reusable catalog.
- Router currently filters by health and exclusions only; it does not check requested model support before selecting a provider.
- Strict override can select a provider that does not support the requested model.
- Fallback can attempt providers that are healthy but model-ineligible.
- There is no internal API for "providers eligible for model X and operation Y".
- Unknown requested models do not have a dedicated deterministic eligibility failure path before provider execution.

### Integration Points
- Add a provider-neutral catalog package or file in an existing internal boundary, likely `internal/providers` or a new `internal/catalog`, avoiding provider-specific adapter imports.
- Wire the catalog into `routing.HealthAwareRouter` so candidate selection is eligibility-first, then health/strategy.
- Keep `/v1/models` public response compatible by sourcing model IDs from the catalog without exposing provider metadata.
- Add focused unit tests around catalog construction and router eligibility before touching broader gateway behavior.
</code_context>

<must_build>
## Must Build In Phase 2.9

- Internal catalog derived from configured adapter models and provider-neutral capability metadata.
- Stable APIs for:
  - listing all model IDs.
  - listing eligible providers for a model and operation.
  - checking provider eligibility for a model and operation/capability.
  - exposing provider default-model information where relevant and already available.
- Router integration so model/capability eligibility filters candidates before health and routing strategy.
- Strict override behavior that fails when the requested provider is model/capability ineligible.
- `/v1/models` remains OpenAI-compatible and deduplicated.
- Tests for shared model names, provider-specific models, unknown models, capability-ineligible providers, strict override eligibility, fallback eligibility, copy safety, and public model response compatibility.
</must_build>

<deferred>
## Deferred Ideas

- Live model discovery from provider APIs.
- Cost-aware model/provider selection.
- Model aliasing, substitution, degradation, or automatic provider model rewriting.
- Embeddings, images, multimodal, streaming, or tool/function calling APIs.
- Admin API or Admin Console provider/model management.
- PostgreSQL or Redis-backed model catalog state.
- Runtime config hot reload or provider CRUD.
- Rate limiting, semantic cache, cost governance, and observability exporters.
</deferred>

<success_criteria>
## Success Criteria

- Catalog derives model/provider eligibility from static config and adapter capability metadata.
- Routing asks one source whether a provider supports a requested model and operation before selecting candidates.
- Shared model names and provider-specific model names are represented correctly.
- Unknown models and capability-ineligible providers are rejected deterministically before provider execution.
- `/v1/models` remains OpenAI-compatible and deduplicated.
- Tests cover shared model names, provider-specific models, unknown models, capability-ineligible providers, strict override eligibility, fallback eligibility, copy safety, and public model response compatibility.
- `gofmt -l .`, `go vet ./...`, and `go test ./...` pass after implementation.
- No live discovery, cost-aware routing, model substitution/degradation, embeddings/images APIs, Admin API/Admin Console CRUD, database-backed catalog, streaming, rate limiting, semantic cache, or cost governance is introduced.
</success_criteria>

---

*Phase: 2.9-Provider Model Catalog and Routing Eligibility*
*Context gathered: 2026-06-17*
