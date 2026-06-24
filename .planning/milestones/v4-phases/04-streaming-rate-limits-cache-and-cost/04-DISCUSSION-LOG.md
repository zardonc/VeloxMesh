# Phase 04: Streaming, Rate Limits, Cache, and Cost - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-21
**Phase:** 04-Streaming, Rate Limits, Cache, and Cost
**Areas discussed:** Phase split, Durable provider configuration, Streaming contract, Credit quotas/admission, Cache/cost boundary

---

## Phase Split

| Option | Description | Selected |
|--------|-------------|----------|
| Smaller slices | Split Phase 04 into independently planned and testable slices. | ✓ |
| One large plan | Plan streaming, rate limits, cache, cost, and circuit-breaker behavior together. | |

**User's choice:** split into smaller slices.
**Notes:** Phase 04 should start with durable provider configuration source-of-truth hardening before streaming/rate/quota work.

---

## Durable Provider Configuration

| Option | Description | Selected |
|--------|-------------|----------|
| Database source of truth | When durable control state is enabled, load providers/routing/fallback from database and stop relying on `config.go` hard-coded provider defaults. | ✓ |
| Keep static config as runtime fallback | Allow `config.go` provider defaults to backfill missing durable provider state. | |
| Remove static config entirely | Delete static/env provider support everywhere. | |

**User's choice:** Database source of truth.
**Notes:** `config.go` currently has hard-coded provider/default model paths. Phase 04 must complete the durable provider config transition so provider records, routing settings, and fallback behavior come from the database. Static/env config remains only for disabled-control-state/local seed compatibility.

### Hardcoded/Transitional Debt Scan

| Finding | Impact | Captured |
|---------|--------|----------|
| Durable repository stubs | SQLite/PostgreSQL routing, API-key, and usage repos currently return nil/no-op values, which would break durable fallback/quota/usage slices. | ✓ |
| Static runtime dependencies | `Readyz`, `health.Prober`, and gateway fallback settings still read static config or constructor-time values. | ✓ |
| Durable startup wiring | `app.New` does not fully wire durable repository startup, Admin routes, migrations, local seed, initial reload, or subscriber flow as the normal app path. | ✓ |
| Native health placeholders | Anthropic/Gemini health checks report available without a real upstream check or explicit unsupported semantics. | ✓ |

**User's choice:** Detect and handle similar hard-coded technical debt from earlier phases.
**Notes:** Captured only Phase 04-relevant debt. README examples and test fixtures were ignored unless they exposed runtime behavior.

## Gateway Runtime Modes

| Option | Description | Selected |
|--------|-------------|----------|
| Lite + full modes | Lite uses SQLite only with basic/limited features; full uses PostgreSQL + Redis for complete functionality. | ✓ |
| Single full-stack mode | Require PostgreSQL + Redis for all durable/runtime behavior. | |
| SQLite-only first | Keep all features SQLite/local until distributed middleware is introduced later. | |

**User's choice:** Add a global roadmap requirement for two gateway startup modes.
**Notes:** Lite mode should start without middleware and provide basic gateway capability. Full mode should require PostgreSQL and Redis to be predeployed through Docker Compose or equivalent before gateway startup.

---

## Streaming Contract

| Option | Description | Selected |
|--------|-------------|----------|
| SSE first, no fallback after bytes sent | Implement `stream: true`, cancel on disconnect, and only allow fallback before response streaming starts. | |
| Streaming with provider fallback attempts before first token | Retry/fallback if the first provider fails before sending any SSE event, then pin once output begins. | ✓ |
| Full streaming resilience | Add reconnect semantics, partial failure metadata, and richer stream observability now. | |

**User's choice:** Streaming with provider fallback attempts before first token.
**Notes:** No provider switching after partial output starts.

---

## Credit Quotas/Admission

| Option | Description | Selected |
|--------|-------------|----------|
| Per API key fixed window, Redis when enabled, local fallback | Enforce request-count limits per data-plane API key; Redis gives multi-instance behavior, no Redis is process-local. | superseded |
| Provider + API key limits | Limit both callers and upstream providers. | |
| Queued admission with priority classes | Use admission priority classes to queue instead of reject. | |

