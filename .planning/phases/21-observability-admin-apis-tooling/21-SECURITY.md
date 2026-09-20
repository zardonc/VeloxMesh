---
phase: 21
slug: observability-admin-apis-tooling
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-06T21:37:07Z
verified_at: 2026-07-06T21:37:07Z
---

# Phase 21 - Security

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Admin API -> scheduler runtime | Authenticated operators read status and replace in-memory SLA rules. | Runtime status, SLA rule projections, replacement requests |
| Admin API -> training sample repository | Operators export safe scheduler training data. | Whitelisted training features and labels |
| Vector search payload -> repository hydration | Vector result IDs drive exact sample lookup. | Sample IDs from vector search |
| Operator config -> embedding provider | Config selects model identifier sent to embedding provider. | Embedding model name |
| Operator config -> heuristic scorer | Override file changes local scoring tables. | Base latency and model multiplier values |
| Scoring path -> quality rollups | SchedulerType metadata is recorded for observability. | Scheduler type/version metadata |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status | Evidence |
|-----------|----------|-----------|-------------|------------|--------|----------|
| T-21-01-01 | Information Disclosure | status endpoint | mitigate | Return only low-cardinality status, warnings, and quality rollups; exclude raw payloads and secrets. | closed | `internal/http/router.go` applies `AdminAuth` and `RequireWritable` to scheduler admin routes; `internal/scheduler/admin_scheduler_service.go` `SchedulerRuntimeStatus` exposes rollout, queue, slots, breaker, rollups, warnings only. Tests: `TestAdminSchedulerStatusRequiresAdminAuth`, `TestAdminSchedulerStatusWarnsWhenRuntimeComponentsUnavailable`. |
| T-21-01-02 | Tampering | SLA rule replacement | mitigate | Validate the whole submitted set and swap atomically only after all rules pass. | closed | `ReplaceSLARules` copies input, calls `config.ValidateSLAPromotionRules`, then calls `SLAPromoter.ReplaceRules`; `ReplaceRules` swaps under mutex after validation. Test: `TestAdminSchedulerInvalidSLARulesLeaveOldRules`. |
| T-21-01-03 | Repudiation | SLA rule audit | mitigate | Audit successful replacement counts and safe rule keys through existing audit repository. | closed | `recordSLARulesAudit` writes `scheduler.sla_rules.replace` with old/new counts and `safeSLARules`; `safeSLARules` omits tenant IDs and uses tenant selector labels. Test: `TestAdminSchedulerSLARulesReplaceAuditsSafeMetadata`. |
| T-21-02-01 | Information Disclosure | training export | mitigate | Use an explicit safe projection and tests proving excluded sensitive fields stay out. | closed | `TrainingExportSample` contains only `features` and `labels`; `projectTrainingSample` maps explicit fields only. Test: `TestAdminSchedulerTrainingExportJSONAndNDJSONAreSafe`. |
| T-21-02-02 | Denial of Service | export endpoint | mitigate | Default limit to 1000 and cap at 10000. | closed | `defaultTrainingExportLimit`, `maxTrainingExportLimit`, `trainingExportLimit`, and `trainingExportQueryLimit` bound repository reads and filtered exports. |
| T-21-02-03 | Tampering | semantic hydration order | mitigate | Preserve vector result order and omit missing IDs without fabricating data. | closed | `SemanticNeighborService.hydrate` calls `Repo.ListByIDs` for exact IDs and iterates original vector results; SQLite and PostgreSQL `ListByIDs` call `orderTrainingSamplesByID`. Tests cover SQLite, PostgreSQL, and service ordering. |
| T-21-03-01 | Tampering | heuristic override file | mitigate | Accept only base_latency and model_multipliers with unknown fields rejected. | closed | `heuristic.LoadConfigFile` decodes into narrow `overrideConfig` with `DisallowUnknownFields`. Tests: `TestLoadConfigFileRejectsUnknownFields`, `TestSchedulerServiceLoadsHeuristicConfigFile`. |
| T-21-03-02 | Information Disclosure | embedding model config and examples | mitigate | Store model identifier only; examples contain no secrets or payloads. | closed | `SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL` and JSON config flow to `SemanticNeighborService.Config.EmbeddingModel`; `embed` sends only `Model` plus bounded request text. `config.heuristic.example.json` contains only numeric override tables. |
| T-21-03-03 | Repudiation | scheduler quality attribution | mitigate | Ensure all scoring paths set SchedulerType before quality evidence is recorded. | closed | Heuristic, gRPC, FIFO, ONNX, predictive, and fallback paths set SchedulerType; `scoreWithDefaultType` fills FIFO before task metadata is written. Tests include `TestScoreCalculatorSetsSchedulerType` and `TestQualityRecorderUsesScoreSchedulerType`. |

## Accepted Risks Log

No accepted risks.

## Unregistered Flags

None. Phase 21 SUMMARY files did not report `## Threat Flags`.

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-06 | 9 | 9 | 0 | Codex |

## Automated Checks

- `go test -count=1 -timeout 60s ./internal/http ./internal/http/handlers ./internal/scheduler ./internal/scheduler/heuristic ./internal/config ./internal/app ./internal/controlstate/sqlite ./internal/controlstate/postgres ./cmd/scheduler`

## Sign-Off

- [x] All threats have a disposition.
- [x] Accepted risks documented.
- [x] `threats_open: 0` confirmed.
- [x] `status: verified` set in frontmatter.
