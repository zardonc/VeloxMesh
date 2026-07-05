# Phase 18: Anomaly and OOD Conservative Scoring - Context

**Gathered:** 2026-07-04T22:39:15-07:00
**Status:** Ready for planning

<domain>
## Phase Boundary

Let ONNX Scheduler recognize unfamiliar tasks and respond conservatively through confidence and uncertainty signals. This phase adds anomaly/OOD threshold metadata to versioned ONNX artifacts, validates and degrades that metadata clearly at runtime, applies conservative scoring without changing the scheduler RPC contract, and extends quality evidence for anomaly behavior.

Scheduler remains a stateless scoring oracle. Gateway keeps queue ownership, execution, rollback, and FIFO fallback behavior. No raw prompts, embeddings, semantic-cache payloads, authorization headers, API keys, provider secrets, tenant/API-key identity, or original request payloads may enter scheduler artifacts, metrics, logs, or quality records.

</domain>

<decisions>
## Implementation Decisions

### Threshold Scope And Artifact Shape
- **D-01:** Publish anomaly/OOD thresholds by `task_type + coverage_level`, using Phase 17 semantic coverage levels such as `none`, `fallback`, and `tenant`.
- **D-02:** Compute thresholds from successful safe scheduler samples only. Track failure and timeout outcomes separately as comparison evidence; do not mix them into the normal threshold distribution.
- **D-03:** If a `task_type + coverage_level` sample set is too small, fall back to a `task_type` threshold. If the task-type threshold is also too sparse, mark anomaly as unavailable rather than guessing.
- **D-04:** Store threshold metadata in nested manifest shape: `anomaly_thresholds[task_type][coverage_level]`.
- **D-05:** Each threshold entry should include `threshold`, `sample_count`, `mean`, and `stddev`.
- **D-06:** Offline tooling should compute both `mean + k*stddev` and percentile thresholds, then publish the more conservative runtime threshold.

### Artifact Validation And Degradation
- **D-07:** Missing anomaly metadata must not make an otherwise valid ONNX artifact fail startup. ONNX continues to score, while anomaly/OOD behavior is marked unavailable or degraded.
- **D-08:** Structurally invalid anomaly metadata disables anomaly/OOD behavior only; the main ONNX scoring path remains available.
- **D-09:** Degraded anomaly state must be visible in metrics, logs, and admin status.
- **D-10:** Use low-cardinality enum reasons in metrics and admin status, with detailed validation errors only in logs.

### Conservative Scoring
- **D-11:** OOD hits should lower confidence and raise uncertainty rather than changing the scheduler RPC contract.
- **D-12:** Confidence reduction uses a severity-scaled clamp: larger threshold exceedance lowers confidence more, with a non-zero floor.
- **D-13:** Map OOD severity into `TaskFeature.UncertaintyHint` and reuse the existing heuristic calculator uncertainty penalty path.
- **D-14:** Compute OOD severity from observed distance over threshold, expressed as relative exceedance beyond the selected threshold.

### Quality Evidence
- **D-15:** Compare anomaly/OOD quality by `scheduler_version + task_type + coverage_level`.
- **D-16:** Extend durable quality rollups with `anomaly_count`, `anomaly_rate`, and `anomaly_unavailable_count`.
- **D-17:** Live metrics may add low-cardinality `coverage_level` and `anomaly_status` labels.
- **D-18:** Count anomaly unavailable/degraded separately from scheduler fallback rate. Metadata degradation is not a scheduler backend fallback.

### Agent Discretion
- Planner may choose exact enum names, minimum sample defaults, `k` and percentile defaults, and admin status field names as long as the decisions above hold, labels remain low-cardinality, and existing scheduler artifact, scoring, metrics, and control-state patterns are reused.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 18 goal, success criteria, and candidate plan slices.
- `.planning/REQUIREMENTS.md` - ANOM-01 through ANOM-04 and v7.5 out-of-scope rules.
- `.planning/PROJECT.md` - Project-level scheduler optionality, safety, and stateless scoring boundaries.
- `.planning/phases/17-semantic-neighbor-feature-aggregates/17-CONTEXT.md` - Semantic coverage fields and safe aggregate boundaries that Phase 18 builds on.
- `.planning/phases/16-a-b-rollout-and-prediction-quality/16-CONTEXT.md` - ONNX rollout, prediction quality, MAPE, and low-cardinality evidence decisions.
- `.planning/phases/15-training-feedback-and-onnx-path/15-CONTEXT.md` - Safe sample, offline artifact, and ONNX runtime decisions.

