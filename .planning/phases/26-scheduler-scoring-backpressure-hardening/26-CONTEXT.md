# Phase 26: Scheduler Scoring Backpressure Hardening - Context

**Gathered:** 2026-07-08
**Status:** Ready for planning
**Source:** User-provided Phase 26 plan

<domain>
## Phase Boundary

Harden the synchronous Scheduler scoring path so slow or unhealthy external scorer calls degrade quickly instead of piling up Gateway intake goroutines.

This phase keeps the existing `TaskIntake.Submit -> Scorer.Score -> Queue.Push` shape. It does not change queue ordering semantics or introduce async scoring.
</domain>

<decisions>
## Implementation Decisions

### Locked
- `scheduler.timeout` remains `15ms` by default.
- `scheduler.scorer_max_concurrency` / `SCHEDULER_SCORER_MAX_CONCURRENCY` caps external scoring calls.
- `scheduler.scorer_slow_threshold` / `SCHEDULER_SCORER_SLOW_THRESHOLD` treats slow successful calls as degraded.
- `GRPCScorer` and `PythonONNXPredictorClient` both need quick-fail behavior.
- Breaker logic should use a small sliding window; a single success must not clear recent failures.
- No `gobreaker` or new dependency.
- Bug 1 executor `ErrQueueEmpty`/`IsRunning` race is considered fixed; keep it covered with regression tests.
- Bug 2 rollout alert retention is considered fixed; add a regression test that `RecordAlert` keeps only the most recent 100 alerts.
- Bug 3 quality alerting must be changed from single-sample alerting to window statistics.
- ONNX quality sample window size defaults to 100 and must be runtime-admin configurable.
- Avoid adding multiple knobs unless needed. Derive a small minimum sample threshold from the window instead of adding another admin setting.

### Out of Scope
- Async/side-channel scoring.
- Queue reordering based on scores that arrive after enqueue.
- Predictor accuracy changes.
- Distributed task recovery.
</decisions>

<canonical_refs>
## Canonical References

- `internal/scheduler/intake.go` - synchronous score-before-push intake flow.
- `internal/scheduler/client.go` - gateway-side gRPC Scheduler scorer.
- `internal/scheduler/predictor/python_client.go` - scheduler-side Python ONNX predictor client.
- `internal/scheduler/predictor/breaker.go` - predictor client breaker.
- `internal/config/config_types.go` - scheduler config surface.
- `cmd/scheduler/main.go` - scheduler process predictor wiring.
- `docs/scheduler-1.0-runbook.md` - operator constraints.
- `internal/scheduler/quality.go` - ONNX quality alert logic.
- `internal/scheduler/quality_test.go` - quality recorder and rollup tests.
- `internal/scheduler/rollout_control.go` - rollout status, alert retention, and runtime alert config.
- `internal/scheduler/rollout_control_test.go` - rollout status and alert tests.
- `internal/scheduler/admin_scheduler_service.go` - admin runtime update surface.
- `internal/http/handlers/admin_scheduler.go` - admin scheduler PATCH handler.
- `internal/http/handlers/admin_scheduler_test.go` - admin scheduler API tests.
</canonical_refs>

<specifics>
## Specific Ideas

- Use non-blocking semaphore acquisition. If no slot is available, return fallback immediately.
- Record slow successful responses as breaker failures and return fallback.
- Keep slow threshold default at or below `scheduler.timeout`.
- Document 50-100ms only as an upper bound for unusual deployments, not a default.
- Implement quality window statistics with a tiny in-memory ring buffer in `PredictionQualityRecorder`.
- Alert only when enough ONNX samples are available and the aggregate error rate or average MAPE crosses the configured threshold.
- Expose `quality_sample_window` in rollout status and PATCH update body.
</specifics>

<deferred>
## Deferred Ideas

- A future architecture phase can enqueue with heuristic score first and refine scores asynchronously.
- Persistent/durable quality alert windows across process restarts are out of scope.
</deferred>
