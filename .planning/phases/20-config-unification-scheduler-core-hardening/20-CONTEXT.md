# Phase 20: Config Unification + Scheduler Core Hardening - Context

**Gathered:** 2026-07-05T21:18:49-07:00
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 20 unifies gateway subsystem config into named nested structs and hardens the existing optional Scheduler path. It covers ControlState, Redis, Cache/Qdrant, component-scoped config files, scheduler executor concurrency, Redis task execution idempotency, QueueGuard observability, semantic-neighbor embedding input caps, and startup Qdrant collection initialization.

This phase does not change the OpenAI-compatible data-plane contract. Scheduler remains optional, disabled by default, fail-open, and stateless. Gateway continues to own queueing, task state, execution, semantic lookup, promotion, fallback, and sensitive payload boundaries.

</domain>

<decisions>
## Implementation Decisions

### Config Shape And Compatibility
- **D-01:** Canonical nested structs are the source of truth. Use named nested config blocks for `ControlState`, `Redis`, and `Cache`, matching the existing `SchedulerConfig` pattern.
- **D-02:** Existing ENV variable names remain valid and backward-compatible. Legacy flat JSON keys are accepted as input aliases.
- **D-03:** Do not keep long-term duplicate root fields as a second live source of truth. Normalize legacy flat inputs into the nested structs during loading.
- **D-04:** If both nested and legacy flat JSON keys are present, nested keys win.
- **D-05:** ENV seeds defaults, the main config JSON overrides ENV, and component-scoped config files override their component block last.

### Cache And Component Config Files
- **D-06:** Use one `Cache` block for semantic cache plus vector backend config. Qdrant settings live under Cache, for example `cache.qdrant.addr`.
- **D-07:** `cache.vector_dimension` is the shared configured vector dimension for semantic cache and semantic neighbors in Phase 20.
- **D-08:** Component config files load after the main config and override only their component block. `scheduler_config_file` overrides Scheduler config; `cache_config_file` overrides Cache config.
- **D-09:** Disabled optional subsystems receive structural/default validation only. Connection details such as Scheduler endpoint, Redis address, and Qdrant address are required only when the relevant subsystem is enabled.
- **D-10:** Phase 20 should add minimal config examples now: `config.json.example`, `.env.example`, and small scheduler/cache component examples. Full operator docs and runbook polish remain Phase 22 work.

### Scheduler Execution Hardening
- **D-11:** `SCHEDULER_EXECUTOR_CONCURRENCY > 1` means fixed worker slots drain the shared queue while preserving per-task registry delivery.
- **D-12:** Redis idempotency locks are claimed just before executing a popped task and released after delivery. Use Redis `SET NX` per task ID with a short TTL.
- **D-13:** Single-node and memory-queue deployments skip Redis execution locks without code or config changes.
- **D-14:** If a worker pops a task but cannot claim the Redis lock, it skips execution and delivery, records sanitized lock-skip evidence, and continues draining.
- **D-15:** QueueGuard observability belongs in `TaskIntake.Submit` around `Guard.Check`, where backend, priority, throttle/reject outcome, guard errors, and queue depth are all known.

### Semantic-Neighbor Safeguards
- **D-16:** `requestText()` enforces a configurable/defaulted character cap before the embedding call.
- **D-17:** Default semantic-neighbor embedding input cap is `16000` characters.
- **D-18:** Long request text is safely truncated before embedding. Record sanitized truncation evidence only; do not log raw prompt text.
- **D-19:** When semantic neighbors are enabled, app startup checks/creates the `scheduler_training_samples` Qdrant collection using the configured `cache.vector_dimension`.
- **D-20:** If Qdrant collection initialization fails, semantic neighbors disable and fail open. Gateway startup and request forwarding should continue.

### Agent Discretion
Planner may choose exact struct names, helper names, metric names, lock TTL, and example file locations as long as the decisions above hold, existing ENV compatibility is preserved, optional subsystems stay disabled by default, and sensitive payloads never enter logs, metrics, Scheduler, or training artifacts.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 20 scope and v7.6 milestone boundary.
- `.planning/REQUIREMENTS.md` - CFG-01 through CFG-04, SCH-05 through SCH-07, and QDR-05 through QDR-06.
- `.planning/PROJECT.md` - Project-level scheduler optionality, priority safety, config unification, and sensitive-payload constraints.
- `.planning/phases/19-sla-waiting-time-promotion/19-CONTEXT.md` - Gateway-owned queue and promotion boundary plus sanitized metric/audit precedent.
- `.planning/phases/18-anomaly-and-ood-conservative-scoring/18-CONTEXT.md` - Predictor/Scheduler boundary and fail-open optional ML behavior.
- `.planning/phases/17-semantic-neighbor-feature-aggregates/17-CONTEXT.md` - Semantic-neighbor safety boundary, safe aggregate fields, and Gateway-owned lookup.

