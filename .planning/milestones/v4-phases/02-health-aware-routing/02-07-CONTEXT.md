# Phase 2.7: Provider Adapter Capability Contract - Context

**Gathered:** 2026-06-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.7 should make provider capabilities explicit and provider-neutral across the OpenAI-compatible, Anthropic, and Gemini adapters.

Phase 2.1 introduced multi-provider routing and health-aware selection. Phase 2.3 added native Anthropic and Gemini adapters. Phase 2.4 standardized provider error behavior. Phase 2.5 added retry/fallback execution. Phase 2.6 added active provider health probing and recovery. The next foundation gap is that the gateway can call adapters, list their models, and probe their health, but it cannot ask them what operations, modalities, generation parameters, or future feature flags they support without provider-specific knowledge.

This phase should add metadata and internal APIs only. It must not implement streaming, tool/function calling, multimodal requests, live model discovery, Admin API, runtime CRUD, Redis, PostgreSQL, rate limiting, semantic cache, or cost governance.
</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.7 should define a provider-neutral capability contract for adapters.
- **D-02:** Capabilities should describe what the adapter supports; they should not enable new request behavior by themselves.
- **D-03:** Current adapters should report conservative capabilities based on implemented behavior, not provider marketing claims.
- **D-04:** Provider-native SDK details must remain inside adapter packages.

### Capability Shape
- **D-05:** Capability metadata should include provider type.
- **D-06:** Supported operations should initially include `chat_completions`.
- **D-07:** Supported modalities should initially include text input and text output.
- **D-08:** Streaming support should be represented as a flag and remain false until streaming is implemented.
- **D-09:** Tool/function calling support should be represented as a flag and remain false.
- **D-10:** Multimodal support should be represented explicitly if a type is added, but current adapters should report text-only behavior.
- **D-11:** Generation parameter support should include implemented parameters such as `temperature` and `max_tokens`.
- **D-12:** Static request/body constraints may be represented when available from config or constants, but this phase should not infer live provider limits.

### Contract Placement
- **D-13:** Prefer placing common capability types in `internal/providers` so registry, routing, handlers, and future catalogs can consume them without importing provider-specific packages.
- **D-14:** Either extend `ProviderAdapter` with `Capabilities()` or add a small optional interface with a safe default. The executor should choose the lower-risk migration for current adapters and tests.
- **D-15:** The registry should expose capability metadata through stable APIs, such as lookup by provider ID and list-all metadata.
- **D-16:** Capability metadata should be immutable or copied defensively so callers cannot mutate adapter internals.

### Visibility
- **D-17:** `/v1/models` must remain OpenAI-compatible and should not gain incompatible fields.
- **D-18:** `/readyz` may include safe provider capability summaries only if that stays compact and secret-safe; otherwise keep capability metadata internal for Phase 2.7.
- **D-19:** Logs, errors, readiness, and tests must not expose API keys, authorization headers, raw prompts, raw upstream bodies, or full provider error payloads.

