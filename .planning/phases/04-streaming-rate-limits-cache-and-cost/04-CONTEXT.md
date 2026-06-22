# Phase 04: Streaming, Rate Limits, Cache, and Cost - Context

**Gathered:** 2026-06-21
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 04 should first finish the durable provider configuration transition, then add advanced data-plane behavior in small, independently planned slices after Phase 3 durable control state: SSE streaming, API-key credit quotas/admission, provider/model credit rates, and usage/cost-ready recording. It should preserve the OpenAI-compatible client API, Go/Chi gateway shape, provider adapter isolation, secret-safe behavior, and Phase 3 durable/Redis boundaries.

This phase should not be planned as one large implementation. Split it into smaller plans that can be tested and shipped separately.

</domain>

<decisions>
## Implementation Decisions

### Phase Split
- **D-01:** Split Phase 04 into smaller slices instead of one large plan.
- **D-02:** Recommended slice order is `04-01` durable provider configuration source-of-truth hardening, `04-02` streaming, `04-03` credit quota/admission primitives, and `04-04` usage settlement plus provider/model credit-rate management.
- **D-03:** Semantic cache should remain deferred until usage/cost recording exists and cache privacy rules are intentionally scoped.

### Durable Provider Configuration
- **D-04:** Phase 04 must remove production dependence on hard-coded provider defaults in `internal/config/config.go`.
- **D-05:** Durable database-backed provider configuration is the source of truth whenever `ControlStateBackend` is `postgres` or `sqlite`.
- **D-06:** Static/env provider config may remain only for `ControlStateBackend=disabled` and explicit local seed flows.
- **D-07:** Startup with durable control state enabled should load active providers, encrypted secrets, routing strategy, default provider, fallback settings, and health/probe config from the database before serving data-plane traffic.
- **D-08:** If durable provider configuration is missing or invalid, the gateway should return actionable no-active-provider/missing-config errors instead of silently falling back to `openai-primary`, `gpt-4o-mini`, or other hard-coded provider defaults.
- **D-09:** `config.go` should keep process/environment settings such as bind addresses, log level, backend DSN, Redis settings, and compatibility seed inputs; provider runtime state should move behind durable repository/runtime manager boundaries.
- **D-10:** Fallback must use the active durable provider set and durable routing config. `max_attempts` must be validated against currently active eligible providers, not stale static config.
- **D-11:** Runtime reload after Admin provider changes must atomically rebuild registry, router, model catalog, prober, fallback settings, and health thresholds from durable records.
- **D-12:** `/readyz`, `/v1/models`, and chat routing must reflect the runtime snapshot, not `cfg.Providers`, when durable control state is enabled.

### Hardcoded/Transitional Debt To Clear
- **D-13:** PostgreSQL and SQLite `RoutingRepository`, `APIKeyRepository`, and `UsageRepository` methods are currently no-op/stub implementations. Phase 04 must implement or fail closed for the portions it depends on before quota, fallback, or usage settlement work.
- **D-14:** `app.New` must stop requiring tests/manual setup to wire durable repositories, Admin provider routes, migrations, local seed, initial `ReloadProviders`, and config-change subscriber behavior. Durable startup should be a normal app path when `ControlStateBackend` is `postgres` or `sqlite`.
- **D-15:** `gateway.Service` currently captures `fallbackEnabled` and `maxAttempts` at construction time. Fallback settings should come from the active runtime/durable routing snapshot so Admin routing changes and durable reloads affect request execution without process restart.
- **D-16:** `health.Prober` currently looks up per-provider probe config through `cfg.Providers`; under durable control state it must use provider health config from the active runtime snapshot.
- **D-17:** `Readyz` currently reports `configured_providers`, `routing_strategy`, and probe settings from static `cfg`. Under durable control state those fields must come from runtime/durable state, not stale config.
- **D-18:** Native Anthropic/Gemini adapter `HealthCheck` implementations currently return available without a real upstream check. Phase 04 should either implement the cheapest safe health behavior or clearly mark unsupported/native probe semantics so fallback/readiness do not imply stronger health guarantees than exist.

