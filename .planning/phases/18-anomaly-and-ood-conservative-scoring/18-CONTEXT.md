# Phase 18: Anomaly and OOD Conservative Scoring - Context

**Gathered:** 2026-07-05T10:58:02-07:00
**Status:** Corrective replanning required

<domain>
## Phase Boundary

Phase 18 must be corrected so the scheduler uses a real model predictor without coupling Scheduler policy to ONNX implementation details.

Gateway does not touch models. Scheduler does not touch ONNX runtime. Predictor does not make scheduling policy decisions. Predictor computes model outputs and model-native signals; Scheduler decides how to convert those outputs and signals into score, confidence, uncertainty, fallback, rollout, and quality evidence.

The current Go `internal/scheduler/onnx` scorer path is not the final architecture: it parses a constant ONNX graph in Go, hard-codes P70 output tokens into the model interface, mixes OOD signal calculation with Scheduler policy, and drops semantic aggregate fields in the ONNX gRPC server mapping. Corrective implementation must replace that shape with a final predictor boundary, not a temporary patch.

</domain>

<decisions>
## Implementation Decisions

### Predictor Contract
- **D-01:** Replace P70-specific predictor contracts with a quantile-aware predictor contract:

  ```go
  type Prediction struct {
      Quantiles    map[int]float64
      ModelVersion string
      Signals      map[string]float64
      Err          error
  }

  type OutputTokenPredictor interface {
      Predict(ctx context.Context, tasks []scheduler.TaskFeature) ([]Prediction, error)
  }
  ```

- **D-02:** `Predict` returns one `Prediction` per input task, index aligned with the request. Batch-level `error` is only for transport, worker, manifest, or systemic failures. Per-task malformed input uses `Prediction.Err` so one bad task does not fail the whole batch.
- **D-03:** Predictor output is descriptive, not prescriptive. It may return quantiles such as P50, P70, and P90, plus signals such as `quantile_spread`, `ood_distance`, `feature_coverage`, or `schema_mismatch_distance`. It must not choose Scheduler score, fallback policy, rollout policy, or conservative thresholds.
- **D-04:** The Scheduler scoring layer chooses which quantile to consume and how to translate spread/OOD signals into confidence, uncertainty, and virtual-deadline score. P70 is a Scheduler policy default, not a Predictor API name.

### Component Boundaries And Naming
- **D-05:** Scheduler-level components must be implementation-neutral. Use names such as `PredictiveScorer`, `MLAugmentedScorer`, `OutputTokenPredictor`, and `PredictorRouter`; do not expose `ONNXScorer` above the predictor implementation package.
- **D-06:** ONNX-specific names may exist only in the predictor implementation and Python worker code, for example `PythonONNXPredictorClient` or worker modules under scheduler training tooling.
- **D-07:** `NoopPredictor` is a first-class implementation for P3 SQLite-only/lightweight deployments and for degraded mode. It is not just a test double.

### Signal And Policy Boundary
- **D-08:** Model-native signal calculation belongs in Predictor because only the model artifact and training metadata know quantile spread, feature distances, and training-distribution statistics.
- **D-09:** Policy decisions belong in Scheduler: threshold interpretation, conservative coefficient, fallback choice, breaker behavior, score inflation, confidence clamp, and rollout routing.
- **D-10:** Missing or degraded model signals must degrade predictive scoring only. Gateway forwarding, Scheduler service startup, and heuristic scoring remain available.

### Runtime ONNX Invocation
- **D-11:** Default runtime ONNX execution uses a long-lived Python worker with `onnxruntime.InferenceSession`. Do not bind ONNX Runtime into the default Go build through CGO.
- **D-12:** The Go Scheduler talks to the worker over a local transport. Prefer gRPC over Unix Domain Socket where supported; keep the endpoint configurable so Windows development can use a safe local transport without introducing CGO.
- **D-13:** The Python worker starts once, loads `manifest.json`, creates one `InferenceSession`, exposes health/readiness, and serves batch predictions. It is not started per request.
- **D-14:** Scheduler startup probes the predictor. If health or manifest validation fails, `PredictiveScorer` must use `NoopPredictor`/heuristic scoring and keep serving.
- **D-15:** Predictor calls have a timeout, a small circuit breaker, restart with backoff after worker crash, and periodic recovery probes after the breaker opens.