### Testing
- **D-20:** Unit tests should prove OpenAI-compatible, Anthropic, and Gemini adapters report consistent metadata.
- **D-21:** Registry tests should prove capabilities can be listed and fetched without provider-specific imports.
- **D-22:** Tests should assert conservative false values for unimplemented streaming/tools/multimodal capabilities.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Context
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md` - Phase 2 routing, health, and deferred scope.
- `.planning/phases/02-health-aware-routing/02-FOLLOWUP-ROADMAP.md` - Follow-up roadmap naming Phase 2.7 and its scope.
- `.planning/phases/02-health-aware-routing/02-05-SUMMARY.md` - Completed retry/fallback behavior summary.
- `.planning/phases/02-health-aware-routing/02-06-CONTEXT.md` - Active health probing decisions.
- `.planning/phases/02-health-aware-routing/02-06-PLAN.md` - Active health probing implementation plan.
- `.planning/phases/02-health-aware-routing/02-06-UAT.md` - Verification record confirming Phase 2.6 passes 15/15 checks.

### Current Code Integration Points
- `internal/providers/adapter.go` - Existing provider adapter contract with `ID`, `Models`, `Complete`, and `HealthCheck`.
- `internal/providers/registry.go` - Stable provider listing and lookup boundary.
- `internal/providers/openai/adapter.go` - OpenAI-compatible adapter behavior and supported parameters.
- `internal/providers/anthropic/adapter.go` - Anthropic native adapter behavior and supported parameters.
- `internal/providers/gemini/adapter.go` - Gemini native adapter behavior and supported parameters.
- `internal/config/config.go` - Static provider types and any future request/body limit config.
- `internal/http/handlers/models.go` - OpenAI-compatible model listing boundary.
- `internal/http/handlers/health.go` - Secret-safe readiness boundary.
- `internal/app/app.go` - Provider-specific adapter wiring location.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `providers.ProviderAdapter` is already the shared adapter interface.
- Current adapters already know their provider ID and configured model list.
- The registry already preserves stable provider ordering through `ids`.
- Current adapters support non-streaming chat completions and map `temperature` and `max_tokens` where available.
- `/v1/models` already gets deduplicated model IDs through gateway service/registry behavior.

### Gaps
- The adapter contract has no capability metadata.
- Router, registry, readiness, and future model catalog work cannot query supported operations or feature flags.
- Current provider types are known in config/app wiring, but not exposed as provider-neutral adapter metadata.
- Streaming, tools, multimodal support, and live model discovery are not implemented and must remain false/absent.

### Integration Points
- Add capability types and constants in `internal/providers`.
- Add `Capabilities()` to adapters directly or through a compatibility interface plus registry defaults.
- Add registry APIs for capability lookup and stable list output.
- Add unit tests beside each adapter package and in registry tests.
- Optionally add compact readiness capability visibility if it stays OpenAI-compatible elsewhere and secret-safe.
</code_context>

<must_build>
## Must Build In Phase 2.7

- Provider-neutral capability structs/types.
- Provider type metadata for current adapters.
- Supported operation metadata, initially `chat_completions`.
- Supported modality metadata, initially text input/output.
- Explicit false flags for streaming and tool/function calling.
- Generation parameter support metadata for implemented parameters.
- Optional static request/body constraint metadata if available without live discovery.
- Adapter capability reporting for OpenAI-compatible, Anthropic, and Gemini.
- Registry APIs to fetch/list provider capabilities without provider-specific imports.
- Secret-safe tests covering adapter and registry capability metadata.

</must_build>

<deferred>
## Deferred Ideas

- SSE streaming implementation.
- Tool/function calling request/response normalization.
- Multimodal input/output support.
- Live model discovery from provider APIs.
- Provider model catalog and routing eligibility; planned for Phase 2.9.
- Adapter conformance harness; planned for Phase 2.10.
- Admin API or Admin Console capability endpoints.
- PostgreSQL or Redis-backed provider metadata.
- Runtime config CRUD or hot reload.
- Rate limiting, semantic cache, cost governance, and Prometheus/OpenTelemetry exporters.
</deferred>

<success_criteria>
## Success Criteria

- The gateway can ask each adapter for provider-neutral capability metadata.
- OpenAI-compatible, Anthropic, and Gemini adapters report conservative, consistent capabilities.
- Registry callers can list and resolve capabilities without importing provider-specific packages outside app wiring.
- `/v1/models` remains OpenAI-compatible.
- Any readiness/debug capability visibility is secret-safe and compact.
- Tests prove current adapters report `chat_completions`, text input/output, supported generation parameters, and false values for unimplemented streaming/tools/multimodal features.
- No streaming, tools, multimodal, live model discovery, Admin API, Redis/PostgreSQL, runtime CRUD, rate limiting, semantic cache, or cost governance is introduced.
</success_criteria>

---

*Phase: 2.7-Provider Adapter Capability Contract*
*Context gathered: 2026-06-16*
