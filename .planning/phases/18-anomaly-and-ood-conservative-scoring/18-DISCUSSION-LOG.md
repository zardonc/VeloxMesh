# Phase 18: Anomaly and OOD Conservative Scoring - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-04T22:39:15-07:00
**Phase:** 18-Anomaly and OOD Conservative Scoring
**Areas discussed:** OOD threshold scope, Artifact missing or invalid behavior, Conservative scoring strength, Quality evidence dimensions

---

## OOD Threshold Scope

| Question | Options | Selected |
|----------|---------|----------|
| Threshold granularity | Task type; Task + model; Add coverage | Add coverage |
| Coverage use | Task type + coverage level; Task type + coverage ratio bucket; Both level and ratio | Task type + coverage level |
| Sample source | Successful samples only; All completed outcomes; Success primary, failures tracked separately | Success primary, failures tracked separately |
| Sparse samples | Fallback to task type only; Use global threshold; Mark anomaly unavailable | Fallback to task type only |
| Manifest shape | Nested by task type + coverage; Flat rows; Both plus summary | Nested by task type + coverage |
| Threshold fields | Threshold + sample count; Add mean/stddev; Add window + method | Add mean/stddev |
| Threshold algorithm | Mean + K stddev; Percentile threshold; Hybrid | Hybrid |

**Notes:** Thresholds use successful samples as the normal distribution. Failure and timeout outcomes stay as comparison evidence, not threshold input. Sparse scoped thresholds fall back to task type, then anomaly unavailable.

---

## Artifact Missing Or Invalid Behavior

| Question | Options | Selected |
|----------|---------|----------|
| Missing metadata | Degraded ONNX without anomaly; Fail startup; Gateway fallback | Degraded ONNX without anomaly |
| Invalid metadata | Degrade anomaly only; Fail startup; Reject bad groups only | Degrade anomaly only |
| Operator visibility | Metrics + logs only; Add admin status field; Both | Both |
| Reason granularity | Small enum; Freeform error string; Enum + detail in logs | Enum + detail in logs |

**Notes:** Missing or invalid anomaly metadata must not make the core ONNX scoring path unavailable. Degraded state is visible, but bounded to safe enum values outside logs.

---

## Conservative Scoring Strength

| Question | Options | Selected |
|----------|---------|----------|
| Conservative behavior | Lower confidence + raise uncertainty; Adjust virtual deadline score directly; Both | Lower confidence + raise uncertainty |
| Confidence reduction | Severity-scaled clamp; Fixed confidence floor; Artifact-configured curve | Severity-scaled clamp |
| Score penalty path | Use existing uncertainty penalty path; Separate ONNX penalty; Confidence only | Use existing uncertainty penalty path |
| Severity source | Distance over threshold; Binary anomaly flag; Multi-signal severity | Distance over threshold |

**Notes:** OOD behavior should reuse existing confidence and uncertainty semantics instead of changing the Scheduler RPC contract or creating a parallel scoring formula.

---

## Quality Evidence Dimensions

| Question | Options | Selected |
|----------|---------|----------|
| Rollup dimensions | Scheduler version + task type + coverage level; Add model class; Scheduler version + task type only | Scheduler version + task type + coverage level |
| Rollup fields | Counts + rate; Add severity avg; Add confidence/uncertainty deltas | Counts + rate |
| Live metric labels | Yes, coverage level only; No, rollup only; Coverage + anomaly status | Coverage + anomaly status |
| Fallback counting | Count anomaly unavailable/degraded separately; Treat degraded as fallback; Only logs/admin status | Count anomaly unavailable/degraded separately |

**Notes:** Anomaly unavailable/degraded is evidence about metadata or threshold availability, not a scheduler backend fallback.

## Agent Discretion

- Exact enum names.
- Minimum sample defaults.
- `k` and percentile defaults for threshold computation.
- Admin status field names.
- Internal helper names and placement.

## Deferred Ideas

None.

---

## Corrective Replanning: ONNX Runtime Boundary

**Date:** 2026-07-05T10:58:02-07:00

| Area | Decision | Notes |
|------|----------|-------|
| Predictor contract | Use quantile-aware `OutputTokenPredictor.Predict` returning `Prediction{Quantiles, ModelVersion, Signals, Err}`. | `PredictP70OutputTokens` leaks Scheduler policy into Predictor and is superseded. |
| Signal boundary | Predictor computes model-native signals; Scheduler owns policy. | Quantile spread, OOD distance, and feature coverage are evidence, not scheduling decisions. |
| Runtime architecture | Use a long-lived Python worker with `onnxruntime.InferenceSession`; keep default Go build free of ONNX Runtime CGO. | Matches the existing portability stance used for LanceDB isolation. |
| Manifest gate | Add concrete `predictor-v1` manifest fields and validate feature schema before prediction. | Schema drift must fail fast before a Python tensor shape error. |
| Lifecycle and fallback | Add startup health, timeout, breaker, restart backoff, recovery probe, and `NoopPredictor`. | Scheduler must keep serving through heuristic/noop degradation. |
| Partial failure | Return per-task prediction errors without failing sibling tasks in the batch. | Batch-level errors are reserved for systemic/transport failures. |
| Rollout | Put champion/challenger and shadow behavior in `PredictorRouter`. | Predictor contract remains unchanged. |
| Acceptance | Phase 18 is accepted only when ONNX is actually callable through the Python worker and Scheduler smoke test. | The current Go constant ONNX parser is not sufficient runtime evidence. |

**Notes:** User explicitly rejected a temporary Phase 18 patch. `18-CONTEXT.md` and `18-04-PLAN.md` now define the final corrective plan for ONNX invocation and Scheduler/Predictor separation.
