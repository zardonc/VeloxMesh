# Phase 26: Scheduler Scoring Backpressure Hardening - Research

## Finding

The risk is real because Scheduler scoring runs before enqueue. The default timeout is already short (`15ms`), so the worst-case "1.9s default slow query" example is not true for default config. The pile-up risk still exists when operators increase timeout or the predictor oscillates between slow success and failure.

## Existing Patterns

- Gateway scorer fallback already returns FIFO scores with a `FallbackReason`.
- Predictive scorer fallback already uses `NoopPredictor` per-task errors.
- Config already uses env + JSON fields under `SchedulerConfig`.
- Tests already use real local TCP gRPC servers for scheduler and predictor coverage.

## Chosen Approach

- Add local, non-blocking max concurrency at external scorer boundaries.
- Treat slow successful calls as degraded and route to existing fallback.
- Replace consecutive-failure-only breaker behavior with a tiny sliding-window failure-rate check.
- Reuse existing fallback paths and tests; add no dependency.

## Rejected

- `gobreaker`: more dependency surface than this phase needs.
- Async scoring: changes scheduling semantics and should be planned separately.
