# Phase 14: Scheduler Queue Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-03
**Phase:** 14-Scheduler Queue Foundation
**Areas discussed:** Queue placement / client contract, fallback behavior, priority model, heuristic feature boundary

---

## Queue Placement / Client Contract

| Option | Description | Selected |
|--------|-------------|----------|
| Internal synchronous wait | Chat/API calls remain synchronous while Gateway internally queues, schedules, executes, and waits for the result. | x |
| Explicit task API | Add task intake/status/result APIs now, requiring clients to submit and poll or subscribe. | |
| Both, default sync | Wire synchronous path and expose/prep async task APIs in the same phase. | |

**User's choice:** Initially selected internal synchronous wait, then refined the decision: use one internal API to simplify development and reduce complexity.
**Notes:** Final decision is one internal task/queue/executor core. Existing data-plane API remains synchronous externally. Future public async queue APIs may expose the same core but must not create a second execution chain.

---

## Fallback Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Fail-open degradation | Scheduler failures degrade to FIFO; Redis queue failures degrade to in-memory queue; hard safety limits still reject. | x |
| Fail-closed in some configs | Some Scheduler/Redis failures return errors instead of degrading. | |
| Strict scheduler dependency | Scheduler/Redis are required for scheduled operation. | |

**User's choice:** Accepted fail-open degradation.
**Notes:** Scheduler disabled/unavailable/timeout/breaker-open falls back to FIFO. Redis queue failures fall back to single-node in-memory queue. Rejection is reserved for queue hard cap, invalid input, timeout/cancellation, or explicit strict configuration.

---

## Priority Model

| Option | Description | Selected |
|--------|-------------|----------|
| Keep three classes | Use exactly `high`, `normal`, and `low`; do not add `urgent`. | x |
| Change to four classes | Make `urgent`, `high`, `normal`, and `low` the internal model. | |
| Two-layer model | Keep workload class and add a separate boost/deadline field for urgency. | |

**User's choice:** Keep three classes only: `high`, `normal`, `low`.
**Notes:** No four-class mapping and no `urgent` class. `high` does not bypass admission, quotas, hard caps, or concurrency limits. Existing older admission terms must not confuse the Scheduler priority vocabulary.

---

## Heuristic Feature Boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Gateway extracts structured features | Gateway locally extracts safe structured features; Scheduler receives features only, no raw prompt/messages. | x |
| Scan limited user text | Inspect the first 500 chars of the last user message for classification. | |
| Never inspect payload | Use only headers/config/model/token count and avoid payload-derived features entirely. | |

**User's choice:** Gateway extracts structured features.
**Notes:** Scheduler must never receive raw prompts, messages, tool arguments, auth headers, API keys, provider secrets, or original payloads. User later provided `Gateway-Scheduler-prompt-process.md`; adopt the Gateway-local numeric/categorical summary boundary from it, but defer Qdrant neighbor enhancement and async score rewriting beyond Phase 14.
**Follow-up:** The in-scope prompt-process pieces are local summary counters, fine-grained `request_kind`, and conservative uncertainty handling. Qdrant semantic neighbors, ONNX classifiers, and training/model paths remain out of Phase 14.

---

## Agent Discretion

- Keep Phase 14 lean: queue foundation, fallback, scoring, priority safety, and core observability.
- Use the supplemental API design as reference, but trim anything that expands into Phase 15 training/history or leaks internals to clients.

## Deferred Ideas

- Public async task API.
- Durable scheduler history and training-sample pipeline.
- ONNX/model rollout and A/B prediction quality.
- Admin Console queue health UI.