### Manifest Contract
- **D-16:** Runtime artifacts must publish a concrete predictor manifest. At minimum:

  ```json
  {
    "protocol_version": "predictor-v1",
    "model_version": "v1.3.0",
    "task_type": "quantile_regression",
    "quantiles": [50, 70, 90],
    "feature_schema": [
      {"name": "estimated_input_tokens", "type": "float32"},
      {"name": "latency_p50_ms", "type": "float32"},
      {"name": "coverage_level", "type": "enum"}
    ],
    "training_data_hash": "...",
    "compatible_scheduler_version": ">=0.9.0"
  }
  ```

- **D-17:** Scheduler must validate the manifest against its `TaskFeature -> tensor` mapping before the first prediction call. Schema drift must fail fast into degraded predictive scoring, not surface later as a Python tensor shape error.
- **D-18:** The manifest may retain Phase 18 anomaly metadata, but anomaly metadata cannot be the only contract. Quantiles, feature schema, protocol version, model version, and compatibility must be explicit.

### Rollout And Routing
- **D-19:** Canary and shadow behavior belongs in a `PredictorRouter` between `PredictiveScorer` and `OutputTokenPredictor`. The Predictor contract stays unchanged.
- **D-20:** `PredictorRouter` may hold champion and challenger predictors, route by weight, or run shadow predictions for logging only. Scheduler policy adopts only the selected predictor output.

### Corrective Acceptance
- **D-21:** Phase 18 is not accepted until ONNX is actually callable through `onnxruntime.InferenceSession` in the Python worker and the Scheduler receives real quantile/signal predictions through the predictor client.
- **D-22:** The current Go constant-graph parser is not sufficient acceptance evidence. It may be deleted or retained only as an offline test helper, not as the production runtime predictor.
- **D-23:** The Scheduler service mapping must preserve every safe `TaskFeature` field already present in `proto/scheduler/v1/scheduler.proto`, including semantic aggregate fields from Phase 17.
- **D-24:** Verification must include a smoke/integration path that starts the worker, runs a small ONNX artifact through `InferenceSession`, calls Scheduler `BatchScoreTasks`, and proves the returned score used predictor quantiles and model signals without falling back.

