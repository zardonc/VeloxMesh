# Phase 16: A/B Rollout and Prediction Quality - Context

**Gathered:** 2026-07-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 16 lets operators compare heuristic and ONNX Scheduler scoring safely during rollout. The gateway routes scheduler calls between heuristic and ONNX backends by configuration, records prediction-quality evidence, and supports rollback without changing the OpenAI-compatible data-plane API. Scheduler remains optional and fail-open; gateway keeps queue ownership, task state, execution, and FIFO fallback behavior.

</domain>

<decisions>
## Implementation Decisions

### Rollout Traffic Shape

- **D-01:** Use a weighted canary rollout. ONNX receives a configurable percentage of scheduler traffic, heuristic remains the baseline, and rollback sets ONNX weight to zero.
- **D-02:** Assign rollout per scheduled task, not by sticky tenant/API-key assignment.
- **D-03:** If an ONNX-selected task cannot get a usable ONNX score, fall back to heuristic first, then FIFO.
- **D-04:** Operators must be able to control the ONNX rollout percentage through the admin/control config surface, not only startup env/config. Source-controlled examples must use placeholders only.

### Quality Evidence

- **D-05:** Prediction-quality comparison should use live metrics plus durable summary records.
- **D-06:** Primary prediction-quality metric is MAPE on predicted latency: `abs(predicted_ms - actual_ms) / actual_ms * 100`, averaged over completed tasks.
- **D-07:** Durable quality evidence should be aggregated rollups plus safe sample links, not a new per-task quality event stream by default.
- **D-08:** Rollups should summarize quality by scheduler type, scheduler version, and task type, with links back to existing safe scheduler training samples when deeper inspection is needed.
- **D-09:** Operators must compare wait time, scheduler call latency, scheduler errors, and prediction MAPE side by side during rollout.

### Rollback Policy

- **D-10:** Bad ONNX quality or reliability should alert operators; Phase 16 should not automatically change rollout state.
- **D-11:** Rollback alerts should be triggered by MAPE degradation or scheduler error spikes.
- **D-12:** Operational rollback means setting ONNX weight to zero while keeping heuristic and ONNX services running for diagnostics.
- **D-13:** Reuse the existing scheduler disable/FIFO path as the emergency bypass. Do not add a dedicated kill-switch field.

### Safe Comparison Dimensions

- **D-14:** Live comparison metric labels are limited to scheduler type, scheduler version, and task type.
- **D-15:** `model_class` may appear in durable rollups only, not live metric labels.
- **D-16:** Tenant/API-key identity must not be included in Phase 16 quality metric labels or durable quality records.
- **D-17:** Confidence may appear in durable rollups only, not live metric labels.

### Agent Discretion

Planner may choose exact config field names, repository table names, rollup interval, and alert threshold defaults as long as the decisions above hold, cardinality stays low, and no raw prompts, authorization headers, API keys, provider secrets, tenant/API-key identity, or original payloads are stored in quality evidence.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning Scope

- `.planning/ROADMAP.md` - Phase 16 goal, success criteria, and v7.4 scheduler boundary.
- `.planning/REQUIREMENTS.md` - OBS-02 and ML-03 requirements and out-of-scope scheduler constraints.
- `.planning/PROJECT.md` - Project-level scheduler optionality, security constraints, and current milestone decisions.
- `.planning/phases/15-training-feedback-and-onnx-path/15-CONTEXT.md` - Prior decisions for safe samples, ONNX artifact/runtime behavior, and Phase 16 handoff.
- `.planning/phases/14-scheduler-queue-foundation/14-CONTEXT.md` - Prior queue ownership, fail-open fallback, priority, feature-boundary, and FIFO decisions.

### Scheduler Interfaces

- `proto/scheduler/v1/scheduler.proto` - Existing `BatchScoreTasks`, `TaskFeature`, and `ScoreResult` wire contract.
- `internal/scheduler/types.go` - `TaskFeature`, `ScoreResult`, `Scorer`, scheduler version, confidence, and fallback result fields.
- `internal/scheduler/client.go` - Current gateway-side gRPC scorer, FIFO scorer, timeout, breaker, and fallback behavior.
- `internal/scheduler/heuristic/score.go` - Heuristic scoring path and predicted-latency output.
- `internal/scheduler/heuristic/config.go` - Heuristic scheduler version and bounded scoring config.
- `internal/scheduler/onnx/scorer.go` - ONNX scorer behavior, scheduler version, confidence, and output-token-to-latency mapping.

### Config, Metrics, and Evidence

- `internal/config/config.go` - Existing `SchedulerConfig`, env/config-file loading, scheduler mode, endpoint, feedback, and ONNX artifact fields.
- `internal/config/config_validation.go` - Scheduler config validation patterns.
- `internal/observability/metrics.go` - Existing queue wait, scheduler call, scheduler error, breaker, priority, and classification metric surface.
- `internal/scheduler/training.go` - Safe scheduler sample recording and scheduler-version labels.
- `internal/controlstate/repository.go` - Control-state repository boundary for durable scheduler evidence.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/scheduler.ScoreResult` already carries `PredictedLatencyMs`, `Confidence`, `SchedulerVersion`, and `FallbackReason`, which are the core fields needed for rollout comparison.
- `internal/scheduler.TaskFeature` already contains bounded, non-textual task dimensions, including request kind and model class.
- `internal/scheduler.GRPCScorer` and `FIFOScorer` already implement the 15ms timeout, breaker-aware fallback, and FIFO emergency path.
- `internal/scheduler/heuristic.ScoreCalculator` already produces baseline predicted latency and scheduler version.
- `internal/scheduler/onnx.Scorer` already produces ONNX scores, confidence, and scheduler version from a startup-loaded artifact.
- `internal/scheduler.TrainingRecorder` already links safe feature snapshots with completion labels and scheduler version.

### Established Patterns

- Scheduler is disabled by default and must degrade without breaking gateway startup or forwarding.
- Gateway owns queueing, execution, and fallback; Scheduler remains a stateless scoring oracle.
- Redis is hot state, not durable prediction-quality history.
- Durable backend parity matters for SQLite and PostgreSQL-backed control state.
- Observability must stay sanitized and low-cardinality; raw prompts, payloads, auth material, provider secrets, tenant/API-key identity, and high-cardinality labels are out.
- Public data-plane responses must not expose scheduler topology, rollout assignment, quality evidence, backend choice, or model internals.

### Integration Points

- Extend scheduler config/control surfaces to represent heuristic and ONNX backends plus ONNX rollout weight.
- Add gateway-side scorer selection so each scheduled task can be assigned to ONNX by weight, otherwise heuristic, with ONNX failure falling back to heuristic then FIFO.
- Record prediction-quality inputs from scheduler results and task completion labels without storing sensitive request content.
- Add low-cardinality metrics for live comparison by scheduler type, scheduler version, and task type.
- Add durable rollups for MAPE, counts, wait time, scheduler call latency, scheduler errors, `model_class`, and confidence, with safe sample links for deeper review.

</code_context>

<specifics>
## Specific Ideas

- Rollback should be operationally cheap: set ONNX weight to zero through admin/control config and keep services running for diagnostics.
- The emergency scheduler bypass should reuse existing scheduler-disable/FIFO behavior rather than introducing another kill-switch field.
- MAPE is the primary model-quality signal, but operators should see wait time, call latency, and scheduler errors beside it so rollout decisions reflect both prediction quality and runtime behavior.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

*Phase: 16-A/B Rollout and Prediction Quality*
*Context gathered: 2026-07-04*
