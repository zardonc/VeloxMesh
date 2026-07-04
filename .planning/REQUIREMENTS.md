# Requirements: VeloxMesh

**Defined:** 2026-07-03
**Core Value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## v7.3 Requirements

### PostgreSQL Deployment

- [x] **PG-01**: Operators can run Plan 4 with Redis Stack, PostgreSQL, and pgvector using documented local deployment steps.
- [x] **PG-02**: Operators can enable PostgreSQL through configuration without hardcoded DSNs, passwords, or source-controlled secrets.
- [x] **PG-03**: Gateway startup clearly reports PostgreSQL readiness and fails closed when required Plan 4 dependencies are unavailable.

### Control State Compatibility

- [x] **CTRL-01**: PostgreSQL control-state repositories support the active gateway capabilities used by routing, provider CRUD, API keys, usage settlement, semantic cache, and fallback logging.
- [x] **CTRL-02**: PostgreSQL capability reporting matches the features actually implemented for Plan 4.
- [x] **CTRL-03**: PostgreSQL behavior is covered by focused integration tests that can run with an externally supplied test DSN.

### pgvector Semantic Path

- [x] **VECT-01**: Plan 4 can store and search semantic-cache embeddings through pgvector behind the existing vector adapter boundary.
- [x] **VECT-02**: pgvector search preserves tenant/API-key scoping and never stores raw prompts or provider secrets.

### Migration and Verification

- [x] **MIGR-01**: Operators can migrate supported SQLite control-state data into PostgreSQL with a repeatable command or documented runbook.
- [x] **MIGR-02**: Migration preserves provider configs, encrypted provider secrets, routing config, API keys, rates, usage records, semantic cache metadata, and fallback log state.
- [x] **TEST-01**: Phase 13 verification proves the gateway can serve OpenAI-compatible chat traffic against the PostgreSQL-compatible Plan 4 deployment.

## Future Requirements

### Admin Console

- **ADMIN-01**: BFF/Admin Console can expose PostgreSQL deployment health after Phase 11 exists.

### Limit Rule Expansion

- **LIMIT-01**: Full `LimitRule` unification across all scopes can be completed after the Plan 4 compatibility path is stable.

## Out of Scope

| Feature | Reason |
| --- | --- |
| BFF/Admin Console UI | Phase 13 prioritizes the backend deployment path; UI remains Phase 11 scope. |
| Replacing SQLite as the default deployment | Plans 1/2 stay SQLite + Redis Stack + Qdrant; PostgreSQL is an extension path. |
| Replacing Qdrant for Plans 1/2 | pgvector is only required for Plan 4 compatibility. |
| Raw prompt storage | Existing semantic-cache privacy constraints remain in force. |

## Traceability

| Requirement | Phase | Status |
| --- | --- | --- |
| PG-01 | Phase 13 | Complete |
| PG-02 | Phase 13 | Complete |
| PG-03 | Phase 13 | Complete |
| CTRL-01 | Phase 13 | Complete |
| CTRL-02 | Phase 13 | Complete |
| CTRL-03 | Phase 13 | Complete |
| VECT-01 | Phase 13 | Complete |
| VECT-02 | Phase 13 | Complete |
| MIGR-01 | Phase 13 | Complete |
| MIGR-02 | Phase 13 | Complete |
| TEST-01 | Phase 13 | Complete |

**Coverage:**

- v7.3 requirements: 11 total
- Mapped to phases: 11
- Unmapped: 0

---
*Requirements defined: 2026-07-03*
*Last updated: 2026-07-03 after starting v7.3 PostgreSQL Compatibility*
