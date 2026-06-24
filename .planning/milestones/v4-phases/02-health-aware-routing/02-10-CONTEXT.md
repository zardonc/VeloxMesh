# Phase 2.10: Adapter Conformance Test Harness - Context

**Gathered:** 2026-06-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.10 should create a reusable conformance test harness for provider adapters so every current and future adapter can be checked against the same provider-neutral contract.

Previous Phase 2 work added multi-provider routing, native Anthropic and Gemini adapters, structured provider errors, retry/fallback, active health probing, capability metadata, static provider configuration, and model/provider routing eligibility. The current adapter tests are provider-specific and repeat similar assertions for capabilities, request mapping, response normalization, finish reasons, and error categories. This phase should consolidate those shared expectations into reusable Go test helpers while keeping SDK-specific tests close to each adapter package.

This phase is testing infrastructure and contract coverage only. It must not add new provider adapters, live provider discovery, Admin API/Admin Console runtime configuration, streaming, tools, multimodal behavior, rate limiting, semantic cache, cost governance, PostgreSQL, Redis, or runtime provider CRUD.
</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.10 should build a reusable Go test harness for the `providers.ProviderAdapter` contract.
- **D-02:** The harness should prove provider-neutral adapter behavior, not introduce new runtime provider behavior.
- **D-03:** OpenAI-compatible, Anthropic, and Gemini adapters should each run through the harness.
- **D-04:** Provider-specific tests should remain where SDK details, native request shapes, or provider-specific edge cases matter.
- **D-05:** The harness should make the future-adapter test contract clear before an adapter is registered for routing.

### Harness Shape
- **D-06:** The harness should live in a shared test-support location that avoids production imports of test-only helpers.
- **D-07:** The harness should expose a compact adapter fixture/spec type that each adapter package can instantiate with provider-specific fake upstream behavior.
- **D-08:** Harness APIs should be idiomatic Go tests using `testing.T`, table-driven cases, and deterministic fake servers or fake SDK responses.
- **D-09:** Harness tests should run locally without real provider credentials and without external network calls.
- **D-10:** The harness should support provider-specific setup hooks when an adapter needs an `httptest.Server`, mocked client behavior, or provider-native response payload.

### Contract Coverage
- **D-11:** Harness coverage should include adapter identity, configured model list, capability metadata, and defensive-copy expectations where applicable.
- **D-12:** Harness coverage should include request mapping sanity for model, user/system/assistant messages, temperature, and max tokens.
- **D-13:** Harness coverage should include normalized `llm.LLMResponse` shape, choice content, response model/provider metadata where applicable, and provider-neutral assistant messages.
- **D-14:** Harness coverage should include finish reason mapping for at least stop, length/max-token, and provider-specific blocked/safety/tool-style cases where supported.
- **D-15:** Harness coverage should include structured `GatewayError` category mapping for auth, rate limit, invalid model/request, timeout/unavailable, and bad response categories where adapters can deterministically produce them.
- **D-16:** Harness coverage should include health-check behavior for available/unavailable states that can be tested without live provider calls.
- **D-17:** Harness coverage should include secret-safe errors: returned errors must not include API keys, authorization headers, raw prompts, raw upstream response bodies, or sensitive provider-native payloads.

### Scope Fences
- **D-18:** The phase should not rewrite adapter implementations except where needed to make existing behavior conform to the shared contract.
- **D-19:** The phase should not replace provider-specific SDK tests with generic tests when native SDK behavior still matters.
- **D-20:** The phase should not add live health checks for Anthropic or Gemini; existing no-live-call health semantics should be captured or explicitly tested as current behavior.
- **D-21:** The phase should not alter the client-facing OpenAI-compatible HTTP API contract.
- **D-22:** The phase should not add Admin Console/database-backed provider configuration; static config remains the Phase 2 control surface.

### Documentation and Verification
- **D-23:** The final plan should include a short adapter authoring/testing guide in README or a local docs file if it helps future adapter authors run the harness.
- **D-24:** Verification should include targeted adapter package tests and full `gofmt -l .`, `go vet ./...`, and `go test ./...`.
- **D-25:** Scope verification should search for accidental live network calls, raw secret exposure, provider-native leakage, and deferred feature implementation.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and Phase Context
- `.planning/PROJECT.md` - Project purpose, constraints, and Go-first gateway decision.
- `.planning/REQUIREMENTS.md` - Requirements registry.
- `.planning/ROADMAP.md` - Phase 2.10 goal and success criteria.
- `.planning/STATE.md` - Current milestone state.
- `.planning/phases/02-health-aware-routing/02-FOLLOWUP-ROADMAP.md` - Phase 2 follow-up roadmap and deferred scope.
- `.planning/phases/02-health-aware-routing/02-07-CONTEXT.md` - Adapter capability contract decisions.
- `.planning/phases/02-health-aware-routing/02-07-01-PLAN.md` - Adapter capability metadata implementation plan.
- `.planning/phases/02-health-aware-routing/02-07-02-PLAN.md` - Capability exposure plan.
- `.planning/phases/02-health-aware-routing/02-08-CONTEXT.md` - Provider config schema and temporary static config decisions.
- `.planning/phases/02-health-aware-routing/02-08-PLAN.md` - Config schema plan.
- `.planning/phases/02-health-aware-routing/02-09-CONTEXT.md` - Model catalog and eligibility decisions.
- `.planning/phases/02-health-aware-routing/02-09-PLAN.md` - Model catalog and routing eligibility plan.
- `.planning/phases/02-health-aware-routing/02-09-SUMMARY.md` - Implemented model catalog summary, if present.

