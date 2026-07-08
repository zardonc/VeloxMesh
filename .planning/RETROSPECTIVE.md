# Project Retrospective

## Cross-Milestone Trends

| Milestone | Shipped | Phases |
|-----------|---------|--------|
| v7.7      | 2026-07-08 | 3      |
| v7.6      | 2026-07-06 | 3      |
| v7.5      | 2026-07-05 | 3      |
| v7.4      | 2026-07-04 | 3      |
| v7.3      | 2026-07-03 | 1      |
| v7.2      | 2026-07-03 | 1      |
| v7.1      | 2026-07-01 | 1      |
| v7.0      | 2026-06-30 | 3      |
| v4        | 2026-06-23 | 4      |

---

## Milestone: v7.7 - Scheduler Hardening + Plan 3 Vector Compatibility

**Shipped:** 2026-07-08
**Phases:** 3 | **Plans:** 3

### What Was Built
- Default in-memory Scheduler queueing for unset, `auto`, and `memory` backend values.
- Explicit Redis Scheduler queueing scoped to the gateway node ID.
- FallbackQueue recovery reads so memory fallback entries are not stranded after Redis recovery.
- Plan 3 deployment guidance for single-node `App + SQLite + LanceDB/Qdrant`.
- Runbook, README, `.env.example`, phase verification, validation, and audit evidence for the shipped behavior.

### What Worked
- Keeping Redis queueing explicit avoided accidental cross-node Scheduler semantics.
- Fixing fallback recovery in the shared queue implementation covered all queue readers with a small change.
- Backfilling phase artifacts before final closeout made the milestone audit reflect the actual shipped state.

### What Was Inefficient
- The first milestone audit ran before Phase 23-25 artifacts existed, so the audit initially reported orphaned requirements.
- `milestone.complete` over-counted historical active phase directories and required manual correction to v7.7 scope.

### Patterns Established
- Scheduler queue backend selection should default to the simplest single-node path.
- Plan 3 docs should state vector-store exclusivity and known runtime limits directly.
- Milestone closeout should verify phase artifacts exist before running archive automation.

### Key Lessons
- Completion artifacts are part of the release, not a later cleanup chore.
- GSD archive automation needs a narrow milestone scope when old phase directories remain active.
- Documented runtime limits are acceptable when build compatibility and degradation behavior are tested.

## Milestone: v7.6 - Scheduler 1.0 + Config System Unification

**Shipped:** 2026-07-06
**Phases:** 3 | **Plans:** 7

### What Was Built
- Nested ControlState, Redis, Cache/Qdrant, and Scheduler config blocks while preserving legacy env compatibility.
- Component-scoped scheduler/cache config file loading.
- Scheduler executor concurrency controls, Redis task idempotency locks, and QueueGuard admission metrics.
- Semantic-neighbor input caps, Qdrant collection startup checks, precise sample hydration, and configurable embedding model.
- Scheduler admin status, SLA promotion rules API, safe training-sample export, heuristic config overrides, and SchedulerType quality attribution.
- Scheduler 1.0 runbook, updated `.env.example` and `config.json.example`, vector-backend guidance, and UAT evidence.

### What Worked
- Keeping Scheduler optional and disabled by default preserved the existing gateway startup path.
- Component-scoped config files solved operator customization without adding new runtime control surfaces.
- Real-component UAT caught the important distinction between documented behavior and deployed-provider behavior.

### What Was Inefficient
- Phase 21 UAT and v7.6 Nyquist validation artifacts had to be reconciled during closeout instead of during the phase.
- Local Python/uv cache configuration caused an initial test failure before `UV_CACHE_DIR` was redirected into the project `.tmp` area.

### Patterns Established
- Optional subsystem configs should use named nested structs plus legacy aliases for compatibility.
- Scheduler-facing admin/export APIs must expose safe structured features only.
- Milestone closeout should run `audit-open`, full tests, and build before archiving.

