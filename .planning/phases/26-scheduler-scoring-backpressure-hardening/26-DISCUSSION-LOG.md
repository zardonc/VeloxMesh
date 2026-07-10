# Phase 26: Scheduler Scoring Backpressure Hardening - Discussion Log

**Date:** 2026-07-08
**Source:** User-provided Phase 26 plan after scheduler risk review

## Decisions

- Confirmed risk: Scheduler scoring happens synchronously before queue push, so a slow ONNX/predictive scorer can block Gateway intake goroutines.
- Keep the synchronous intake model in this phase. Do not implement async or bypass scoring here.
- Do not add a new circuit breaker dependency.
- Keep `SCHEDULER_TIMEOUT=15ms` as the default.
- Add small backpressure controls at the external scorer boundary: max concurrency, slow-call failure accounting, and a sliding-window breaker.
- Add Plan 26-02 for scheduler quality/alert hardening after bug review.
- Bug 1 is already fixed in code; Plan 26-02 should add/keep regression coverage rather than reworking executor flow.
- Bug 2 is already fixed in code; Plan 26-02 should add retention coverage for the 100-alert cap.
- Bug 3 exists: ONNX quality alerts currently treat a single bad sample as a rate/window breach. Replace single-event alerting with an ONNX quality sample window.
- Default ONNX quality sample window size is 100. Admins must be able to change it at runtime.

## Deferred

- Async or side-channel scoring and queue reordering remain a later architecture phase.
- Distributed task recovery is out of scope for this phase.
