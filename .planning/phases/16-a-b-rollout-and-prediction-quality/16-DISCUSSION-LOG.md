# Phase 16: A/B Rollout and Prediction Quality - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md; this log preserves the alternatives considered.

**Date:** 2026-07-04
**Phase:** 16-A/B Rollout and Prediction Quality
**Areas discussed:** Rollout traffic shape, Quality evidence, Rollback policy, Safe comparison dimensions

---

## Rollout Traffic Shape

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| How should Phase 16 route scheduler calls between heuristic and ONNX? | Weighted canary; Single active backend; Shadow compare | Weighted canary |
| For the canary split, what should be the assignment unit? | Per task; Stable per tenant/API key; Explicit allowlist first | Per task |
| When an ONNX-selected task can't get a usable ONNX score, what should happen? | Fall back to heuristic, then FIFO; Fall straight to FIFO; Fail if strict mode is enabled | Fall back to heuristic, then FIFO |
| For rollout configuration, where should operators control the ONNX percentage? | Existing config/env only; Admin/control config surface too; Config file only | Admin/control config surface too |

**Notes:** ONNX weight should be configurable and rollback should set the ONNX weight to zero. Heuristic remains the baseline.

---

## Quality Evidence

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| Where should prediction-quality comparison live? | Metrics plus durable summary records; Metrics only; Durable records only | Metrics plus durable summary records |
| For prediction error, what should be the primary comparison metric? | MAPE on predicted latency; Absolute error buckets; P70 token prediction error | MAPE on predicted latency |
| For durable quality evidence, how much detail should Phase 16 store? | Aggregated rollups plus safe sample links; Per-task quality records; Metrics-derived rollups only | Aggregated rollups plus safe sample links |
| Besides MAPE, which operational signals must be compared side by side? | Wait time, scheduler call latency, and scheduler errors; Add confidence distribution too; Add queue depth impact too | Wait time, scheduler call latency, and scheduler errors |

**Notes:** MAPE was clarified as mean absolute percentage error on predicted versus actual latency.

---

## Rollback Policy

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| What should happen when ONNX quality or reliability looks bad? | Alert and let operators roll back; Automatic rollback to heuristic; Automatic rollback only in strict/canary mode | Alert and let operators roll back |
| Which rollback signals should trigger the operator alert? | MAPE degradation or scheduler error spike; Add wait-time regression too; Any ONNX fallback event | MAPE degradation or scheduler error spike |
| What should rollback mean operationally? | Set ONNX weight to zero, keep services running; Switch scheduler mode back to heuristic; Disable scheduler entirely to FIFO | Set ONNX weight to zero, keep services running |
| Should Phase 16 include an emergency FIFO kill switch too? | Yes, keep existing scheduler disable/FIFO path; No, heuristic is enough; Add a dedicated kill-switch field | Keep existing scheduler disable/FIFO path |

**Notes:** The emergency FIFO path was clarified as bypassing all scheduler scoring and serving queued tasks in first-in-first-out order.

---

## Safe Comparison Dimensions

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| Which labels are allowed for quality comparison? | Scheduler type, scheduler version, and task type; Add model class and priority too; Add tenant/API key too | Scheduler type, scheduler version, and task type |
| Should `model_class` be included anywhere? | Durable rollups only, not live metric labels; Include in both metrics and durable rollups; Exclude entirely | Durable rollups only, not live metric labels |
| Should tenant/API-key identity ever be part of Phase 16 prediction-quality comparison? | No tenant/API-key labels or records; Durable admin-only tenant rollups; Hash API key for durable analysis | No tenant/API-key labels or records |
| Should confidence be exposed in comparison data? | Durable rollups only; Metrics and durable rollups; Exclude confidence | Durable rollups only |

**Notes:** Live metrics should stay low-cardinality. Durable rollups may carry bounded diagnostic dimensions that are not suitable as live metric labels.

---

## Agent Discretion

- Planner may choose exact config field names, repository table names, rollup interval, and alert threshold defaults within the captured safety and cardinality constraints.

## Deferred Ideas

- None.
