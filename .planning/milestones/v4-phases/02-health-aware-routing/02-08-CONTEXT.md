# Phase 2.8: Provider Configuration Schema and Secret-Safe Validation - Context

**Gathered:** 2026-06-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.8 should harden VeloxMesh's static provider configuration into an explicit, reusable schema with strong validation and secret-safe diagnostics.

Phase 2.1 introduced multi-provider static config. Phase 2.3 added `anthropic` and `gemini` provider types. Phase 2.5 added retry/fallback behavior. Phase 2.6 added health-check configuration and provider overrides. Phase 2.7 added provider-neutral capability metadata. The current config has grown organically across those phases, so this phase should make the schema boundaries clear before later durable control state, Admin API, Admin Console, model catalog, or conformance work builds on it.

This phase is still static-config and in-process. It must not add database-backed provider config, secret manager integration, runtime hot reload, Admin API CRUD, Admin Console UI, Redis, PostgreSQL, streaming, rate limiting, semantic cache, or cost governance.
</domain>

<decisions>
## Implementation Decisions

### Phase Focus
- **D-01:** Phase 2.8 should refine the static provider config schema and validation only; it should not add runtime provider management.
- **D-02:** The schema should be clear enough to become the seed for future Admin API/Admin Console provider-management contracts, but no API/UI is built in this phase.
- **D-03:** Existing backward-compatible environment configuration for the single OpenAI-compatible provider must continue to load.
- **D-04:** Provider-native request and response mapping remains inside adapter packages; config schema work should not move SDK-specific behavior into common config.

### Schema Shape
- **D-05:** Provider config should make identity and type explicit: stable provider ID and provider type.
- **D-06:** Endpoint config should require a valid base URL with an HTTP or HTTPS scheme.
- **D-07:** Auth config should move toward an auth reference shape rather than encouraging raw secret exposure in docs, while preserving the existing `api_key` file field if needed for compatibility.
- **D-08:** Model config should include `models` and `default_model`, and validation should require `default_model` to appear in `models` when set.
- **D-09:** Timeout config should parse as Go durations and reject invalid or negative values.
- **D-10:** Health-check global config and provider overrides should continue to use the Phase 2.6 fields and validate durations, thresholds, stale windows, and concurrency.
- **D-11:** Retry/fallback settings should validate `max_attempts` against provider count and fallback state rather than silently normalizing surprising values.
- **D-12:** Capability override schema may be added only if it is needed to safely connect Phase 2.7 capabilities to static config; do not implement new routing behavior through overrides.

### Validation Behavior
- **D-13:** Validation must reject duplicate provider IDs, empty provider IDs, unknown provider types, missing or invalid base URLs, empty model lists, invalid default model references, invalid durations, invalid thresholds, invalid health-check concurrency, and fallback attempts that exceed configured provider count.
- **D-14:** Validation errors should name the failing field and provider ID where useful, but must not echo API keys, authorization headers, raw prompts, raw upstream bodies, or sensitive provider payloads.
- **D-15:** Validation should prefer deterministic, table-testable helper functions over ad hoc checks embedded in `LoadConfig`.
- **D-16:** Defaults should be explicit and covered by tests, especially for env fallback config, health-check defaults, and fallback behavior.

### Documentation and Examples
- **D-17:** README configuration docs should show a multi-provider JSON example covering OpenAI-compatible, Anthropic, and Gemini.
- **D-18:** Documentation examples should use placeholders or auth references that avoid normalizing real secret values in committed docs.
- **D-19:** README must preserve deferred scope notes: no Admin API, Admin Console, Redis/PostgreSQL, streaming, rate limiting, semantic cache, or cost governance in Phase 2.

### Testing
- **D-20:** Unit tests should cover valid multi-provider config, duplicate IDs, unsupported provider type, invalid URL, empty models, default model mismatch, invalid durations, invalid thresholds, invalid max concurrency, fallback attempts vs provider count, secret-safe error text, and env fallback compatibility.
- **D-21:** Tests should prove raw API key values do not appear in validation errors.
- **D-22:** Full Go verification should run after implementation: `gofmt -l .`, `go vet ./...`, and `go test ./...`.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and Phase Context
- `.planning/PROJECT.md` - Project-level purpose and constraints.
- `.planning/REQUIREMENTS.md` - Requirements registry.
- `.planning/ROADMAP.md` - Phase 2.8 goal and success criteria.
- `.planning/STATE.md` - Current milestone state.
- `.planning/phases/02-health-aware-routing/02-FOLLOWUP-ROADMAP.md` - Follow-up roadmap naming Phase 2.8 and its scope.
- `.planning/phases/02-health-aware-routing/02-06-CONTEXT.md` - Health-check config decisions.
- `.planning/phases/02-health-aware-routing/02-06-PLAN.md` - Active health probing plan and config field expectations.
- `.planning/phases/02-health-aware-routing/02-06-UAT.md` - Phase 2.6 verification record.
- `.planning/phases/02-health-aware-routing/02-07-CONTEXT.md` - Capability metadata decisions relevant to optional overrides.
- `.planning/phases/02-health-aware-routing/02-07-01-PLAN.md` - Adapter capability contract plan.
- `.planning/phases/02-health-aware-routing/02-07-02-PLAN.md` - Capability exposure plan, if present.