### Runtime Scheduler
- `internal/scheduler/onnx/artifact.go` - Current ONNX manifest loading, semantic metadata validation, checksum, and artifact contract.
- `internal/scheduler/onnx/scorer.go` - Current ONNX scoring, confidence calculation, semantic normalization, and heuristic calculator reuse.
- `internal/scheduler/types.go` - `TaskFeature`, `ScoreResult`, confidence, uncertainty, semantic coverage, and scheduler type fields.
- `internal/scheduler/heuristic/score.go` - Existing virtual-deadline scoring and uncertainty penalty path.
- `cmd/scheduler/main.go` - Scheduler service mode wiring and ONNX startup behavior.

### Evidence And Offline Tooling
- `internal/scheduler/quality.go` - Prediction quality recorder, MAPE, rollup writes, live metric calls, and rollout alerts.
- `internal/controlstate/types.go` - `SchedulerQualityRollup` durable evidence shape.
- `tools/scheduler_training/scheduler_training/artifacts.py` - Python artifact manifest builder and ONNX artifact publish shape.
- `tools/scheduler_training/scheduler_training/train.py` - Offline training feature preparation and model metadata output.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/scheduler/onnx.Artifact` and `Manifest` already load versioned metadata from `manifest.json`; extend this rather than adding a sidecar file.
- `internal/scheduler/onnx.Scorer` already normalizes semantic aggregates, computes confidence, and routes ONNX output through the heuristic score calculator.
- `internal/scheduler.TaskFeature` already has `ConfidenceHint`, `UncertaintyHint`, semantic aggregate fields, and `CoverageLevel`.
- `internal/scheduler.ScoreResult` already carries confidence, scheduler version, scheduler type, predicted latency, and fallback reason.
- `internal/scheduler.PredictionQualityRecorder` already writes durable rollups and low-cardinality live metrics.
- `tools/scheduler_training` already owns offline export, train, evaluate, and publish behavior for runtime artifacts.

### Established Patterns
- Scheduler is optional and disabled by default from the gateway side.
- ONNX artifacts are loaded at scheduler startup and validated before serving.
- Gateway owns queueing, execution, fallback, and rollback; Scheduler only returns scores and prediction metadata.
- Durable training and quality evidence lives in control-state storage, not Redis.
- Observability must use sanitized, low-cardinality labels.
- Public data-plane responses must not expose scheduler topology, artifact metadata, anomaly status, backend choice, or model internals.

### Integration Points
- Offline tooling computes anomaly thresholds from safe exported samples and writes manifest metadata.
- ONNX artifact loading validates anomaly metadata and records degraded state without breaking the core scoring path.
- ONNX scoring applies anomaly severity before confidence and uncertainty feed the existing score calculation.
- Quality rollup storage and migrations extend SQLite/PostgreSQL parity with anomaly counts and coverage-level grouping.
- Metrics and admin scheduler status expose anomaly status using sanitized enum labels only.

</code_context>

<specifics>
## Specific Ideas

- Use `task_type + coverage_level` for both threshold lookup and quality comparison.
- Use successful samples as the normal distribution, with failure/timeout retained as evidence rather than threshold input.
- Use `anomaly_thresholds[task_type][coverage_level]` in `manifest.json`.
- Prefer degraded ONNX-without-anomaly over failing startup for missing or malformed anomaly metadata.
- Keep scheduler fallback rate semantically clean: anomaly degraded/unavailable is a separate evidence dimension.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

*Phase: 18-Anomaly and OOD Conservative Scoring*
*Context gathered: 2026-07-04T22:39:15-07:00*
