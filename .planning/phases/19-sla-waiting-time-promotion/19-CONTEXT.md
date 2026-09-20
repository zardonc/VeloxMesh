# Phase 19: SLA Waiting-Time Promotion - Context

**Gathered:** 2026-07-05T13:43:48-07:00
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 19 adds gateway-owned SLA waiting-time promotion for queued scheduler tasks. Promotion is a bounded, threshold-triggered exception to the earlier no-dynamic-re-ranking rule: no continuous queue rescoring, no prompt-derived urgency, and no scheduler-owned queue behavior.

Gateway may update an eligible queued task's existing queue score after it exceeds a configured SLA threshold, but only within trusted priority and quota boundaries. Scheduler remains a stateless scoring service.

</domain>

<decisions>
## Implementation Decisions

### Policy Thresholds
- **D-01:** SLA rules are keyed by `tenant + model_class + request_kind`, using only trusted identity/config and safe structured dimensions.
- **D-02:** If no matching SLA rule exists, the task receives no SLA promotion.
- **D-03:** Invalid SLA rules fail config validation when promotion is enabled. Disabled promotion remains harmless.

### Queue Promotion Mechanics
- **D-04:** Promotion updates the existing queue score for a queued task. Reuse Redis `ZAdd` score replacement and memory queue duplicate `Push`/`heap.Fix` behavior instead of adding a new queue store.
- **D-05:** Promotion eligibility is checked just before popping the next task. This is not continuous dynamic re-ranking.
- **D-06:** The pre-pop check inspects only a bounded candidate window, not the entire queue.
- **D-07:** If no eligible task appears in the bounded window, the gateway pops normally and preserves existing scheduler/FIFO order.

### Priority And Quota
- **D-08:** SLA promotion never moves a task into a higher priority class. It can only reorder within the task's already resolved priority.
- **D-09:** If meeting SLA would require priority escalation or quota borrowing, do not escalate. Record a sanitized `blocked_by_priority_or_quota` outcome.
- **D-10:** SLA promotion never uses prompt-derived urgency or complexity. Prompt text has zero priority influence.

### Audit And Metrics Evidence
- **D-11:** Record aggregated metrics plus sanitized durable audit only when promotion happens or is blocked. Do not audit every eligibility check.
- **D-12:** Allowed evidence fields are policy ID, tenant ID/class, model class, request kind, priority, and outcome.
- **D-13:** Explicitly excluded from metrics, audit, logs, and promotion policy: prompts, API keys, auth headers, provider payloads, embeddings, semantic-cache payloads, and raw task text.
- **D-14:** Metrics distinguish `promoted`, `not_eligible`, `blocked_by_priority_or_quota`, `disabled`, and `error`.

### Agent Discretion
Planner may choose exact config names, default bounded-window size, and metric names as long as the decisions above hold, existing queue primitives are reused, and promotion remains disabled by default.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 19 goal, requirements, success criteria, and candidate plan slices.
- `.planning/REQUIREMENTS.md` - SLA-01 through SLA-04 and v7.5 out-of-scope rules.
- `.planning/PROJECT.md` - Project-level scheduler optionality, priority safety, and gateway-owned queue boundary.
- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-CONTEXT.md` - Predictor/Scheduler boundary and sanitized evidence constraints.
- `.planning/phases/17-semantic-neighbor-feature-aggregates/17-CONTEXT.md` - Safe structured scheduler feature boundaries.
- `.planning/phases/16-a-b-rollout-and-prediction-quality/16-CONTEXT.md` - Rollout, quality, and low-cardinality metric precedent.
- `.planning/phases/14-scheduler-queue-foundation/14-CONTEXT.md` - Queue ownership, FIFO fallback, and priority safety baseline.

### Current Source
- `internal/scheduler/queue.go` - `QueueBackend` contract and queue item score shape.
- `internal/scheduler/queue_redis.go` - Redis ZSET score update behavior via `ZAdd`.
- `internal/scheduler/queue_memory.go` - Memory queue duplicate task score update via `heap.Fix`.
- `internal/scheduler/queue_fallback.go` - Redis-to-memory fallback queue behavior that promotion must preserve.
- `internal/scheduler/intake.go` - Gateway-owned enqueue path, safe feature extraction, scheduler score metadata, and priority resolution.
- `internal/scheduler/executor.go` - Current pre-pop execution path through `Executor.RunOne`.
- `internal/scheduler/priority.go` - Trusted priority resolver, max-priority, and high-priority quota behavior.
- `internal/config/config.go` and `internal/config/config_validation.go` - Scheduler config defaults, JSON merge, env loading, and validation patterns.
- `internal/observability/metrics.go` and `internal/observability/prometheus.go` - Existing scheduler metrics and label sanitization patterns.
- `internal/scheduler/admin_scheduler_service.go` and `internal/controlstate/audit.go` - Existing scheduler audit path and `SafeAuditMetadata`.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `QueueBackend.Push` already acts as score replacement for existing task IDs in memory and Redis backends.
- `MemoryQueue.Push` updates duplicate task IDs and calls `heap.Fix`, which fits one-time promotion.
- `RedisQueue.Push` uses `ZAdd`, which replaces member score without a separate promotion API.
- `FallbackQueue` centralizes Redis/memory fallback and should keep promotion behavior consistent across backends.
- `PriorityResolver.Resolve` already enforces trusted priority, max priority, and high-priority quota.
- `controlstate.SafeAuditMetadata` already redacts sensitive audit metadata.

### Established Patterns
- Scheduler is optional and disabled by default.
- Gateway owns queueing, execution, task state, fallback, semantic lookup, and SLA promotion.
- Scheduler receives only bounded scalar/enum features and remains stateless.
- Observability uses sanitized, low-cardinality labels only.
- Optional scheduler enhancements fail open and must not block gateway forwarding.

### Integration Points
- Add disabled-by-default SLA promotion config under `SchedulerConfig`.
- Validate SLA rules in existing scheduler config validation.
- Add a bounded pre-pop promotion step before `Executor.RunOne` calls `Queue.PopMin`.
- Record promotion/block outcomes through existing metrics and durable audit patterns.

</code_context>

<specifics>
## Specific Ideas

- Treat SLA promotion as a narrow exception to the static virtual-deadline model: threshold-triggered, bounded, and one-time.
- Prefer reusing existing queue score update semantics over adding a new promotion-specific queue API.
- A bounded window size should be configurable or named as a constant, but the implementation should not scan the full queue by default.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

*Phase: 19-SLA Waiting-Time Promotion*
*Context gathered: 2026-07-05T13:43:48-07:00*
