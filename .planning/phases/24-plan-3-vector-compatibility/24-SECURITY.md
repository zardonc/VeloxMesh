---
phase: 24
slug: plan-3-vector-compatibility
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-08
verified: 2026-07-08
register_authored_at_plan_time: false
---

# Phase 24 - Security

Retroactive STRIDE register for Plan 3 vector compatibility.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Gateway -> embedding provider | Text is embedded before semantic cache or semantic-neighbor lookup. | Request text bounded by semantic-neighbor input limit |
| Gateway -> Qdrant | Explicit Qdrant vector backend stores embeddings and caller-provided safe metadata. | Vectors plus IDs/scope/model/sample metadata |
| Gateway -> PostgreSQL pgvector | Plan 4/pgvector backend stores embeddings and allowlisted metadata. | Vectors plus allowlisted metadata |
| Operator config -> vector backend selection | Config chooses LanceDB default, explicit Qdrant, or pgvector. | Backend type, endpoint/DSN, vector dimension |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-24-01 | Information Disclosure | Semantic cache vector metadata | mitigate | `SemanticCacheService.Store` constructs metadata from `id`, `scope`, `model`, `response`, and optional `usage_id`, never raw prompt text; `TestSemanticCacheVectorMapsThroughRepository` verifies the `prompt` key is not inserted. | closed |
| T-24-02 | Information Disclosure | Semantic-neighbor vector metadata | mitigate | `SemanticNeighborService.IndexCompletedSample` inserts `sample_id`, `tenant`, `model_class`, `request_kind`, `outcome`, and `completed_at`; `TestSemanticNeighborIndexerWritesSafeMetadata` rejects prompt, embedding, api_key, authorization, and semantic_cache_payload keys. | closed |
| T-24-03 | Information Disclosure | pgvector metadata | mitigate | `safePGVectorMetadata` allowlists `id`, `scope`, `model`, `usage_id`, and `response`; `TestPGVectorMetadataAllowlist` and real `TestPGVectorMigrationAndSearch` verify raw prompt metadata is not returned. | closed |
| T-24-04 | Tampering | Qdrant endpoint config | mitigate | `qdrantClientConfig` rejects invalid schemes/ports and infers TLS from HTTPS; `TestQdrantClientConfigRejectsInvalidAddress` and TLS inference tests verify config parsing. | closed |
| T-24-05 | Denial of Service | Qdrant collection setup | mitigate | Qdrant collection creation and insert reuse are verified against a real Qdrant service; app startup fail-open behavior is covered by semantic-neighbor startup tests. | closed |
| T-24-06 | Tampering | pgvector collection schema | mitigate | `PGVectorAdapter.EnsureCollection` rejects dimension mismatch; `TestPGVectorEnsureCollectionUsesRealSchema` verifies real schema setup and mismatch handling. | closed |
| T-24-07 | Information Disclosure | Scheduler privacy contract | mitigate | Scheduler feature/proto/export field names are tested against forbidden terms including prompt, message, api_key, authorization, secret, embedding, payload, raw, and semantic_cache. | closed |

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-24-01 | T-24-04 | Real Qdrant validation logged plaintext API-key transport warnings for the local `.env` endpoint. This is accepted only for local validation; production should use HTTPS or trusted private networking. | project owner via v7.7 local validation scope | 2026-07-08 |
| AR-24-02 | T-24-03 | Real pgvector validation logged plaintext Postgres password warnings for the local test DSN. This is accepted only for local validation; production DSNs should require TLS where crossing an untrusted network. | project owner via v7.7 local validation scope | 2026-07-08 |

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-08 | 7 | 7 | 0 | Codex inline retroactive STRIDE |

## Verification Evidence

- `go test -v -timeout 60s ./internal/cache ./internal/storage -run 'TestSemanticCacheVectorMapsThroughRepository|TestPGVectorMetadataAllowlist|TestPGVectorMigrationAndSearch|TestQdrantClientConfigRejectsInvalidAddress|TestQdrantEnsureCollectionCreatesRealCollection|TestQdrantInsertReusesEnsureCollection' -count=1` passed.
- `go test -v -timeout 60s ./internal/app -run 'TestApp_SemanticNeighborsEnsureCollectionKeepsServiceEnabled|TestApp_SemanticNeighborsPGVectorEnsureKeepsServiceEnabled' -count=1` passed against real Qdrant and pgvector/Postgres services.
- `go test -v -timeout 60s ./internal/scheduler -run 'TestSchedulerPrivacyContractFieldNames|TestSemanticNeighborIndexerWritesSafeMetadata' -count=1` passed.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.

## Sign-Off

- [x] All threats have a disposition.
- [x] Accepted risks documented in Accepted Risks Log.
- [x] `threats_open: 0` confirmed.
- [x] `status: verified` set in frontmatter.

Approval: verified 2026-07-08

