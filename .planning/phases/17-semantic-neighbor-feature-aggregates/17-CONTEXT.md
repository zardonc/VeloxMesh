# Phase 17: Semantic Neighbor Feature Aggregates - Context

**Gathered:** 2026-07-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Add optional Qdrant/vector-backed semantic-neighbor aggregate features for scheduler tasks while keeping raw prompts, embeddings, semantic-cache payloads, auth headers, API keys, and provider secrets out of Scheduler and training logs.

The Gateway owns semantic lookup and sends Scheduler only bounded numeric or enum aggregate fields. Scheduler remains a stateless scoring oracle.

</domain>

<decisions>
## Implementation Decisions

### Neighbor Source
- **D-01:** Use completed scheduler training samples as the only semantic-neighbor source. Do not use semantic-cache metadata in Phase 17.
- **D-02:** Include all completed outcomes, including success, failure, and timeout samples, so aggregate counts and rates reflect real completion behavior.
- **D-03:** Isolate neighbors by `tenant + model_class + request_kind`. If tenant scope lacks enough samples, fall back to `model_class + request_kind`.
- **D-04:** Require a minimum sample count before using scoped stats. Default minimum is `20`, configurable by admin/config.

### Aggregate Shape
- **D-05:** Add core prediction stats: `neighbor_count`, `latency_p50_ms`, `latency_p90_ms`, `latency_stddev_ms`, `output_tokens_p70`, `success_rate`, `timeout_rate`, `coverage_level`, and `coverage_ratio`.
- **D-06:** Represent coverage as both an enum (`coverage_level`) and numeric ratio (`coverage_ratio`).
- **D-07:** Store semantic aggregate fields as first-class `TaskFeature` fields in proto, Go structs, training samples, export schema, and ONNX feature preparation. Avoid a generic metadata map.
- **D-08:** All fields must have bounded safe defaults when semantic enrichment is disabled, unavailable, timed out, or unsupported by a model artifact.

### Timeout And Fallback Behavior
- **D-09:** Run semantic enrichment in Gateway after safe feature extraction and before the scheduler RPC.
- **D-10:** The feature is disabled by default. Administrators can configure enablement, `min_count=20`, per-task timeout `5ms`, and batch hard cap `15ms`.
- **D-11:** Enrichment fails open: record sanitized metrics and use safe defaults. Do not surface enrichment errors to scheduler callers.
- **D-12:** Metrics should cover attempts, timeouts, errors, and fallback reasons using low-cardinality sanitized labels only.

### Scoring Use
- **D-13:** Training and ONNX scoring may consume semantic aggregates immediately when the artifact supports them.
- **D-14:** If the ONNX artifact does not support these fields, runtime scoring uses neutral/default values without requiring a retrain first.
- **D-15:** Heuristic scoring may observe semantic fields for evidence/metrics only and must not alter scores in Phase 17. FIFO fallback ignores semantic aggregates.

### Agent Discretion
- The planner may choose exact names, config placement, and internal helpers as long as the above boundaries hold and existing scheduler patterns are reused.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 17 goal, success criteria, and candidate plan slices.
- `.planning/REQUIREMENTS.md` - QDR-01 through QDR-04 and v7.5 out-of-scope rules.
- `.planning/PROJECT.md` - Project-level gateway and scheduler boundaries.
- `.planning/phases/16-a-b-rollout-and-prediction-quality/16-CONTEXT.md` - ONNX rollout and prediction quality baseline.
- `.planning/phases/15-training-feedback-and-onnx-path/15-CONTEXT.md` - Training sample/export and ONNX feature path.
- `.planning/phases/14-scheduler-queue-foundation/14-CONTEXT.md` - Scheduler queue foundation and stateless scoring boundary.

### Code
- `proto/scheduler/v1/scheduler.proto` - Scheduler RPC and `TaskFeature` contract.
- `internal/scheduler/features.go` - Safe feature extraction and defaulting path.
- `internal/scheduler/types.go` - Scheduler feature/result types.
- `internal/scheduler/training.go` - Training sample recording/export path.
- `internal/storage/interfaces.go` - Vector adapter abstraction.
- `internal/storage/qdrant.go` - Qdrant vector implementation boundary.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ExtractSafeFeatures` - add semantic aggregates after safe extraction without exposing raw request content.
- `TaskFeature` - extend with first-class bounded numeric/enum fields.
- `TrainingRecorder` - carry fields into durable training samples and export.
- `VectorAdapter` / `QdrantVectorAdapter` - reuse existing vector boundary for Gateway-owned lookup.
- Existing ONNX/training path - add defaults and artifact compatibility instead of creating a parallel model path.

### Established Patterns
- Scheduler-related features are optional and disabled by default.
- Gateway owns queueing, execution, fallback, and semantic lookup boundaries.
- Scheduler receives safe scalar features and remains stateless.
- Observability uses sanitized, low-cardinality labels.
- Missing optional dependencies must degrade without blocking data-plane forwarding.

### Integration Points
- Config validation for enablement, `min_count`, and timeout budgets.
- Gateway feature enrichment between safe feature extraction and scheduler RPC.
- Scheduler proto/Go mapping for new first-class fields.
- Training sample persistence/export and ONNX feature preparation.
- Metrics for attempts, timeouts, errors, and fallback reasons.

</code_context>

<specifics>
## Specific Ideas

- Default `min_count` is `20`, and administrators can change it.
- Coverage must be available as both enum and ratio.
- Heuristic scoring stays behaviorally unchanged in Phase 17 to preserve the v7.4 baseline.
- Tenant-specific neighbor stats are preferred, with model/request-kind fallback for sparse tenants.

</specifics>

<deferred>
## Deferred Ideas

- Semantic-cache metadata as a neighbor source is deferred.
- Heuristic score adjustment using semantic aggregates is deferred.
- Anomaly/OOD thresholds and conservative scoring belong to Phase 18.
- SLA waiting-time promotion belongs to Phase 19.

</deferred>

---

*Phase: 17-Semantic Neighbor Feature Aggregates*
*Context gathered: 2026-07-04*