### Key Lessons
- Validation artifacts should be created at phase close, not milestone close.
- Python worker/model smoke tests need a project-local cache path in constrained workspaces.
- Documentation and env examples are most useful when they mirror the exact structured config shape operators run.

---

## Milestone: v7.5 - Scheduler Enhancements

**Shipped:** 2026-07-05
**Phases:** 3 | **Plans:** 10

### What Was Built
- Optional Gateway-owned semantic-neighbor aggregate features with safe bounded scheduler fields and fail-open enrichment.
- Training/export/ONNX support for semantic aggregate fields while keeping raw prompts, embeddings, auth headers, API keys, and provider secrets out of Scheduler.
- Conservative anomaly/OOD scoring with manifest metadata, quality rollups, and low-cardinality metrics.
- A model-neutral predictive scorer backed by a real Python ONNX Runtime worker and Scheduler RPC smoke coverage.
- Disabled-by-default tenant SLA waiting-time promotion with bounded Redis/memory queue reordering and sanitized audit/log/metric evidence.

### What Worked
- Keeping semantic lookup and queue promotion in Gateway preserved Scheduler as a stateless scoring service.
- Requiring the real Python ONNX worker smoke closed the temporary-parser gap and proved the final production-shaped call chain.
- Retrofitting Nyquist validation from plan and summary artifacts was cheap because each task already carried runnable verification commands.

### What Was Inefficient
- Phase 18 needed corrective work after the first ONNX path did not prove real runtime invocation.
- The milestone audit initially failed the Nyquist process gate because validation artifacts were missing despite automated tests existing.
- `18-VERIFICATION.md` still uses `status: complete` instead of the workflow matrix value `passed`.

### Patterns Established
- Optional scheduler enrichments default off, validate when enabled, and fail open at runtime.
- Predictor workers return model evidence; Scheduler policy owns confidence, uncertainty, fallback, and queue scoring.
- SLA promotion uses bounded pre-pop inspection and existing score replacement instead of adding a second queue mutation API.

### Key Lessons
- Production-shape model verification should be an acceptance criterion whenever a phase introduces model runtime behavior.
- Nyquist validation files should be created during phase execution, not reconstructed at milestone close.
- Low-cardinality metrics and `SafeAuditMetadata` are enough for operator evidence without expanding the sensitive data surface.

---

## Milestone: v7.4 - Gateway Scheduler

**Shipped:** 2026-07-04
**Phases:** 3 | **Plans:** 10

### What Was Built
- Optional scheduler gRPC path with disabled-by-default config, 15ms timeout, circuit-breaker protection, and FIFO fallback.
- Gateway-owned Redis ZSET queueing with task-id-only storage and an in-memory single-node fallback.
- Trusted priority resolution, static virtual deadline scoring, and standalone heuristic Scheduler service metrics.
- Safe opt-in training feedback, offline `uv` training/export/evaluate/publish tooling, and startup-loaded ONNX scheduler mode.
- Weighted heuristic/ONNX rollout, prediction-quality rollups, operator alerts, and authenticated runtime rollback controls.

### What Worked
- Keeping the Scheduler stateless preserved the gateway as source of truth for task ownership, execution, and fallback behavior.
- Treating heuristic scoring as the cold-start baseline let ONNX support land without making model artifacts mandatory.
- Retroactive validation converted the completed implementation into explicit Nyquist evidence before closeout.

### What Was Inefficient
- Some completion artifacts needed reconciliation after implementation, especially global planning docs and final validation records.
- Phase archive automation created a duplicate milestone entry that required manual deduplication.

### Patterns Established
- Scheduler calls are optional, bounded, and fail open to FIFO.
- Training samples must contain safe feature snapshots and completion labels only, never raw prompts or secrets.
- Runtime rollout controls should expose rollback without automatically changing production behavior on alert.

### Key Lessons
- Archive before deletion is the right closeout shape for milestone-scoped requirements.
- Planning docs should be collapsed immediately after milestone close so the active context stays small.
- Model paths are easier to validate when the serving contract is identical to the heuristic path.

