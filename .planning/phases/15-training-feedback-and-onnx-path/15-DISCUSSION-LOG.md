# Phase 15: Training Feedback and ONNX Path - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-04
**Phase:** 15-Training Feedback and ONNX Path
**Areas discussed:** Safe Samples, Model Artifacts, ONNX Runtime

---

## Safe Samples

| Question | Options considered | User's choice |
| --- | --- | --- |
| What should a training sample contain? | Structured feature + completion label only; add stable request hash; minimal labels only; other | Structured feature plus completion label only |
| Where should samples be stored? | SQLite/PostgreSQL control-state table; local JSONL files; Redis Stream; other | SQLite/PostgreSQL control-state table |
| When should the label be written? | On completion/failure; two-step pending/completed records; export-only reconstruction; other | On completion/failure |
| Should recording be enabled by default? | Explicit opt-in flag; enabled whenever Scheduler is enabled; always enabled with durable control state; other | Explicit opt-in flag, with admin/control config exposure |

**Notes:** Samples must never include raw prompts, raw payloads, auth material, provider secrets, original request payloads, or payload hashes. Redis stays out of training history. Feedback recording is operator controlled and ordinary data-plane invisible.

---

## Model Artifacts

| Question | Options considered | User's choice |
| --- | --- | --- |
| Where should offline tooling live? | Go CLI under `cmd/scheduler-train`; scripts under `scripts/scheduler-training`; separate Python package; other | Separate Python package |
| What must a published artifact directory contain? | Model + manifest + metrics; model + manifest only; full training bundle; other | Model + manifest + metrics |
| What is the training target? | P70 output-token predictor; P70 latency predictor; multi-output predictor; other | P70 output-token predictor |

**Notes:** Gateway/runtime remains Go-first. Python is limited to offline export/train/evaluate/publish tooling. Runtime artifacts should not include raw exported datasets or bulky training logs.

---

## ONNX Runtime

| Question | Options considered | User's choice |
| --- | --- | --- |
| What happens when an ONNX artifact is missing or invalid? | ONNX scheduler fails startup clearly; ONNX scheduler returns heuristic fallback; gateway ignores ONNX errors and uses FIFO; other | ONNX scheduler fails startup clearly |
| How should ONNX expose predicted latency? | Output-token prediction feeds existing heuristic latency formula; ONNX returns output tokens only; add predicted output-token proto field; other | Output-token prediction feeds existing heuristic latency formula |
| What should confidence mean? | Artifact/evaluation-derived confidence; fixed confidence per model version; no confidence until Phase 16; other | Artifact/evaluation-derived confidence |
| How should operators select ONNX vs heuristic in Phase 15? | Separate scheduler service selected by endpoint; single scheduler binary with backend flag; gateway-side backend selector now; other | Separate scheduler service selected by endpoint |

**Notes:** Gateway-side backend selector and A/B routing are explicitly deferred to Phase 16.

---

## Agent Discretion

- Planner may choose names for the Python package, commands, artifact directories, and concrete persistence schema.

## Deferred Ideas

- Gateway-side heuristic/ONNX backend selector and A/B routing belongs in Phase 16.
- Prediction quality comparison by scheduler type/version/task type belongs in Phase 16.
- Full latency prediction, multi-output prediction, and pending sample crash-gap records are deferred.