**Earlier coarse choice:** Per API key fixed window, Redis when enabled, local fallback.
**Notes:** Superseded by the later quota/credits decision below.

### Quota Model Revision

| Option | Description | Selected |
|--------|-------------|----------|
| Credit quota abstraction | Each API key owns credits; provider/model rates consume credits by token usage. | ✓ |
| Fixed-window request count | Limit number of requests per key/window. | |
| Provider-bound API key quotas | Bind quota to provider or provider+model. | |

**User's choice:** Add a quota abstraction. Each API key has user-allocated credits. Different providers and models consume credits according to their own token-based standards. API keys and providers are fully decoupled.
**Notes:** This replaces fixed-window request-count limiting as the primary admission model.

### Quota Exhaustion

| Option | Description | Selected |
|--------|-------------|----------|
| Hard reject | If an API key has no usable credits, return `429` before calling a provider. | ✓ |
| Allow overdraft | Permit a small configurable overdraft. | |
| Meter only | Record usage without enforcing quota. | |

**User's choice:** Hard reject.
**Notes:** Requests rejected by quota must not create upstream provider cost.

### Deduction Timing

| Option | Description | Selected |
|--------|-------------|----------|
| Deduct after success by actual usage | Use provider usage/token fields after a successful response. | ✓ |
| Pre-deduct and reconcile | Reserve credits before provider call, then refund or charge the delta. | |
| Check before, deduct after | Simpler check without full affordability reservation. | |

**User's choice:** Deduct after success by actual usage.
**Notes:** Strict reservation is deferred. A request can start with positive credits and settle into deficit if actual usage exceeds balance.

### Credit Rates

| Option | Description | Selected |
|--------|-------------|----------|
| Admin-managed model pricing table | Durable provider+model rates define input/output token credit consumption. | ✓ |
| Static config first | Store rates in config/env before Admin support. | |
| Code defaults | Hardcode rates in source. | |

**User's choice:** Admin-managed model pricing table.
**Notes:** Rates are decoupled from API keys and aligned with Phase 3 durable control state.

### Credit Unit

| Option | Description | Selected |
|--------|-------------|----------|
| Integer credits | API keys hold integer credits; rates are credits per 1K input/output tokens. | ✓ |
| USD-like decimal balance | Store money-like decimal balances. | |
| Token units | Treat quota directly as token units. | |

**User's choice:** Integer credits.
**Notes:** Avoids floating-point/currency accounting in this phase.

### Streaming Settlement

| Option | Description | Selected |
|--------|-------------|----------|
| Deduct after stream completion when usage exists | Use provider stream usage; if missing, do not guess and record missing usage. | ✓ |
| Estimate missing usage | Estimate tokens when provider usage is missing. | |
| Do not charge streaming | Let streaming bypass credits initially. | |

**User's choice:** Deduct after stream completion when usage exists.
**Notes:** Missing usage is recorded as unsettled/missing; no estimated charge.

---

## Cache/Cost Boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Usage/cost hooks first, semantic cache later | Record usage/cost-ready data where available; keep cache disabled. | ✓ |
| Exact request cache only | Add opt-in exact-match cache for non-streaming chat. | |
| Semantic cache now | Add similarity cache behavior, keys, TTLs, and privacy rules now. | |

**User's choice:** Usage/cost hooks first, semantic cache later.
**Notes:** Existing cache headers remain explicit misses until cache behavior is scoped.

## the agent's Discretion

- Choose exact slice filenames and config names during planning.
- Keep each slice minimal and independently testable.

## Deferred Ideas

- Semantic cache.
- Exact request cache.
- Provider-level quotas.
- Queued priority admission.
- Fixed-window request-count limits as the primary model.
- Strict credit reservation before provider calls.
- Full streaming reconnect/resume behavior.
- Cost-aware routing and budget policy.
