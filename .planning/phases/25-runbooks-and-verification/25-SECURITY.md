---
phase: 25
slug: runbooks-and-verification
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-08
verified: 2026-07-08
register_authored_at_plan_time: false
---

# Phase 25 - Security

Retroactive STRIDE register for v7.7 runbooks and verification artifacts.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Documentation -> operator environment | Operators may copy sample env/config files into local deployments. | Config keys, placeholder values, enablement flags |
| Admin scheduler APIs -> audit log | Scheduler admin actions record audit metadata. | Policy IDs, tenant/model/request metadata, outcomes |
| Scheduler metrics/logging -> observability systems | Scheduler promotion and queue behavior emits labels and logs. | Sanitized labels and scalar status values |
| Verification docs -> release decision | Planning artifacts summarize test and component evidence. | Commands, pass/fail evidence, known limits |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-25-01 | Information Disclosure | Copyable examples | mitigate | `TestEnvExampleSchedulerDisabledAndSecretSafe`, `TestConfigExamplesParseAndStayDisabled`, and `TestCopyableExamplesDoNotContainSecretShapedValues` verify examples stay disabled and contain no `sk-`, password, token, or inline API-key shaped values. | closed |
| T-25-02 | Tampering | Operator enablement docs | mitigate | README/runbook document memory queue defaults, explicit Redis queueing, Plan 3 single-node limits, and LanceDB/Qdrant no-migration scope; source assertions in `25-VALIDATION.md` verify required strings. | closed |
| T-25-03 | Information Disclosure | Admin scheduler audit metadata | mitigate | `TestAdminSchedulerAuditMetadataIsSanitized`, `TestAdminSchedulerSLARulesReplaceAuditsSafeMetadata`, and `TestSafeAuditMetadata` verify audit metadata is allowlisted. | closed |
| T-25-04 | Information Disclosure | Scheduler SLA logs and metrics | mitigate | `TestSLAPromoterPromotedWritesSanitizedAuditAndLog`, `TestSLAPromoterBlockedWritesSanitizedAuditAndLog`, and `TestPrometheusSchedulerSLAPromotionLabelsAreSanitized` verify sanitized audit/log/metric evidence. | closed |
| T-25-05 | Repudiation | Verification evidence | mitigate | `25-VALIDATION.md` records real Redis, Qdrant, pgvector/Postgres, app startup, full test, and build commands; skipped Plan4 provider smoke is explicitly not counted. | closed |
| T-25-06 | Information Disclosure | Semantic cache docs | mitigate | `TestSecretSafe` and `TestSemanticCacheVectorMapsThroughRepository` verify semantic cache errors and vector metadata do not leak raw prompt material. | closed |

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-25-01 | T-25-05 | External real-provider Plan4 smoke was skipped because `PLAN4_*` provider credentials were not present. It is documented as optional and not counted as security closure evidence. | project owner via v7.7 local validation scope | 2026-07-08 |
| AR-25-02 | T-25-02 | Local component validation used plaintext Redis/Qdrant/Postgres endpoints from `.env`; docs and logs surface this as local validation behavior, not a production transport recommendation. | project owner via v7.7 local validation scope | 2026-07-08 |

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-08 | 6 | 6 | 0 | Codex inline retroactive STRIDE |

## Verification Evidence

- `go test -v -timeout 60s ./internal/config -run 'TestEnvExampleSchedulerDisabledAndSecretSafe|TestConfigExamplesParseAndStayDisabled|TestCopyableExamplesDoNotContainSecretShapedValues' -count=1` passed.
- `go test -v -timeout 60s ./internal/http/handlers ./internal/observability ./internal/controlstate -run 'TestAdminSchedulerAuditMetadataIsSanitized|TestAdminSchedulerSLARulesReplaceAuditsSafeMetadata|TestPrometheusSchedulerSLAPromotionLabelsAreSanitized|TestSafeAuditMetadata' -count=1` passed.
- `go test -v -timeout 60s ./internal/scheduler -run 'TestSLAPromoterPromotedWritesSanitizedAuditAndLog|TestSLAPromoterBlockedWritesSanitizedAuditAndLog|TestSLAPromoterSkipsAuditForDisabledNotEligibleAndError' -count=1` passed.
- `go test -v -timeout 60s ./internal/cache -run 'TestSecretSafe|TestSemanticCacheVectorMapsThroughRepository' -count=1` passed.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.

## Sign-Off

- [x] All threats have a disposition.
- [x] Accepted risks documented in Accepted Risks Log.
- [x] `threats_open: 0` confirmed.
- [x] `status: verified` set in frontmatter.

Approval: verified 2026-07-08

