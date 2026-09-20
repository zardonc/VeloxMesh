# Phase 17: Semantic Neighbor Feature Aggregates - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-04
**Phase:** 17-Semantic Neighbor Feature Aggregates
**Areas discussed:** Neighbor Source, Aggregate Shape, Timeout Behavior, Scoring Use

---

## Neighbor Source

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| Which source should semantic neighbors use? | Training samples only; training samples + semantic cache metadata; successful completed tasks only; other | Training samples only |
| Which outcomes should neighbor samples include? | All completed samples; success only; success + timeout only; other | All completed samples |
| What isolation grain should neighbors use? | Tenant + model_class + request_kind; model_class + request_kind only; tenant + model_class only; other | Tenant + model_class + request_kind, fallback to model_class + request_kind when tenant scope lacks enough samples |
| How should insufficient samples be handled? | Min count with fallback; always emit partial stats; strict tenant-only; other | Min count with fallback; default minimum count is 20 and admin/config can change it |

**Notes:** The neighbor source is deliberately limited to completed scheduler training samples to keep Phase 17 safe and bounded.

---

## Aggregate Shape

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| Which aggregate fields are required in v17? | Core prediction stats; minimal only; rich stats; other | Core prediction stats |
| How should coverage_level be represented? | Enum; numeric ratio; both enum + ratio; other | Both enum + numeric ratio |
| Where should semantic aggregate fields live in schema? | First-class TaskFeature fields; metadata map; training-only first; other | First-class TaskFeature fields |
| Should runtime scorers immediately consume semantic aggregates? | ONNX/training consumes, heuristic observes; ONNX + heuristic both consume; training-only; other | ONNX/training consumes; heuristic observes and does not alter score |

**Notes:** Core prediction stats are `neighbor_count`, latency percentiles/stddev, `output_tokens_p70`, outcome rates, and coverage fields.

---

## Timeout Behavior

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| Semantic enrichment timeout budget? | Per-task 5ms, batch hard cap 15ms; per-task 10ms, batch 25ms; async/background only; other | Per-task 5ms, batch hard cap 15ms; enrichment fails open |
| Semantic enrichment failure handling? | Metrics + safe defaults, no caller-visible error; scheduler metadata warning; short feature circuit breaker; other | Record metrics and use safe defaults; do not expose enrichment errors to scheduler callers |
| Where should semantic enrichment run? | Gateway after safe-feature extraction and before scheduler RPC; Scheduler service internal lookup; training/export path only; other | Gateway after safe feature extraction and before scheduler RPC |
| Semantic aggregate configuration defaults? | Disabled by default with admin-configurable enable/min_count/timeouts; shadow/training on and runtime off; enable whenever vector store is configured; other | Disabled by default; administrators can configure enablement, min_count=20, and 5ms/15ms timeout budgets |

**Notes:** The selected behavior keeps scheduler calls bounded and fail-open while preserving the Gateway-owned semantic lookup boundary.

---

## Scoring Use

| Question | Options Considered | User's Choice |
| --- | --- | --- |
| Scoring use compatibility policy? | ONNX/training may consume with neutral/default values for unsupported artifacts and no heuristic/FIFO score change; require retrain/new ONNX before runtime enablement; ONNX + heuristic both immediately adjust score; other | ONNX and training may consume semantic aggregates; unsupported artifacts use neutral/default values; heuristic and FIFO fallback do not change scores |

**Notes:** Phase 17 may add fields and training signal without changing the v7.4 heuristic or FIFO baseline.

## Agent Discretion

- Exact implementation names, helper boundaries, and config layout are left to the planner, subject to the locked decisions in `17-CONTEXT.md`.

## Deferred Ideas

- Semantic-cache metadata as a neighbor source.
- Heuristic score adjustment using semantic aggregates.
- Phase 18 anomaly/OOD conservative scoring.
- Phase 19 SLA waiting-time promotion.
