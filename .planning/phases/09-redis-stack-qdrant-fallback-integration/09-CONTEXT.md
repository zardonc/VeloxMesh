# Phase 9: Redis Stack + Qdrant Fallback Integration - Context

**Gathered:** 2026-06-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Harden Plan 1 Redis Stack integration around hot cache, atomic rate limiting, config Pub/Sub, token-cost aggregation, session/API-key hot state, and Redis VSS as an optional fallback only when Qdrant is degraded or slow.

</domain>

<decisions>
## Implementation Decisions

### Qdrant Fallback Policy
- **D-01:** Qdrant remains the primary vector and semantic-cache path for Plan 1/2.
- **D-02:** Redis VSS is default off and may activate only when Qdrant has a hard failure or crosses a timeout/slow-threshold degradation policy.
- **D-03:** Vector degradation must not block core LLM proxying, auth, routing, provider fallback, or non-vector cache behavior.

### Redis Durability Boundary
- **D-04:** Redis is hot-path acceleration, not the source of truth for user/account/security/billing state.
- **D-05:** Token cost aggregation, rate/budget counters that affect billing, session blacklist state, and API-key hot-cache state must all reconcile or flush back to SQLite.
- **D-06:** SQLite remains authoritative for rule definitions, long-lived totals, audit trails, and recoverable state.

### Rate Limit Design
- **D-07:** Rate limiting and budget/quota are separate concepts. Do not collapse request/token rate caps and lifetime budget into one ambiguous "quota" path.
- **D-08:** Phase 9 should introduce a unified `LimitRule` domain/interface direction, but persist only the smallest necessary subset: API-key quota/RPM and upstream RPM/periodic absolute usage limits.
- **D-09:** Full `LimitRule` database unification across all scopes is deferred and must be visible in the global roadmap.
- **D-10:** API-layer limits and upstream-account limits are independent gates. A request must pass both; do not implement override/arbitration rules between them.
- **D-11:** Lifetime budget/total quota is SQLite-authoritative. Redis can support hot reads or pre-deduct checks but cannot be the only durable record.
- **D-12:** Periodic windows, RPM, and short-lived counters should use Redis atomic execution, preferably Lua for check-and-increment / pre-deduct behavior.
- **D-13:** Upstream provider "remaining balance ratio" limits are out of scope unless a provider exposes reliable balance data. Prefer absolute periodic usage or request limits.

### Config Hot Reload
- **D-14:** Pub/Sub config reload should route by event type, not blanket-reload every runtime surface for every change.
- **D-15:** Existing semantic-rule/provider config-change behavior should be reused and refined instead of replaced with a parallel event system.

### the agent's Discretion
- Keep Phase 9 narrow: implement the Redis atomic execution layer and minimal rule model needed by current API-key/upstream-account limits.
- Reuse existing `internal/hotstate`, `internal/health`, `internal/cache`, `internal/storage`, and SQLite repository patterns.
- Avoid a large schema rewrite unless planning finds the existing schema cannot express the minimal Phase 9 limits.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` — Phase 9 scope, dependency chain, and deferred full `LimitRule` roadmap note.
- `.planning/PROJECT.md` — architecture v2.1 and SQLite + Redis Stack + Qdrant source of truth.
- `.planning/phases/07-adapter-interfaces-sqlite-foundation/07-CONTEXT.md` — Qdrant primary path, Redis VSS fallback-only decision, degraded vector behavior.
- `.planning/phases/08-semantic-pipeline/08-CONTEXT.md` — hot-reload and semantic-rule runtime context.

### External Design
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-ratelimiting.md` — rate limit/quota planning philosophy. Treat examples as reference, not mandatory code.

### Existing Code
- `internal/hotstate/redis.go` — existing Redis client, config Pub/Sub, auth cache, health/probe snapshots.
- `internal/health/redis_store.go` — current Redis health-store pattern and local fallback behavior.
- `internal/storage/qdrant.go` — Qdrant vector adapter and health/degraded behavior.
- `internal/storage/adapters.go` — memory cache, no-op coordination, no-op/degraded vector adapters.
- `internal/cache/semantic.go` — semantic-cache lookup/store path and vector adapter integration.
- `internal/config/config.go` — Redis/Qdrant/semantic-cache config shape and validation.
- `internal/controlstate/sqlite/repository.go` — SQLite rate, usage, semantic cache, and settlement persistence.
- `internal/app/app.go` — startup wiring for Redis, Qdrant, semantic cache, and config reload.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `hotstate.RedisClient` already supports Ping, auth cache, health/probe snapshots, and config-change Pub/Sub.
- `health.RedisStore` already demonstrates Redis-backed hot state with local fallback.
- `storage.QdrantVectorAdapter`, `NoopVectorAdapter`, and `DegradedVectorAdapter` provide the vector availability seam.
- SQLite repositories already persist usage records, provider model rates, semantic cache entries, combos, routing config, and semantic rules.
- `StartConfigChangeSubscriber` already reloads runtime state on hot-state notifications.

### Established Patterns
- SQLite is the durable Plan 1 relational source of truth.
- Redis may degrade to local behavior for non-authoritative hot state, but correctness/security/billing state must reconcile to SQLite.
- Qdrant failures degrade vector/semantic-cache capability; they do not take down the gateway.
- Provider-specific details stay behind adapters.

### Integration Points
- Rate/budget checks should sit in the admission/gateway path before upstream provider calls.
- Actual usage/cost reconciliation belongs after provider responses, using existing usage/settlement patterns.
- Config-change event routing should extend existing hotstate Pub/Sub message handling.
- Redis VSS fallback should connect through the vector adapter seam instead of leaking into gateway handlers.

</code_context>

<specifics>
## Specific Ideas

- Use Redis Lua for atomic check-and-increment or pre-deduct flows where concurrency matters.
- Use pre-deduct plus correction/rollback for request paths where actual token cost is only known after provider response.
- Periodic counters can tolerate Redis reset better than lifetime budget; lifetime budget must be recoverable from SQLite records.
- Keep "billing multiplier" as pricing configuration, not a rate-limit rule.

</specifics>

<deferred>
## Deferred Ideas

- Full database-level unification of all rate/budget/window limits into one `LimitRule` table/model across API, upstream account, team, model, or future scopes.
- Fair-share allocation or priority reservations when multiple users compete for the same upstream account limit.
- Provider-balance ratio limits based on real upstream remaining balance, unless reliable provider balance APIs or trusted operator-entered balances exist.

</deferred>

---

*Phase: 9-Redis Stack + Qdrant Fallback Integration*
*Context gathered: 2026-06-30*
