# Phase 21: Observability, Admin APIs & Tooling - Context

**Gathered:** 2026-07-05T22:21:21-07:00
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 21 adds operator-facing Scheduler observability and safe admin tooling for the v7.6 Scheduler 1.0 polish path. It covers scheduler status, runtime SLA rule controls, safe training-sample export, precise semantic-neighbor sample hydration, semantic-neighbor embedding model config, SchedulerType attribution, and heuristic override tooling.

This phase does not add an Admin Console UI, persistent SLA-rule storage, scheduler-owned queueing, scheduler-owned vector lookup, or automatic rollout decisions. Scheduler remains optional, disabled by default, fail-open, and stateless. Gateway continues to own queueing, task state, execution, semantic lookup, SLA promotion, fallback, and sensitive-payload boundaries.

</domain>

<decisions>
## Implementation Decisions

### Scheduler Status Endpoint
- **D-01:** Add a new `GET /admin/v1/scheduler/status` endpoint. Keep existing rollout endpoints compatible instead of folding status into rollout.
- **D-02:** Status responses are partial when a component is unavailable. Return the available data plus a `warnings` list rather than failing the whole request.
- **D-03:** Quality rollups use a `limit` query parameter with default `100`.
- **D-04:** Expose a per-component status summary: queue depth, executor `slots_used` / `slots_total`, and circuit-breaker state per scorer when available.
- **D-05:** Planner may choose exact warning strings and field names, but the response must be stable enough for operator scripts and must not include raw prompts, embeddings, provider payloads, auth headers, API keys, or secrets.

### SLA Rules Admin API
- **D-06:** Runtime SLA-rule updates replace the whole in-memory rule set atomically.
- **D-07:** Runtime admin changes are in-memory only. On restart, rules revert to config-file rules.
- **D-08:** If any submitted rule is invalid, reject the whole replacement and keep the previous active rule set.
- **D-09:** Audit successful replacements with old/new counts and safe rule keys only: policy IDs, tenant class where present, model class, and request kind. Do not audit full submitted rule bodies or tenant/user identifiers for this replacement operation.
- **D-10:** Use the existing admin scheduler namespace and auth/writeability protections. Exact route naming is planner discretion; prefer the smallest clear route such as `/admin/v1/scheduler/sla-rules`.

### Training Sample Export
- **D-11:** The export endpoint supports JSON and NDJSON. JSON is the default unless the request asks for NDJSON.
- **D-12:** Filters are optional: `start`, `end`, `task_type`, and `limit`. With no filters, return a recent capped export.
- **D-13:** Use split `features` and `labels` objects for each exported sample, matching the safe training-data shape.
- **D-14:** Include low-cardinality fields only, such as model class, request kind, priority, semantic coverage, scheduler version/type, outcome, latency/token labels, and provider class. Exclude tenant IDs, user IDs, raw task text, raw prompts, embeddings, semantic-cache payloads, provider payloads, auth headers, API keys, and secrets.
- **D-15:** Export limit defaults to `1000` and has max `10000`.
- **D-16:** Planner may choose `format=ndjson` vs `Accept: application/x-ndjson`; prefer the smallest existing handler pattern.

### Operator Tuning And Attribution
- **D-17:** Add a dedicated semantic-neighbor embedding model config setting under Scheduler: `scheduler.semantic_neighbors_embedding_model` and `SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL`. Default to the current hardcoded model.
- **D-18:** `SchedulerTrainingSampleRepository.ListByIDs` returns found samples only and preserves requested ID order. Missing IDs reduce semantic-neighbor coverage without returning an error.
- **D-19:** `heuristic_config_file` supports only `base_latency` and model-family multiplier overrides in Phase 21. Do not expose the full heuristic config surface yet.
- **D-20:** Provide a small heuristic config template file for operator customization.
- **D-21:** `ScoreResult.SchedulerType` must be populated everywhere a `ScoreResult` is produced: FIFO, heuristic, ONNX, predictive, fallback, gRPC/proto decode, weighted/merged paths, and any other scoring path before quality evidence is recorded.

