# Phase 3: Durable Control State - Context

**Gathered:** 2026-06-18
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 3 introduces durable control state for VeloxMesh provider configuration. It replaces Phase 2's transitional static provider config as the normal production control surface with database-backed provider records, encrypted provider secrets, a versioned Admin API, runtime provider activation, and Redis-backed hot state where configured.

The phase should preserve the Go/Chi gateway, the OpenAI-compatible data-plane contract, and provider adapter isolation. The initial durable scope is provider configuration first: provider connection details, encrypted upstream credentials, optional model config, runtime activation, test-connection support, minimal audit records, and hot-state coordination. It must not implement semantic cache, rate limiting, cost governance, streaming, full user/session admin auth, or broad usage analytics.

</domain>

<decisions>
## Implementation Decisions

### Source-of-Truth Transition
- **D-01:** Normal project initialization should create only the core database table structures needed by VeloxMesh; it should not configure provider records, provider names, endpoints, API keys, model lists, or provider options.
- **D-02:** Provider-related information must be entered by the user at runtime through the control surface.
- **D-03:** Provider-dependent functions called before required provider configuration exists must return clear actionable errors naming the missing provider configuration.
- **D-04:** PostgreSQL is the primary durable control-state backend.
- **D-05:** SQLite should be available as a lightweight durable replacement backend for simpler/local deployments.
- **D-06:** Storage backends should expose explicit capability profiles. PostgreSQL can support durable/distributed control-state features; SQLite should advertise local durable config only, with advanced/distributed features such as future semantic cache disabled or rejected.
- **D-07:** Static/env config remains as a local development seed/convenience path only, not the production initialization path.
- **D-08:** If local-dev seeding is used and durable records already exist, durable storage wins. Startup must not silently overwrite provider records from static config.

### Control-State Data Model
- **D-09:** Phase 3 should persist provider configuration first. Usage records and broader control-plane expansion can wait.
- **D-10:** Upstream provider secrets should be stored as encrypted secret values in durable storage. Secret values must remain redacted in API responses, validation errors, logs, docs, tests, and audit records.
- **D-11:** Provider records should be valid on save for their current required contract; incomplete required connection fields should be rejected rather than stored as drafts.
- **D-12:** The initial required provider save contract should be minimal provider connection data: provider name or ID, provider type, base URL, and encrypted API key or equivalent required credential.
- **D-13:** Optional model configuration should be persisted when provided, including models and default model, but should not be mandatory for first provider save.
- **D-14:** "Valid on save" means the required minimal connection fields are valid and secret-safe. Model/default completeness is optional unless a provider-dependent operation requires it.

### Admin API Behavior
- **D-15:** Admin API provider create/update should validate required fields, persist durable config, refresh runtime provider registry/config, and make the provider eligible immediately without restart.
- **D-16:** Phase 3 Admin API should expose provider CRUD plus a test-connection action.
- **D-17:** The test-connection response contract should support an Admin Console provider configuration screen with a Test Connection button and actionable success/failure details.
- **D-18:** Admin API should use a dedicated admin bearer token separate from the data-plane development API key.
- **D-19:** Provider removal should support disable first, with hard delete allowed later only when safe. Runtime routing must immediately ignore disabled providers.
- **D-20:** Create/update validation failures should return structured field errors with stable codes and per-field messages suitable for API clients and Admin Console forms.
- **D-21:** Phase 3 should include a minimal audit table for provider create, update, disable, delete, and test events. Audit records should include actor label, timestamp, target provider ID, action, outcome, and no secrets.
- **D-22:** Provider changes should use transactional save plus runtime reload. If runtime activation fails, the config change should not commit and the API should return a structured actionable error.
- **D-23:** Provider secret rotation should replace the encrypted stored secret on update when a new `api_key` is provided; omitted secret means keep the existing encrypted secret. No secret versioning is required in Phase 3.
- **D-24:** `test connection` should validate base URL/auth and perform the cheapest safe provider call available, without sending user prompts.
- **D-25:** Admin routes should be versioned, for example `/admin/v1/providers`.
- **D-26:** Provider updates should use optimistic concurrency. Provider records should expose a version/revision, and stale updates should fail with a structured conflict error.
- **D-27:** Audit records should have configurable retention and safe purge behavior with conservative local/dev defaults.
- **D-28:** Admin API mutations should support idempotency keys for create, update, delete, disable, and test actions.
- **D-29:** Provider list/read responses must never return raw secrets or encrypted ciphertext. They may return redacted secret metadata such as whether a secret is configured and last-updated metadata.
- **D-30:** Provider listing should support basic filters for enabled/disabled state, provider type, and search by name/ID.