### Streaming Contract
- **D-19:** Implement OpenAI-compatible SSE streaming for `stream: true`.
- **D-20:** Streaming may retry or fallback to another eligible provider only before the first SSE event/body bytes are sent.
- **D-21:** Once streaming output has begun, the gateway must not switch providers or replay partial content.
- **D-22:** Streaming should honor client disconnect/cancellation and stop upstream work promptly.
- **D-23:** Provider-specific stream mapping belongs inside provider adapter packages; HTTP handlers and gateway orchestration should consume provider-neutral streaming contracts.

### Rate Limits and Admission
- **D-24:** Replace simple fixed-window request limits with a provider-decoupled credit quota model.
- **D-25:** Each data-plane API key owns an integer credit balance that users/Admin can allocate or top up.
- **D-26:** API keys must not be bound to providers. Provider/model choice only affects how many credits a successful request consumes.
- **D-27:** Provider/model credit rates should be Admin-managed durable configuration, keyed by provider and model.
- **D-28:** Credit rates should use integer credits per 1K input tokens and per 1K output tokens; avoid floating-point or currency-like balances in this phase.
- **D-29:** If an API key has no usable credits, admission must hard-reject before calling any provider.
- **D-30:** Quota failures should return `429` with safe quota/rate-limit-style headers and structured OpenAI-compatible error JSON.
- **D-31:** Do not implement provider-level quotas, weighted quota policies, fixed-window request counts, or queued priority admission in the first quota slice.
- **D-32:** The existing admission boundary is the right integration point, but keep it minimal.

### Usage and Cost Boundary
- **D-33:** Deduct credits only after a successful provider response, using actual provider usage/token data.
- **D-34:** Failed provider requests should not consume credits in the first quota slice.
- **D-35:** For non-streaming responses, record provider, model, prompt tokens, response tokens, total tokens, duration, timestamp, credit rate, and credits consumed when usage is available.
- **D-36:** For streaming responses, deduct credits after the stream completes when provider usage is available.
- **D-37:** If a provider response or completed stream lacks usage fields, do not guess token counts. Record a safe `missing_usage`/unsettled outcome for follow-up.
- **D-38:** The first version does not need strict pre-request credit reservation. If a request starts with positive credits but final actual usage exceeds the balance, record the deficit; add reservation later if this matters operationally.
- **D-39:** Do not implement exact request cache or semantic cache in the first usage/cost slice.
- **D-40:** Preserve existing cache headers as explicit misses until cache behavior is intentionally scoped.

### Test Environment
- **D-41:** Planning should account for the roadmap's verified local resources: PostgreSQL, Redis, and local provider configurations for `sanf` and `sans`.
- **D-42:** Tests and docs may reference that these resources exist in `.env` and `.env.local`, but planning artifacts must not include credentials, provider secrets, raw prompts, or sensitive payloads.
- **D-43:** Automated tests should keep using fakes by default. Live/local environment checks may be opt-in and secret-safe.

### Gateway Runtime Modes
- **D-44:** Phase 04 planning must preserve two startup modes: lite mode and full mode.
- **D-45:** Lite mode uses SQLite only, requires no PostgreSQL or Redis, and should provide the basic OpenAI-compatible gateway path plus durable local provider configuration.
- **D-46:** Lite mode should explicitly disable, degrade, or fail closed for features that require distributed PostgreSQL/Redis semantics.
- **D-47:** Full mode uses PostgreSQL + Redis and is the complete/production-capable path for distributed durable state, hot-state coordination, config-change propagation, quota/cost behavior, and full Phase 04 functionality.
- **D-48:** Full mode requires PostgreSQL and Redis to be deployed before gateway startup, with Docker Compose or equivalent local middleware deployment documented.
- **D-49:** Each Phase 04 plan must list lite/full behavior and include focused tests or capability checks for mode gating.