### Config
- `internal/config/config.go` - Current flat `Config` fields, existing `SchedulerConfig`, env loading, JSON merge, defaults, and scheduler merge helper.
- `internal/config/config_validation.go` - Current validation boundaries for optional scheduler, semantic cache, and duration/limit fields.
- `.planning/config.json` - Project GSD config, including `commit_docs: false`.

### Scheduler
- `internal/app/app.go` - Scheduler runner wiring, queue backend selection, intake/executor construction, and semantic-neighbor service wiring.
- `internal/scheduler/executor.go` - Current `Executor.RunOne` and `SynchronousRunner.waitForTask` execution flow.
- `internal/scheduler/intake.go` - `TaskIntake.Submit`, priority resolution, `QueueGuard.Check`, queue push, and queue depth metric location.
- `internal/scheduler/queue.go` - Queue backend contract.
- `internal/scheduler/queue_guard.go` - Soft/hard queue admission policy helper.
- `internal/scheduler/queue_redis.go` - Redis ZSET queue behavior.
- `internal/scheduler/queue_memory.go` - Memory queue behavior.
- `internal/scheduler/queue_fallback.go` - Redis-to-memory fallback behavior.
- `internal/observability/metrics.go` - Metrics interface to extend for queue admission and lock-skip evidence.
- `internal/observability/prometheus.go` - Prometheus implementation and label-sanitization pattern.

### Semantic Neighbors And Vectors
- `internal/app/semantic_cache.go` - Vector adapter creation and Qdrant/Redis VSS fallback wiring.
- `internal/app/semantic_neighbors.go` - Semantic-neighbor service construction and enablement checks.
- `internal/scheduler/semantic_neighbors.go` - `requestText()`, embedding call, collection name, enrichment, indexing, metrics, and fail-open defaults.
- `internal/storage/interfaces.go` - `VectorAdapter` boundary that may need an ensure-collection capability or narrow adapter-specific path.
- `internal/storage/qdrant.go` - Qdrant client, lazy collection creation, and current dimension-from-first-vector behavior.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `SchedulerConfig` already provides the desired nested-struct pattern for new `ControlState`, `Redis`, and `Cache` config blocks.
- `mergeSchedulerConfig` and `applySchedulerDefaults` are existing examples for component-specific merge/default behavior.
- `TaskIntake.Submit` already has priority, backend, guard, queue length, and metrics in one place.
- `QueueGuard.Check` already distinguishes soft-limit throttling and hard-limit rejection.
- `QueueBackend` already hides Redis vs memory queue behavior from the executor.
- `RedisQueue` already depends on `redis.Cmdable`, which can support a narrow task-lock helper without changing data-plane APIs.
- `SemanticNeighborService` already fails open on missing dependencies and enrichment errors.
- `QdrantVectorAdapter.Insert` already knows how to create a collection, but currently does so lazily from the first vector length.

### Established Patterns
- Optional scheduler, Redis, semantic cache, Qdrant, ONNX, and semantic-neighbor features must be disabled by default and fail open.
- Gateway owns queueing, execution, task state, fallback, and semantic lookup.
- Scheduler remains a stateless scoring oracle.
- Observability labels must stay sanitized and low-cardinality.
- Existing ENV deployments must keep starting without Scheduler, Redis, or semantic cache enabled.
- Do not log prompts, API keys, authorization headers, provider payloads, embeddings, semantic-cache payloads, or raw task text.

### Integration Points
- Normalize legacy flat config fields into nested structs during `LoadConfig`.
- Add component-scoped file loading after main config parsing and before defaults/validation.
- Wire executor worker slots and Redis task locks through scheduler runner construction in `internal/app/app.go`.
- Record QueueGuard admission counters and queue-depth histogram from `TaskIntake.Submit`.
- Enforce `requestText()` cap before `SemanticNeighborService.embed` calls the provider embedder.
- Ensure Qdrant collection during semantic-neighbor service startup when semantic neighbors are enabled.

</code_context>

<specifics>
## Specific Ideas

- Keep Phase 20 documentation minimal and operator-useful: examples that compile conceptually, not a full runbook.
- Prefer the shortest compatibility migration: parse both old and new shapes, then operate on nested config only.
- Treat Redis idempotency lock skips as coordination evidence, not request-facing execution errors.
- Use `16000` characters as a named default, not an inline magic number.

</specifics>

<deferred>
## Deferred Ideas

- Full Scheduler 1.0 operator runbook, deployment guide, and degradation-scenario documentation remain Phase 22 work.
- Dedicated semantic-neighbor embedding model configuration belongs to Phase 21 (`QDR-08`).
- Precise completed-sample hydration by IDs belongs to Phase 21 (`QDR-07`).

</deferred>

---

*Phase: 20-Config Unification + Scheduler Core Hardening*
*Context gathered: 2026-07-05T21:18:49-07:00*