### Agent Discretion
- Planner may choose exact package names and config field names as long as the boundaries above hold, Go remains pure by default, tests prove real ONNX Runtime invocation, and existing heuristic/fallback/control-state patterns are reused.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 18 goal and existing success criteria to revise with corrective acceptance.
- `.planning/REQUIREMENTS.md` - ANOM-01 through ANOM-04 and out-of-scope scheduler boundaries.
- `.planning/PROJECT.md` - Scheduler optionality, pure-Go default runtime philosophy, and gateway/scheduler boundaries.
- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-01-PLAN.md` - Existing artifact-threshold plan that produced the current manifest shape.
- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-02-PLAN.md` - Existing runtime plan that mixed ONNX runtime details and Scheduler policy.
- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-03-PLAN.md` - Existing quality evidence plan to preserve after the predictor boundary changes.
- `.planning/phases/17-semantic-neighbor-feature-aggregates/17-CONTEXT.md` - Semantic aggregate fields that must survive scheduler service mapping.
- `.planning/phases/16-a-b-rollout-and-prediction-quality/16-CONTEXT.md` - Rollout and quality comparison baseline.
- `.planning/phases/15-training-feedback-and-onnx-path/15-CONTEXT.md` - Original ONNX path and artifact decisions now superseded where they hard-code P70 or Go-side model execution.

### Current Source
- `proto/scheduler/v1/scheduler.proto` - Existing Gateway -> Scheduler RPC contract and safe `TaskFeature` field list.
- `internal/scheduler/types.go` - `TaskFeature`, `ScoreResult`, scheduler type constants, and current anomaly status fields.
- `internal/scheduler/onnx/model.go` - Current constant ONNX graph parser; not acceptable as final runtime invocation.
- `internal/scheduler/onnx/scorer.go` - Current mixed ONNX scorer/policy implementation to replace with `PredictiveScorer`.
- `internal/scheduler/onnx/server.go` - Current ONNX service mapping that drops semantic aggregate fields.
- `internal/scheduler/heuristic/score.go` - Existing virtual-deadline and uncertainty penalty path to reuse.
- `internal/scheduler/client.go` - Gateway-side timeout, breaker, FIFO fallback, and weighted scoring patterns.
- `cmd/scheduler/main.go` - Scheduler service mode wiring, health/status endpoints, and current ONNX startup behavior.

### Offline And Worker Tooling
- `tools/scheduler_training/pyproject.toml` - Python dependency surface; currently lacks `onnxruntime` and worker dependencies.
- `tools/scheduler_training/scheduler_training/artifacts.py` - Current manifest builder and constant ONNX publisher.
- `tools/scheduler_training/scheduler_training/train.py` - Current quantile/anomaly training logic.
- `tools/scheduler_training/scheduler_training/publish.py` - Runtime artifact publication path.
- `tools/scheduler_training/README.md` - Artifact contents and command documentation to update.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/scheduler/heuristic.ScoreCalculator` already converts output-token estimates, confidence, and uncertainty into virtual-deadline scores.
- `internal/scheduler.GRPCScorer`, breaker logic, and FIFO fallback already provide the fail-open pattern to reuse for predictor calls.
- `proto/scheduler/v1/scheduler.proto` already contains the safe scalar and enum features needed by the predictor path.
- `tools/scheduler_training` already owns artifact publish and manifest generation; extend it instead of adding a second artifact format.

### Problems To Correct
- `internal/scheduler/onnx/model.go` only reads constant ONNX output bytes in Go; it does not call ONNX Runtime.
- `internal/scheduler/onnx/scorer.go` combines model execution, OOD signal calculation, Scheduler scoring policy, metrics, and ONNX-specific naming.
- `internal/scheduler/onnx/server.go` does not map Phase 17 semantic aggregate proto fields back into `TaskFeature`.
- `tools/scheduler_training/pyproject.toml` depends on `onnx` but not `onnxruntime`, `grpcio`, or worker runtime dependencies.
- Existing tests prove the constant parser path, but do not prove an actual `InferenceSession` call through Scheduler.

### Established Patterns
- Scheduler is optional, disabled by default from Gateway, and must fail open.
- Gateway owns queueing, execution, fallback, and any raw prompt or vector lookup.
- Scheduler receives only safe scalar/enum features and returns scores.
- Observability uses sanitized, low-cardinality labels only.
- SQLite/PostgreSQL durable parity matters for quality evidence.

### Integration Points
- Add predictor interfaces under Scheduler internals without changing Gateway data-plane API.
- Add Python worker artifact loading and health under existing scheduler training/tooling ownership.
- Update Scheduler service mode wiring so `predictive`/ML mode composes `PredictiveScorer + PredictorRouter + OutputTokenPredictor`.
- Preserve existing quality rollup fields while sourcing anomaly status from Scheduler policy over predictor signals.

</code_context>

<specifics>
## Specific Ideas

- Keep Python worker as the default because Go ONNX Runtime bindings require CGO and native shared libraries, the same portability class previously isolated for LanceDB.
- A future low-latency tier may add `//go:build onnxruntime_cgo`, but that is optional and not the default path.
- `NoopPredictor` directly matches lightweight P3 deployments where running Python is not worth the operational cost.
- `PredictorRouter` is the right place for champion/challenger and shadow predictions; do not put rollout fields into the Predictor contract.

</specifics>

<deferred>
## Deferred Ideas

- Optional Go ONNX Runtime CGO implementation behind `//go:build onnxruntime_cgo`.
- Automated rollout decisions based on quality signals remain future `AUTO-01` work.

</deferred>

---

*Phase: 18-Anomaly and OOD Conservative Scoring*
*Context gathered: 2026-07-05T10:58:02-07:00*