### the agent's Discretion
- Planner may choose exact config names, package boundaries, quota header names, and storage wiring as long as the decisions above are preserved.
- Prefer the smallest runnable checks per slice: focused unit tests plus `go test ./...`.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project and Phase Context
- `.planning/PROJECT.md` - Project purpose, constraints, and Go-first gateway decision.
- `.planning/REQUIREMENTS.md` - Phase 04 requirements: STRM-01, RATE-01, CACHE-01, COST-01, CB-01.
- `.planning/ROADMAP.md` - Phase 04 goal, success criteria, and local development resources.
- `.planning/STATE.md` - Current project state and next milestone slice.
- `.planning/phases/03-durable-control-state/03-CONTEXT.md` - Durable control-state, Redis, and secret-safety decisions.
- `.planning/phases/03-durable-control-state/03-03-PLAN.md` - Runtime provider loading, actionable missing-config errors, disabled-provider filtering, and reload without restart.
- `.planning/phases/03-durable-control-state/03-07-PLAN.md` - Redis config-change notification and multi-instance runtime reload behavior.
- `.planning/phases/02-health-aware-routing/02-09-CONTEXT.md` - Model catalog and routing eligibility decisions.
- `.planning/phases/02-health-aware-routing/02-10-CONTEXT.md` - Adapter conformance and no-live-call test decisions.

### Current Code Integration Points
- `internal/http/handlers/chat.go` - Current chat handler rejects `stream: true`, sets cache/fallback headers, and encodes OpenAI-compatible responses.
- `internal/gateway/service.go` - Gateway routing, fallback, health updates, metrics, and provider execution loop.
- `internal/admission/controller.go` - Existing admission boundary and priority validation.
- `internal/llm/types.go` - OpenAI-compatible chat request/response and internal LLM request/response types.
- `internal/providers/adapter.go` - Provider adapter boundary that streaming should extend.
- `internal/providers/openai/adapter.go` - OpenAI-compatible adapter implementation and likely first streaming target.
- `internal/providers/anthropic/adapter.go` - Native adapter mapping boundary.
- `internal/providers/gemini/adapter.go` - Native adapter mapping boundary.
- `internal/routing/router.go` - Health-aware provider selection and strict override behavior.
- `internal/hotstate/hotstate.go` - Redis hot-state interface patterns and namespace rules.
- `internal/controlstate/types.go` - Existing `UsageRecord` shape and durable record patterns.
- `internal/config/config.go` - Existing config loading, Redis config, fallback settings, and validation style.
- `internal/app/app.go` - Current app wiring, static adapter construction, runtime manager, hot state, and gateway service setup.
- `internal/controlstate/runtime.go` - Runtime provider manager, durable record adapter construction, routing snapshot activation, and fallback/router integration.
- `internal/controlstate/repository.go` - Durable provider, routing, API key, usage, audit, and idempotency repository contracts.
- `internal/controlstate/sqlite/repository.go` - SQLite durable repository implementation; routing/API-key/usage methods are currently simplified stubs.
- `internal/controlstate/postgres/repository.go` - PostgreSQL durable repository implementation; routing/API-key/usage methods are currently simplified stubs.
- `internal/controlstate/seed.go` - Static-to-durable local seed path that must remain seed-only.
- `internal/http/handlers/health.go` - Readiness behavior that currently mixes config and runtime state.
- `internal/health/prober.go` - Active provider probing currently reads provider health overrides from static config.
- `internal/providers/anthropic/adapter.go` - Native adapter health behavior currently returns available without live probing.
- `internal/providers/gemini/adapter.go` - Native adapter health behavior currently returns available without live probing.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `gateway.Service.HandleChatCompletion` already has retry/fallback before provider execution and can enforce the "fallback only before first stream event" rule at the orchestration boundary.
- `config.LoadConfig` still has hard-coded compatibility defaults such as `openai-primary`, `gpt-4o-mini`, and OpenAI env var names; these should not drive production durable provider runtime.
- `app.New` currently constructs adapters from `cfg.Providers` before only activating them when control state is disabled; durable startup needs an explicit database load/reload path before serving.
- `controlstate.RuntimeProviderManager` can atomically swap registry/router/prober snapshots and is the right place to centralize durable provider activation.
- `controlstate.RoutingRepository` already exists but runtime fallback/default-provider behavior still needs to be wired to durable routing config instead of static `cfg` values.
- SQLite/PostgreSQL repository implementations persist providers, audit, and idempotency, but routing, API key, and usage repositories are no-op stubs today.
- `gateway.Service` owns fallback config as immutable constructor fields, so reloads cannot change fallback behavior yet.
- `Readyz` and `health.Prober` still depend on `cfg.Providers`/`cfg.HealthCheck` for runtime reporting and probe settings.
- Anthropic/Gemini native health checks currently report available without a real check, which can make readiness/fallback trust weaker data than planned.
- `admission.Controller` already exists and currently passes through; it is the narrowest place to add API-key credit checks before provider calls.
- `hotstate.Client` already wraps Redis/local hot state with namespaced keys and TTL semantics.
- `controlstate.UsageRecord` already names the durable usage fields that can be made real in the usage/cost slice.
- `ChatHandler` already emits `X-Cache-Hit: false` and `X-Cache-Level: none`, so cache can stay visibly disabled.