### Agent Discretion
- Planner may choose exact JSON field names, helper names, route constants, template file location, query parameter parsing helpers, warning wording, and validation error text.
- Keep Phase 21 changes narrow. Prefer existing admin handler/service/repository patterns over new abstractions.
- If current code has not fully caught up with Phase 20 config-unification decisions, follow Phase 20 context as the intended v7.6 direction while keeping the Phase 21 diff as small as possible.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 21 scope and v7.6 milestone boundary.
- `.planning/REQUIREMENTS.md` - SCH-08, QDR-07, QDR-08, OBS-03 through OBS-06.
- `.planning/PROJECT.md` - Project-level scheduler optionality, gateway ownership, low-latency, and sensitive-payload constraints.
- `.planning/phases/20-config-unification-scheduler-core-hardening/20-CONTEXT.md` - Nested config direction, executor/concurrency assumptions, QueueGuard observability, and semantic-neighbor startup/config decisions.
- `.planning/phases/19-sla-waiting-time-promotion/19-CONTEXT.md` - SLA promotion boundaries, safe rule dimensions, sanitized audit/metrics precedent.
- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-CONTEXT.md` - Scheduler/predictor boundary, fail-open behavior, and SchedulerType/quality attribution context.

### Admin API And App Wiring
- `internal/http/router.go` - Current admin scheduler rollout routes and admin auth/writeability pattern.
- `internal/http/handlers/admin_scheduler.go` - Current JSON handler style for scheduler admin GET/PATCH.
- `internal/http/handlers/admin_scheduler_test.go` - Existing admin scheduler auth, validation, audit, and SQLite-backed test patterns.
- `internal/app/app.go` - Scheduler runner, rollout controller, SLA promoter, admin service, quality recorder, and semantic-neighbor wiring.
- `internal/app/semantic_neighbors.go` - Semantic-neighbor service construction and embedder provider selection.

### Scheduler Runtime
- `internal/scheduler/admin_scheduler_service.go` - Current rollout status/update service and audit helper.
- `internal/scheduler/client.go` - FIFO, gRPC, weighted scorer, fallback, merge, breaker, and SchedulerType assignment paths.
- `internal/scheduler/executor.go` - Executor and synchronous runner flow for queue draining and completion evidence.
- `internal/scheduler/intake.go` - Feature enrichment, scoring, queue admission, metadata capture, and queue-depth metric point.
- `internal/scheduler/quality.go` - Quality rollup evidence reading `scheduler_type` metadata.
- `internal/scheduler/sla_promotion.go` - Current SLA promotion rule matching, promotion evidence, audit, and metrics.
- `internal/scheduler/training.go` - Safe structured training-sample construction.
- `internal/scheduler/types.go` - `TaskFeature`, `ScoreResult`, `SchedulerType`, and safe scalar/enum fields.
- `internal/scheduler/semantic_neighbors.go` - Current embedding model constant, broad hydration scan, aggregate behavior, and fail-open defaults.

### Repositories And Config
- `internal/controlstate/repository.go` - `SchedulerTrainingSampleRepository` and `SchedulerQualityRollupRepository` contracts to extend.
- `internal/controlstate/sqlite/scheduler_training_samples.go` - SQLite training sample insert/list query and scanner.
- `internal/controlstate/postgres/scheduler_training_samples.go` - PostgreSQL training sample insert/list query and scanner.
- `internal/controlstate/sqlite/scheduler_quality_rollups.go` - SQLite quality rollup list filtering and limit behavior.
- `internal/controlstate/postgres/scheduler_quality_rollups.go` - PostgreSQL quality rollup list filtering and limit behavior.
- `internal/config/config.go` - Scheduler config fields, env loading, JSON merge, and defaults.
- `internal/config/config_validation.go` - Scheduler validation patterns, SLA rule validation, and request-kind validation.
- `internal/scheduler/heuristic/config.go` - Default base latency table and model multipliers.
- `internal/scheduler/heuristic/score.go` - Heuristic scoring consumption of base latency and model multipliers.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `AdminSchedulerService.Status` already returns rollout status and quality rollups; extend or split from this pattern for the new status endpoint.
- `AdminSchedulerHandler` already uses `json.Decoder.DisallowUnknownFields`, `sendAdminError`, and direct JSON encoding.
- Router admin groups already apply `AdminAuth` and `RequireWritable`; reuse them for status, SLA rules, and export where appropriate.
- `SchedulerQualityRollupRepository.ListByWindow` already supports time window, scheduler type/version/task type filters, and limit.
- `SchedulerTrainingSampleRepository.ListByWindow` already exists for both SQLite and PostgreSQL and can be reused for export filters before adding `ListByIDs`.
- `SLAPromoter` already holds rules in memory and emits sanitized audit/metrics evidence.
- `controlstate.SafeAuditMetadata` already provides audit metadata sanitization.
- `TrainingRecorder` already writes only safe structured features and completion labels.

### Established Patterns
- Optional scheduler features must fail open and must not block gateway forwarding.
- Gateway owns queueing, execution, task state, semantic lookup, SLA promotion, and fallback.
- Scheduler receives and returns safe scalar/enum data only.
- Observability labels and admin responses must stay sanitized and low-cardinality.
- Durable control state has SQLite/PostgreSQL parity; repository changes need both implementations.
- Backward compatibility matters for existing admin rollout endpoints and config/env names.

### Integration Points
- Add `GET /admin/v1/scheduler/status` in `internal/http/router.go` and `internal/http/handlers/admin_scheduler.go`.
- Pass the minimum runtime handles needed by `AdminSchedulerService` to read queue depth, executor slots, breaker snapshots, quality rollups, SLA rules, and training samples.
- Add an in-memory SLA rule replacement path that updates the active `SLAPromoter` without persisting to control state.
- Add safe export projection from `controlstate.SchedulerTrainingSample` into `features` and `labels` response objects.
- Extend `SchedulerTrainingSampleRepository` with `ListByIDs` in the interface plus SQLite/PostgreSQL implementations.
- Replace semantic-neighbor hydration's broad time-window scan with precise ID lookup.
- Add `SemanticNeighborsEmbeddingModel` to scheduler config loading/defaults/validation and wire it into `SemanticNeighborService.embed`.
- Load `heuristic_config_file` into the heuristic scorer with a narrow override shape for base latency and model multipliers.
- Ensure `SchedulerType` is set before `scoreMetadata` captures scheduler evidence.

</code_context>

<specifics>
## Specific Ideas

- Default status rollup `limit` is `100`; training export default `limit` is `1000` with max `10000`.
- Status should be usable during incidents, so partial data with warnings is preferred over all-or-nothing errors.
- Runtime SLA replacement should be boring: validate the full submitted set, swap once, audit counts and safe keys.
- JSON export should be friendly for admin tools; NDJSON is available for larger pulls without making it the default.
- Heuristic override scope is intentionally small: only the two tables required by OBS-06.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

*Phase: 21-Observability, Admin APIs & Tooling*
*Context gathered: 2026-07-05T22:21:21-07:00*