### Current Code Integration Points
- `internal/providers/adapter.go` - `ProviderAdapter` contract.
- `internal/providers/capabilities.go` - Provider-neutral capability metadata.
- `internal/providers/registry.go` - Adapter registration and model/capability surfaces.
- `internal/errors/errors.go` - Shared `GatewayError` categories, retryability, and health-impact semantics.
- `internal/llm/types.go` - Internal request and response types.
- `internal/providers/openai/adapter.go` - OpenAI-compatible adapter implementation.
- `internal/providers/openai/adapter_test.go` - Current OpenAI adapter test patterns.
- `internal/providers/anthropic/adapter.go` - Anthropic adapter implementation.
- `internal/providers/anthropic/adapter_test.go` - Current Anthropic adapter test patterns.
- `internal/providers/gemini/adapter.go` - Gemini adapter implementation.
- `internal/providers/gemini/adapter_test.go` - Current Gemini adapter test patterns.
- `internal/providers/registry_test.go` - Registry and capability test patterns.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `providers.ProviderAdapter` already defines the shared adapter surface: `ID`, `Models`, `Complete`, `HealthCheck`, and `Capabilities`.
- `providers.CapabilitySet` is provider-neutral and already has clone/support helpers.
- `internal/errors.GatewayError` already defines shared provider error category strings and retryability/health-impact helpers.
- OpenAI-compatible adapter tests already use `httptest.Server` to assert parameter forwarding and error category mapping.
- Anthropic and Gemini adapter tests already use fake HTTP responses through their SDK clients' configured base URLs.
- Existing tests already verify core success, finish reason mapping, auth errors, rate-limit errors, and bad response categories in each adapter package.

### Gaps
- Capability assertions are duplicated across adapter test files.
- Request mapping sanity is uneven; OpenAI tests assert temperature and max tokens directly, while native adapter tests focus mostly on normalized responses.
- There is no shared assertion that adapter errors stay secret-safe.
- There is no reusable contract for future adapter authors to run before registration.
- Health-check behavior is not consistently covered across adapters.
- Finish reason coverage differs per adapter and is not expressed as a shared contract.

### Integration Points
- Prefer a test-only helper package such as `internal/providers/adaptertest` or equivalent, keeping production code free of test harness dependencies.
- Let each provider package create its own fake upstream/server and then call shared harness helpers.
- Keep native SDK request-shape assertions provider-specific when the official SDK hides transport details or makes direct request inspection brittle.
- Keep the harness provider-neutral by asserting internal `llm.LLMRequest`, `llm.LLMResponse`, `providers.CapabilitySet`, and `errors.GatewayError` outcomes rather than provider-native SDK structs.
</code_context>

<must_build>
## Must Build In Phase 2.10

- A reusable Go conformance harness for provider adapters.
- Harness fixtures/specs for OpenAI-compatible, Anthropic, and Gemini adapters.
- Shared checks for:
  - adapter ID and model list behavior.
  - capability metadata consistency.
  - request mapping sanity for supported roles and generation parameters.
  - response normalization into `llm.LLMResponse`.
  - finish reason mapping.
  - structured provider error categories.
  - health-check behavior.
  - secret-safe returned errors.
- Adapter package tests that invoke the harness while preserving provider-specific tests for SDK and native payload details.
- A clear future-adapter testing contract in docs or README if useful.
</must_build>

<deferred>
## Deferred Ideas

- New provider adapters.
- Runtime provider configuration through Admin Console or database storage.
- Live provider model discovery.
- Live provider health checks that call Anthropic or Gemini APIs.
- Streaming, tool/function calling, multimodal, embeddings, images, rate limiting, semantic cache, cost governance, Redis, PostgreSQL, or Admin API.
- Moving provider-native request/response mapping out of adapter packages.
</deferred>

<success_criteria>
## Success Criteria

- Shared test helpers cover request mapping sanity, response normalization, finish reason mapping, structured error categories, health-check behavior, and secret-safe errors.
- OpenAI-compatible, Anthropic, and Gemini adapters run through the harness.
- Provider-specific tests remain where SDK details matter.
- A future adapter has a clear test contract before registration.
- The harness runs locally with no real credentials and no external provider calls.
- `gofmt -l .`, `go vet ./...`, and `go test ./...` pass after implementation.
- No Admin API/Admin Console runtime config, database-backed provider config, live discovery, streaming, rate limiting, semantic cache, cost governance, Redis, PostgreSQL, or new provider behavior is introduced.
</success_criteria>

---

*Phase: 2.10-Adapter Conformance Test Harness*
*Context gathered: 2026-06-17*
