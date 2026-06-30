# Phase 9: Redis Stack + Qdrant Fallback Integration - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-30
**Phase:** 9-Redis Stack + Qdrant Fallback Integration
**Areas discussed:** Qdrant fallback, Redis durability boundary, atomic rate limiting, config hot reload, LimitRule scope

---

## Qdrant Fallback

| Option | Description | Selected |
|--------|-------------|----------|
| Hard failure only | Redis VSS activates only when Qdrant cannot be reached. | |
| Hard failure and timeout/slow threshold | Redis VSS activates on hard failure or degradation policy. | yes |
| Manual flag only | Operator explicitly switches to Redis VSS. | |

**User's choice:** Hard failure and timeout/slow threshold.
**Notes:** Redis VSS remains fallback-only, not the default vector path.

---

## Redis Durability Boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Redis-only for hot state | Leave listed hot-path state in Redis only. | |
| Flush all listed items to SQLite | Cost counters, rate/budget state, session blacklist, and API-key cache reconcile to SQLite. | yes |
| Split by category | Persist only billing/security state, leave cache-only state in Redis. | |

**User's choice:** All listed items should flush to SQLite.
**Notes:** The agent agreed: Redis should be fast working memory, while SQLite remains authoritative for user/account/security/billing state.

---

## Atomic Rate Limiting

| Option | Description | Selected |
|--------|-------------|----------|
| API-key limits only | Replace user-facing request/token quota first. | |
| All known rate counters | Move API-key, provider, model, and admin/control counters to Redis now. | |
| Hybrid with LimitRule direction | Define unified domain/interface direction, persist the smallest necessary subset. | yes |

**User's choice:** Hybrid with LimitRule direction.
**Notes:** Phase 9 should define the `LimitRule` direction but implement only API-key quota/RPM and upstream RPM/periodic absolute usage limits. Missing full unification must be marked in the global roadmap.

---

## Config Hot Reload

| Option | Description | Selected |
|--------|-------------|----------|
| Blanket reload | Reload all runtime state on every config event. | |
| By event type | Route reloads by provider, semantic rules, API keys, etc. | yes |

**User's choice:** By event type.
**Notes:** Reuse existing Pub/Sub behavior, refine event routing.

---

## the agent's Discretion

- Keep the implementation small and reuse existing Redis/hotstate and SQLite repository patterns.
- Treat `gateway-ratelimiting.md` as design guidance, not required example code.

## Deferred Ideas

- Full `LimitRule` table/model unification across all scopes.
- Fair-share resource allocation for users sharing upstream account limits.
- Upstream provider balance-ratio limits.