### Current Code Integration Points
- `internal/config/config.go` - Current config structs, JSON file load path, env fallback, defaults, and validation.
- `internal/config/config_test.go` - Existing config validation coverage.
- `internal/app/app.go` - Provider-specific adapter wiring from static config.
- `internal/providers/capabilities.go` - Phase 2.7 capability types.
- `internal/providers/adapter.go` - Adapter contract.
- `internal/providers/registry.go` - Provider registry boundary.
- `internal/health/prober.go` - Health-check config consumption.
- `README.md` - Current setup and multi-provider config docs.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/config.Config` already owns gateway addresses, routing strategy, fallback settings, global health-check config, and providers.
- `internal/config.ProviderConfig` already has provider ID, type, base URL, raw API key, models, default model, timeout, weight, and provider health-check override fields.
- `LoadConfig` already supports `CONFIG_FILE` JSON and a backward-compatible environment fallback for a single OpenAI-compatible provider.
- `Validate` already checks routing strategy, provider presence, duplicate IDs, supported provider types, missing base URL, missing models, default provider existence, provider timeout parsing, and health-check duration/threshold constraints.
- `app.New` is the current provider wiring boundary and should remain the place where provider type selects the adapter package.

### Gaps
- `base_url` validation only checks non-empty strings; malformed URLs and non-HTTP schemes can pass.
- `default_model` is not validated against the provider model list.
- Current docs show `"api_key": "YOUR_KEY"` directly and do not demonstrate Anthropic/Gemini configuration.
- `MaxAttempts` is silently clamped between 1 and 5 instead of validating fallback attempts against configured provider count.
- Raw `api_key` remains the only schema-level auth shape; no explicit auth reference exists for future provider-management contracts.
- Validation errors are not systematically tested for secret-safety.
- Config structs are still broad, organic structs rather than clearly grouped schema concepts for identity, endpoint/auth, models, health overrides, fallback/retry, and optional capability overrides.

### Integration Points
- Refactor config structs in `internal/config` while preserving JSON compatibility for existing config files.
- Keep provider type constants aligned with `internal/providers` where practical without creating import cycles.
- Update `app.New` only as needed to consume renamed or nested schema fields.
- Add focused config unit tests before broad app/routing changes.
- Update README examples after the schema is settled.
</code_context>

<must_build>
## Must Build In Phase 2.8

- Explicit provider config schema covering identity, type, base URL, auth reference or compatible secret field, models, default model, timeout, health-check overrides, retry/fallback settings, and capability overrides if needed.
- Secret-safe validation for duplicate IDs, unknown provider types, invalid base URLs, empty models, default model mismatches, invalid durations, invalid health thresholds, invalid concurrency, and fallback attempts that do not fit provider count.
- Backward-compatible env config loading for the existing single OpenAI-compatible provider path.
- README configuration docs with a secret-safe multi-provider example covering OpenAI-compatible, Anthropic, and Gemini.
- Unit tests for validation behavior, docs-facing compatibility assumptions where feasible, and secret-safe error text.
</must_build>

<deferred>
## Deferred Ideas

- Database-backed provider configuration.
- Secret manager integration.
- Runtime config hot reload.
- Admin API CRUD for providers.
- Admin Console provider-management UI.
- Redis or PostgreSQL-backed health/config state.
- Live model discovery from provider APIs.
- Provider model catalog and routing eligibility; planned for Phase 2.9.
- Adapter conformance harness; planned for Phase 2.10.
- SSE streaming proxy.
- Tool/function calling normalization.
- Multimodal APIs.
- Rate limiting, semantic cache, cost governance, and observability exporters.
</deferred>

<success_criteria>
## Success Criteria

- Provider configuration is represented by stable, explicit structs suitable for later provider-management contracts.
- Validation rejects duplicate provider IDs, unknown provider types, invalid URLs, invalid durations, invalid thresholds, model/default mismatches, and invalid fallback attempts.
- Validation errors, logs, docs, and tests do not expose API keys, authorization headers, raw prompts, raw upstream bodies, or sensitive provider payloads.
- Existing backward-compatible environment config still loads.
- README includes a secret-safe multi-provider config example for OpenAI-compatible, Anthropic, and Gemini.
- `gofmt -l .`, `go vet ./...`, and `go test ./...` pass after implementation.
</success_criteria>

---

*Phase: 2.8-Provider Configuration Schema and Secret-Safe Validation*
*Context gathered: 2026-06-16*