---

## Milestone: v7.2 — Multi-Node Coordination

**Shipped:** 2026-07-03
**Phases:** 1 | **Plans:** 5

### What Was Built
- Redis coordination adapter for leadership election and streaming.
- SQLite WAL replication producer and consumer via Redis stream.
- Leader-only write fencing for relational routes.
- Recovery worker for sync replay on follower nodes.
- Resilient multi-node cluster topology and health API.

### What Worked
- Reusing the Redis test harness allowed full coverage of multi-node leader loss, follower failover, and write fencing without needing a separate cluster setup.
- Enforcing write boundaries at the middleware layer via `RequireWritable` effectively shielded the underlying adapter logic.

### What Was Inefficient
- N/A.

### Patterns Established
- Multi-node topology strictly isolates the vector paths (Qdrant native replication) from the relational paths (SQLite over Redis).

### Key Lessons
- Explicit interface separation between coordination primitives (Redis) and persistence logic makes cluster failover predictable.
- Local E2E testing of concurrency issues is very reliable when using isolated Redis instances across multiple gateway instances.

---

## Milestone: v7.0 — Plan 1 Foundation

**Shipped:** 2026-06-30
**Phases:** 3 | **Plans:** 8

### What Was Built
- SQLite-first Plan 1 runtime foundation and adapter contracts.
- Qdrant primary vector path with graceful degradation.
- Configurable semantic pipeline with persisted global and per-user rules.
- Ordered semantic rule execution with safe skip behavior.
- Redis hot-state primitives, atomic limiter, session blacklist, auth cache, and cost aggregation.
- Redis VSS fallback and typed config event routing.

### What Worked
- Keeping Redis as hot state and SQLite as source of truth kept the architecture clear.
- Real Redis Stack verification caught and then confirmed the Redis VSS integration behavior.
- The semantic pipeline stayed compact by using a registry and deterministic execution order.

### What Was Inefficient
- ROADMAP/STATE lagged behind actual Phase 8-9 progress and needed manual reconciliation at close.
- REQUIREMENTS.md still reflected v5 scope, so v7 requirements had to be archived from actual summaries instead of a clean active requirement file.

### Patterns Established
- Source-of-truth boundaries: SQLite durable, Redis hot, Qdrant primary vector.
- Fallback-only vector behavior for Redis VSS.
- Rule registry pattern for semantic processing.

### Key Lessons
- Milestone scopes should be split before execution when a roadmap section contains future phases.
- Integration tests should read environment variables for real local services instead of hardcoding localhost.
- UAT files need normalized status values so audit-open can distinguish pass/complete from unknown.

---

## Milestone: v4 — Streaming, Rate Limits, Cache, and Cost

**Shipped:** 2026-06-23
**Phases:** 4 | **Plans:** 20

### What Was Built
- Initial Go/Chi gateway walking skeleton.
- Health-aware multi-provider routing and native adapters for Anthropic/Gemini.
- Durable control state with PostgreSQL/SQLite and Redis-backed hot state.
- SSE streaming proxy.
- Credit quota limits and admission control.
- Semantic caching.
- Cost and usage tracking with final usage settlement.
- Circuit breaker and fallback-chain behaviors.

### What Worked
- The transition from static configuration to durable database-backed configuration was highly successful.
- Extensive unit and integration testing locally with full PostgreSQL and Redis infrastructures eliminated many regressions.

### What Was Inefficient
- Deferring the VERIFICATION.md generation required an explicit audit block at the end, but test automation proved strong.

### Patterns Established
- Using `.env` and `.env.local` to securely test end-to-end integration flows without committing secrets.
- Creating isolated test/database environments for reliable validation.

### Key Lessons
- Advanced gateway features like streaming and caching should only be implemented after core durable and routing states are firmly established, as this prevented overlapping technical debt.