### Established Patterns
- Static provider config is compatibility/local seed only, not the production source of truth.
- Hard-coded provider/model defaults are acceptable only in disabled-control-state local mode.
- OpenAI-compatible public responses stay stable even when provider-native behavior changes behind adapters.
- Provider-native request/response mapping stays inside adapter packages.
- Redis is optional; when absent, local-only behavior must be explicit.
- Tests should use deterministic fake upstreams and avoid real credentials unless an opt-in local check is explicitly requested.

### Integration Points
- First harden durable provider startup and reload so `config.go` no longer controls provider runtime under database-backed control state.
- Replace or fail closed on durable repository stubs before relying on routing/API-key/usage persistence in quota and cost slices.
- Load durable routing config with providers so fallback/default-provider behavior is rebuilt atomically with the active provider set.
- Make readiness/model listing/chat routing use the runtime snapshot under durable control state.
- Then add provider-neutral streaming support between `internal/llm`, `internal/providers`, `internal/gateway`, and `internal/http/handlers`.
- Add credit quota admission through `internal/admission` using API-key identity from auth middleware or a safe derived token hash.
- Add durable API-key credit balance storage and Admin-managed provider/model credit-rate storage.
- Use Redis only where it helps coordinate hot quota/admission state; PostgreSQL/SQLite durable records remain the source of truth.
- Wire usage settlement after provider completion for non-streaming and after stream completion for streaming where usage data is available.

</code_context>

<specifics>
## Specific Ideas

- Before streaming/quota work, Phase 04 should fix provider configuration hard-coding by loading active provider and routing records from the database when durable control state is enabled.
- The first cleanup slice should include a hardcoded/transitional-debt audit with targeted tests so these items do not regress silently.
- Fallback behavior must be proven against database-backed provider records, including disabled providers, missing secrets, default provider changes, and max-attempt validation.
- Durable repository stubs for routing/API key/usage must either be implemented in the relevant slice or return explicit unsupported errors instead of pretending success.
- First streaming slice should support retry/fallback before first SSE event, then pin to the selected provider once bytes start.
- First quota slice should introduce API-key credits and hard-reject keys with no usable credits.
- Provider/model credit rates should be Admin-managed durable config and expressed as integer credits per 1K input/output tokens.
- First usage/cost slice should settle credits from actual provider usage, not estimate missing token counts.
- Roadmap's local test resources should inform verification, but credentials must stay in local env files only.

</specifics>

<deferred>
## Deferred Ideas

- Semantic cache.
- Exact request cache.
- Provider-level quotas.
- Fixed-window request-count limits as the primary model.
- Queued priority admission.
- Strict pre-request credit reservation and refund/reconcile flows.
- Reconnection/resume semantics for interrupted streams.
- Switching providers after partial streaming output.
- Cost-aware routing, model degradation, budget enforcement, or pricing policy.
- Broad analytics dashboard or Admin Console UI.
- Deleting the static/env provider path entirely; keep it for `ControlStateBackend=disabled` and explicit local seed compatibility.

</deferred>

---

*Phase: 04-Streaming, Rate Limits, Cache, and Cost*
*Context gathered: 2026-06-21*