### Redis Boundary
- **D-31:** Phase 3 should use Redis, when configured, for provider health/probe hot state and gateway API-key/auth cache hot state.
- **D-32:** Semantic cache, rate limits, and cost governance remain out of scope.
- **D-33:** Redis should be optional by backend capabilities. PostgreSQL/SQLite durable config can run without Redis; Redis enables distributed hot-state features when configured.
- **D-34:** If Redis is configured but unavailable, gateway should degrade to process-local hot state where safe, start with clear warnings, and avoid enabling features that require distributed guarantees.
- **D-35:** Redis must not store provider API keys, decrypted credentials, or encrypted secret blobs. Provider secrets stay encrypted in durable storage.
- **D-36:** Redis keys should include a configurable namespace/environment prefix to avoid collisions across deployments.
- **D-37:** Redis health snapshots and auth cache entries should have explicit TTLs/staleness windows.
- **D-38:** When Redis is configured, Admin API changes should publish Redis pub/sub config-change notifications so other instances refresh runtime provider state.
- **D-39:** Without Redis, Phase 3 should document single-instance/local runtime consistency. Runtime reload is guaranteed only for the instance handling the Admin API call; Redis is the supported path for multi-instance runtime consistency.

### the agent's Discretion
- Planner may choose concrete package names, repository interfaces, migration tooling, encryption implementation details, and exact payload schemas as long as the decisions above are preserved and tested.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and Phase Context
- `.planning/PROJECT.md` - Project purpose, constraints, and Go-first gateway decision.
- `.planning/REQUIREMENTS.md` - Requirements registry and deferred scope boundaries.
- `.planning/ROADMAP.md` - Phase 3 goal and success criteria.
- `.planning/STATE.md` - Current project state.
- `.planning/phases/02-health-aware-routing/02-08-CONTEXT.md` - Static provider config schema and secret-safe validation decisions.
- `.planning/phases/02-health-aware-routing/02-09-CONTEXT.md` - Model catalog and routing eligibility decisions.
- `.planning/phases/02-health-aware-routing/02-10-CONTEXT.md` - Adapter conformance and secret-safe provider behavior decisions.

### Current Code Integration Points
- `internal/config/config.go` - Current static/env config structs, validation, provider auth references, and local-dev fallback.
- `internal/app/app.go` - Current adapter wiring from config into registry/router/gateway service.
- `internal/gateway/service.go` - Request-time routing, fallback, metrics, and provider health update behavior.
- `internal/health/store.go` - Current in-memory provider health and probe snapshot state.
- `internal/health/prober.go` - Current active health probe lifecycle.
- `internal/providers/registry.go` - Provider registry, stable ordering, capabilities, and catalog integration.
- `internal/providers/catalog.go` - Current model/provider eligibility source.
- `internal/routing/router.go` - Current routing selection and strict override behavior.
- `internal/errors/errors.go` - Shared structured error categories and retry/health impact semantics.
- `internal/http/router.go` - Current HTTP route wiring and the future admin route integration point.
- `internal/http/middleware/auth.go` - Current data-plane auth pattern; Admin API must use a separate admin token.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config.Config` and `ProviderConfig` already define most of the provider fields that need a durable representation: provider ID/type, base URL, API key/auth reference, models, default model, timeout, weight, and health-check override.
- `ProviderConfig.ResolveAPIKey()` already centralizes current secret resolution, but Phase 3 should replace production secret sourcing with encrypted durable secret retrieval.
- `app.New` is the current composition boundary for loading config, constructing adapters, registry, health store, prober, router, and gateway service.
- `health.Store` is already an interface, which makes Redis-backed or hybrid health state feasible without rewriting routing callers.
- Provider adapter conformance and structured gateway errors already establish secret-safe behavior patterns that should be preserved in Admin API validation and test-connection responses.

### Established Patterns
- Static provider config is explicitly transitional from Phase 2 and should not be over-optimized as long-term production control state.
- Runtime provider selection is provider-neutral; provider-native request/response mapping belongs in adapter packages.
- Public `/v1/models` and `/v1/chat/completions` must remain OpenAI-compatible even as control-plane state changes.
- Tests should run locally without real provider credentials or external network calls unless a specific test-connection integration is isolated behind fakes.

### Integration Points
- Add durable storage abstractions below config/app wiring so `app.New` can assemble providers from durable state in production and from static/env config only in local dev.
- Add a storage backend capability surface for PostgreSQL, SQLite, and optional Redis integration.
- Add Admin API routes under a versioned admin path, protected by a dedicated admin bearer token.
- Add runtime registry/provider reload support that can apply validated provider changes transactionally.
- Add Redis implementations or adapters for provider health/probe state, auth cache hot state, and config-change pub/sub notifications while preserving local fallbacks.

</code_context>

<specifics>
## Specific Ideas

- Admin Console provider configuration should have a Test Connection button backed by the Admin API test-connection action.
- SQLite is intentionally constrained: it is a supported lightweight durable backend, but advanced/distributed features should be disabled or rejected through backend capability checks.
- Provider-dependent errors before configuration should name what is missing, such as no active provider configured, missing API key, missing base URL, or missing model config when the operation needs a model.

</specifics>

<deferred>
## Deferred Ideas

- Semantic cache implementation.
- Rate limiting and admission quota policies.
- Cost governance and usage aggregation beyond any minimal schema placeholders required later.
- SSE streaming proxy.
- Full Admin Console implementation if the current phase is backend-only; the API contract should still support the provider configuration UI.
- Admin users, sessions, roles, RBAC, or a full identity system.
- Secret manager integration and secret version rollback.
- Live model discovery unless needed only as a safe optional test/validation behavior.

</deferred>

---

*Phase: 3-Durable Control State*
*Context gathered: 2026-06-18*
