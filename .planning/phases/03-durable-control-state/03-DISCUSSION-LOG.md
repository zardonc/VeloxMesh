# Phase 3: Durable Control State - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-18
**Phase:** 3-Durable Control State
**Areas discussed:** Source-of-truth transition, Control-state data model, Admin API behavior, Redis boundary

---

## Source-of-Truth Transition

| Option | Description | Selected |
|--------|-------------|----------|
| Seed then DB | Static config seeds initial records or local dev defaults; PostgreSQL becomes runtime source of truth after startup. | Partial |
| Dual mode | Gateway can run either static-only or DB-backed depending on configuration. | |
| DB required | Phase 3 requires PostgreSQL for provider/config state. | |
| Schema-only init with dev seed | Production initialization creates schema only; provider config is entered at runtime; static/env seed remains local-dev convenience. | Yes |

**User's choice:** Started with seed-then-DB, then refined to schema-only project initialization with local-dev static/env seed allowed.
**Notes:** User explicitly required that provider names, endpoints, API keys, credentials, and options are entered at runtime. Provider-dependent functions before configuration must return clear actionable missing-config errors. User also requested SQLite as a replacement backend with advanced features such as semantic cache disabled when SQLite is selected.

---

## Control-State Data Model

| Option | Description | Selected |
|--------|-------------|----------|
| Provider config first | Persist provider definitions, provider auth/secrets, optional model config, routing defaults, and validation status first. | Yes |
| Provider plus API keys | Persist provider config and gateway client API keys together. | |
| Full control state slice | Persist providers, gateway API keys, routing settings, health snapshots, and basic usage records in first plan. | |

**User's choice:** Provider config first.
**Notes:** User chose encrypted secret values in DB, always-valid provider saves, minimal provider connection fields as mandatory, and optional model config now. "Always valid" was clarified to mean required minimal connection fields must be valid; model/default config can be optional until an operation requires it.

---

## Admin API Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Validate and activate immediately | Successful create/update persists config, refreshes runtime registry/config, and makes provider eligible immediately. | Yes |
| Save then explicit apply | Changes are persisted but not used until explicit apply/reload. | |
| Restart required | Durable config is picked up only on restart. | |

**User's choice:** Validate and activate immediately.
**Notes:** User selected provider CRUD plus test connection and asked that the Admin Console provider configuration screen expose a Test Connection button. User chose dedicated admin bearer token, disable-first deletion, structured field errors, minimal audit table, transactional save plus reload, replace-secret-on-update rotation, cheapest safe provider test call, versioned admin routes, optimistic concurrency, configurable audit retention, idempotency keys for all mutations, redacted secret metadata only, and basic provider list filters.

---

## Redis Boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Provider health hot state only | Redis stores provider health/probe snapshots and routing-visible health state. | Yes |
| Auth/cache hot state | Redis stores gateway API-key/auth cache and provider health, but not semantic cache/rate limits. | Yes |
| Redis optional, mostly deferred | Define boundaries but keep provider health process-local in Phase 3. | |

**User's choice:** Provider health hot state plus auth/cache hot state.
**Notes:** User chose optional Redis by backend capabilities, local hot-state fallback if Redis is unavailable, no secrets in Redis, configurable Redis namespace prefix, explicit TTLs for health/auth state, Redis pub/sub for multi-instance config-change notifications, and documented single-instance/local consistency when Redis is absent.

---

## the agent's Discretion

- Exact storage package structure, migration tooling, encryption implementation, repository interfaces, and Admin API payload field names may be chosen during planning/implementation.

## Deferred Ideas

- Semantic cache, rate limiting, cost governance, streaming, full Admin Console implementation if outside backend scope, admin users/sessions/RBAC, external secret manager integration, and secret version rollback.
