# Phase 13: PostgreSQL Compatibility - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-03
**Phase:** 13-PostgreSQL Compatibility
**Areas discussed:** Deployment, PostgreSQL repository parity, pgvector semantic path, migration and verification

---

## Deployment Packaging

| Option | Description | Selected |
|--------|-------------|----------|
| Same compose, optional profile | Add PostgreSQL + pgvector to existing `docker-compose.yml` behind a profile-style path. | |
| Separate compose override | Keep current compose untouched and add a PostgreSQL/pgvector override. | ✓ |
| Docs only for now | Document external PostgreSQL/pgvector setup without local compose support. | |

**User's choice:** Separate compose override.
**Notes:** Keep Plan 1/2 compose lightweight; Plan 4 gets explicit opt-in packaging.

---

## Dependency Failure Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Fail closed | PostgreSQL or pgvector unavailable means startup fails. | |
| Relational fail closed, vector degrade | PostgreSQL control state unavailable fails startup; pgvector unavailable degrades semantic/vector capability. | ✓ |
| Best-effort degrade | Degrade to disabled/noop/local behavior where possible. | |

**User's choice:** Relational fail closed, vector degrade.
**Notes:** Core forwarding may continue only when relational control state is healthy.

---

## Configuration Examples

| Option | Description | Selected |
|--------|-------------|----------|
| Example config + `.env.example` | Add secret-safe example files with placeholders only. | ✓ |
| README snippet only | List environment variables in README/runbook only. | |
| Compose override inline env | Put all placeholder variables in the compose override. | |

**User's choice:** Example config + `.env.example`.
**Notes:** No real DSNs, passwords, API keys, provider secrets, or encryption keys in git-managed source.

---

## Readiness and Health Detail

| Option | Description | Selected |
|--------|-------------|----------|
| Coarse public, detailed admin/internal | Public `/readyz` shows ready/not ready only; dependency details are admin/internal. | ✓ |
| Public readyz includes dependency names | Public readiness can name PostgreSQL/pgvector failures. | |
| Logs only | Health remains coarse; details only in logs. | |

**User's choice:** Coarse public, detailed admin/internal.
**Notes:** Extends Phase 12 topology secrecy and avoids leaking backend deployment details to ordinary callers.

---

## Plan 4 Smoke Verification

| Option | Description | Selected |
|--------|-------------|----------|
| Compose + migration + chat smoke | Start override, run migrations, start gateway, verify one OpenAI-compatible chat request uses PostgreSQL-backed control state. | ✓ |
| Database-only smoke | Verify containers, migrations, and repository reads/writes only. | |
| Docs-only manual checklist | Provide a manual runbook with no automated smoke. | |

**User's choice:** Compose + migration + chat smoke.
**Notes:** Smoke should prove the gateway path, not just database availability.

---

## PostgreSQL Repository Parity Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Active gateway parity | Cover provider CRUD, API keys, routing, rates/usage settlement, semantic cache metadata, fallback log, semantic rules. | |
| SQLite full parity | Match SQLite repository methods, including `LimitRules` and `SessionBlacklist`. | ✓ |
| Minimum Plan 4 boot parity | Only cover boot and chat-smoke required paths; fail closed elsewhere. | |

**User's choice:** SQLite full parity.
**Notes:** Avoid hidden nil/stub traps in Plan 4.

---

## Missing Repository Capability Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Explicit unsupported error | Missing methods fail closed with clear unsupported errors and blocking planner tasks. | ✓ |
| Complete everything before continuing | No unsupported repository capability may remain. | |
| Allow nil for disabled | Keep nil-return behavior and let callers branch. | |

**User's choice:** Explicit unsupported error.
**Notes:** No nil repo or fake success for unsupported PostgreSQL behavior.

---

## Capability Profile

| Option | Description | Selected |
|--------|-------------|----------|
| Truthful per-feature flags | Mark true only for implemented and verified PostgreSQL capabilities. | ✓ |
| Target-state flags | Mark target parity up front and make implementation catch up. | |
| Single Plan4Enabled flag | Use a coarse Plan 4 capability flag. | |

**User's choice:** Truthful per-feature flags.
**Notes:** Capability profile must match verified runtime behavior.

---

## pgvector Role

| Option | Description | Selected |
|--------|-------------|----------|
| Plan 4 semantic vector store | Use pgvector only when `SEMANTIC_CACHE_VECTOR_STORE=pgvector`; Plans 1/2 keep Qdrant. | ✓ |
| Only migration-compatible storage | Store metadata/vector bytes but do not use pgvector search yet. | |
| Replace Qdrant everywhere | Make pgvector the default vector store. | |

**User's choice:** Plan 4 semantic vector store.
**Notes:** pgvector is Plan 4 only.

---

## pgvector Search Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Exact vector search first | Start with direct distance ordering; add indexes later. | |
| HNSW/IVFFlat from day one | Include approximate indexes and tunable parameters in the first version. | ✓ |
| Postgres metadata + app cosine | Store vectors in PostgreSQL but search in application code. | |

**User's choice:** HNSW/IVFFlat from day one.
**Notes:** Planner should include indexing and parameter tuning in Phase 13 scope.

---

## Vector Dimension Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Config-required dimension | Require explicit dimension configuration. | |
| Infer from first insert | Infer/create dimension on first write. | |
| Fixed default dimension | Hard-code a common dimension. | |
| Explicit config with reasonable default | Make dimension visible/configurable, provide a safe default, and validate actual embeddings. | ✓ |

**User's choice:** Explicit config with reasonable default.
**Notes:** User clarified: "要求显示设置，但提供合理的默认值".

---

## pgvector Privacy and Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse semantic cache scope | Filter by existing `scope + model`; never store raw prompts. | ✓ |
| Stricter tenant namespace | Add collection/schema namespace beyond `scope + model`. | |
| Single shared collection | Use one collection and rely only on metadata filters. | |

**User's choice:** Reuse semantic cache scope.
**Notes:** Keep the existing privacy model and avoid raw prompt storage.

---

## Migration Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Runbook + idempotent migrator | Provide operator runbook and repeatable migration command/script. | ✓ |
| Runbook only | Manual steps and SQL/command examples only. | |
| Online dual-write migration | Dual-write SQLite/PostgreSQL and cut over live traffic. | |

**User's choice:** Runbook + idempotent migrator.
**Notes:** Cover supported control-state records: providers, secrets, routing, API keys, rates, usage, semantic cache, and fallback log.

---

## Migration Failure Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Stop with report | Stop immediately and report completed/failed tables plus repair steps. | ✓ |
| Best-effort continue | Record errors and continue other tables. | |
| Transactional all-or-nothing | Wrap all migration work in one transaction and roll back on failure. | |

**User's choice:** Stop with report.
**Notes:** Do not auto-rollback everything or skip failed tables.

---

## the agent's Discretion

- Exact file names for compose override, example config, migrator, and smoke script.
- Exact HNSW/IVFFlat defaults and parameter names, provided they are documented and configurable.
- Exact Phase 13 plan split, as long as all requirements remain covered.

## Deferred Ideas

- BFF/Admin Console UI for deployment health.
- Replacing SQLite as default deployment.
- Replacing Qdrant in Plans 1/2.
- Online dual-write migration and live cutover.
