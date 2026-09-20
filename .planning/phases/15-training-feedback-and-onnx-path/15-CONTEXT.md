# Phase 15: Training Feedback and ONNX Path - Context

**Gathered:** 2026-07-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 15 records safe scheduler training samples, adds offline model export/train/evaluate/publish tooling, and establishes an ONNX scheduler path that loads versioned artifacts once at startup. The Scheduler remains optional, the gateway keeps queue ownership, and the OpenAI-compatible data-plane contract stays unchanged. A/B routing and prediction-quality comparison are Phase 16 work.

</domain>

<decisions>
## Implementation Decisions

### Safe Samples

- **D-01:** Training samples contain existing safe `TaskFeature` fields plus completion labels: actual latency, token counts, outcome, provider/model class, scheduler version, and timestamps.
- **D-02:** Do not store raw prompts, raw payloads, authorization headers, API keys, provider secrets, original request payloads, or payload hashes in training samples.
- **D-03:** Store samples in the durable control-state backend: SQLite for default Plans 1/2 and PostgreSQL for Plan 4 parity. Redis stays out of training history.
- **D-04:** Write the completed training sample only after task completion or failure, using the enqueue-time feature snapshot plus actual labels. Do not add pending sample state unless a later phase proves crash-gap visibility is needed.
- **D-05:** Feedback recording is explicit opt-in and separate from Scheduler enablement.
- **D-06:** Expose feedback recording config through existing admin/control config surfaces so operators can enable or disable recording without a code deploy. Keep this config secret-safe and invisible to ordinary data-plane responses.

### Model Artifacts

- **D-07:** Use a separate Python package for offline ML tooling. Keep the gateway runtime Go-first; Python is only for export, train, evaluate, and publish tooling.
- **D-08:** Published artifacts contain `model.onnx`, `manifest.json`, feature schema/version, training data window, evaluation metrics, ONNX parity check result, and checksum.
- **D-09:** Do not publish raw exported datasets or bulky training logs inside the runtime artifact directory.
- **D-10:** The first model target is the Phase 15 P70 output-token predictor. Full latency prediction and multi-output prediction stay out of Phase 15.
- **D-11:** Latency prediction can be derived downstream from output-token estimates plus existing scheduler heuristics.

### ONNX Runtime

- **D-12:** If configured for ONNX, missing or invalid artifacts make the ONNX scheduler fail startup clearly.
- **D-13:** Heuristic remains a separate runnable backend. Gateway FIFO fallback still handles scheduler service failure from the gateway side.
- **D-14:** ONNX predicts P70 output tokens, then the scheduler maps that prediction through the existing model/request heuristic path to fill `predicted_latency_ms`.
- **D-15:** Avoid proto churn unless implementation proves output-token metadata must be exposed.
- **D-16:** ONNX confidence is derived from artifact/evaluation quality plus request-time feature validation and feature coverage. Low coverage or invalid/missing feature values lower confidence.
- **D-17:** In Phase 15, operators select heuristic versus ONNX by running distinct scheduler services and pointing `SCHEDULER_ENDPOINT` at the desired service.
- **D-18:** Gateway-side heuristic/ONNX backend selection and A/B routing move to Phase 16.

### Agent Discretion

Planner may choose the Python package name, command names, artifact directory naming convention, and exact control-state table schema as long as the safety, parity, and artifact-contract decisions above hold.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning Scope

- `.planning/ROADMAP.md` - Phase 15 goal, success criteria, requirements, and Phase 16 boundary.
- `.planning/REQUIREMENTS.md` - FEED-01, ML-01, ML-02, OBS-02, ML-03 traceability and out-of-scope scheduler requirements.
- `.planning/PROJECT.md` - Scheduler optionality, security constraints, and current milestone decisions.
- `.planning/phases/14-scheduler-queue-foundation/14-CONTEXT.md` - Prior scheduler queue, safe feature, stateless scheduler, and fallback decisions that Phase 15 builds on.

### Existing Scheduler Interfaces

- `proto/scheduler/v1/scheduler.proto` - Existing `BatchScoreTasks`, `TaskFeature`, and `ScoreResult` contract.
- `internal/scheduler/types.go` - Current `TaskFeature`, `ScoreResult`, `Scorer`, priority, and request-kind types.
- `internal/scheduler/features.go` - Existing safe feature extraction boundary.
- `internal/scheduler/client.go` - FIFO fallback and gRPC scorer behavior.
- `internal/scheduler/heuristic/score.go` - Existing heuristic scoring path that ONNX output-token predictions should feed.
- `internal/scheduler/heuristic/server.go` - Current scheduler service implementation shape and `ScoreResult` response mapping.
- `internal/config/config.go` - Existing scheduler config surface and env/config-file loading pattern.
- `internal/config/config_validation.go` - Existing scheduler config validation pattern.
- `cmd/scheduler/main.go` - Current heuristic scheduler binary wiring.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/scheduler.TaskFeature`: already contains bounded, non-textual scheduler features suitable for durable sample input.
- `internal/scheduler.ScoreResult`: already exposes `predicted_latency_ms`, `confidence`, and `scheduler_version`; ONNX can reuse this contract.
- `internal/scheduler.ExtractSafeFeatures`: enqueue-time feature snapshot source for training samples.
- `internal/scheduler.GRPCScorer` and `FIFOScorer`: preserve gateway fallback behavior when scheduler services are unavailable.
- `internal/scheduler/heuristic.ScoreCalculator`: existing latency/score formula that ONNX output-token predictions can feed.
- `internal/config.SchedulerConfig`: existing place to add feedback and ONNX runtime config without inventing a parallel config system.

### Established Patterns

- Scheduler is disabled by default and fail-open from the gateway side.
- Redis is hot state, not durable training history.
- Durable backend parity matters: SQLite for default Plans 1/2, PostgreSQL for Plan 4.
- Public data-plane responses must not expose scheduler topology, backend choice, model internals, or training-recording state.
- Low-cardinality, sanitized logs and metrics are allowed; raw prompts, provider secrets, and task payloads are not.

### Integration Points

- Capture feature snapshots at enqueue and complete samples at task completion/failure in the gateway-owned queue/task flow.
- Add sample persistence behind the control-state repository boundary with SQLite/PostgreSQL implementations.
- Add admin/control config support for opt-in feedback recording.
- Add Python offline tooling that exports safe completed samples from control state, trains/evaluates the P70 output-token predictor, validates ONNX parity, and publishes versioned artifacts.
- Add an ONNX scheduler service mode/binary that loads one artifact directory at startup and serves through `BatchScoreTasks`.

</code_context>

<specifics>
## Specific Ideas

- The runtime artifact directory is intentionally small: loadable model, manifest, schema/version, metrics, parity result, and checksum.
- Raw exported datasets and training logs can exist in offline tooling output, but not in the runtime artifact directory consumed by the scheduler.
- ONNX service startup failure is preferred for invalid configured artifacts because it makes model/config mistakes explicit.
- Phase 15 selects ONNX versus heuristic by endpoint. Phase 16 adds gateway-side backend routing and A/B controls.

</specifics>

<deferred>
## Deferred Ideas

- Gateway-side heuristic/ONNX backend selector and A/B routing belongs in Phase 16.
- Prediction quality comparison by scheduler type/version/task type belongs in Phase 16.
- Full latency prediction and multi-output prediction are future model upgrades after the P70 output-token path works.
- Pending training sample records for crash-gap visibility are deferred unless operational need appears.

</deferred>

---

*Phase: 15-Training Feedback and ONNX Path*
*Context gathered: 2026-07-04*
