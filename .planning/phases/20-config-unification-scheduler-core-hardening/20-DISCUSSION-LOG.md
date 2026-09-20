# Phase 20: Config Unification + Scheduler Core Hardening - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md. This log preserves the alternatives considered.

**Date:** 2026-07-05T21:18:49-07:00
**Phase:** 20-Config Unification + Scheduler Core Hardening
**Areas discussed:** Config shape + compatibility, Scheduler execution hardening, Semantic-neighbor safeguards

---

## Config Shape + Compatibility

### Canonical Config Shape

| Option | Description | Selected |
|--------|-------------|----------|
| Canonical nested structs, legacy aliases | Use nested `ControlState`, `Redis`, and `Cache/Qdrant` as source of truth; keep old ENV names and flat JSON aliases. | yes |
| Keep nested and flat fields live | Add nested structs but keep root fields populated too. | |
| Nested config only for files | ENV stays backward-compatible, but config files must use nested blocks. | |

**User's choice:** Canonical nested structs, legacy aliases.
**Notes:** Nested config is canonical. ENV names and flat JSON keys remain compatibility inputs only.

### Cache Shape

| Option | Description | Selected |
|--------|-------------|----------|
| One Cache block with Qdrant fields inside | `cache.enabled`, `cache.provider`, `cache.vector_store`, `cache.qdrant.addr`, etc. | yes |
| Separate Cache and Qdrant blocks | Cleaner for Qdrant reuse, more config surface. | |
| Infrastructure-style Qdrant block | Groups external infra together, less clear for cache behavior. | |

**User's choice:** One Cache block with Qdrant fields inside.
**Notes:** `Cache` owns semantic cache and vector backend config.

### Component Config Files

| Option | Description | Selected |
|--------|-------------|----------|
| Component file replaces that component block | Load after main config and override only `scheduler` or `cache`. | yes |
| Component file deep-merges over inline block | More flexible, more surprising. | |
| Component file wins completely and inline block is ignored | Clearest precedence, requires duplicate full component config. | |

**User's choice:** Component file replaces that component block.
**Notes:** Component files have local blast radius.

### Example Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal examples now, full docs later | Add examples in Phase 20; leave runbook/docs polish to Phase 22. | yes |
| Full config docs now | Bigger Phase 20. | |
| Defer examples to Phase 22 | Smallest Phase 20, conflicts with CFG-03. | |

**User's choice:** Minimal examples now, full docs later.
**Notes:** Phase 20 should satisfy CFG-03 without absorbing Phase 22.

### Disabled Optional Validation

| Option | Description | Selected |
|--------|-------------|----------|
| Validate structural/default fields only | Require connection details only when enabled. | yes |
| Validate everything always | Catches bad config early, breaks optional defaults. | |
| Skip all validation when disabled | Hides typos until enablement. | |

**User's choice:** Validate structural/default fields only.
**Notes:** Matches CFG-04.

### Nested Versus Legacy Conflicts

| Option | Description | Selected |
|--------|-------------|----------|
| Nested wins | New canonical shape wins over flat aliases. | yes |
| Reject as ambiguous | Forces clean config, rougher migration. | |
| Legacy wins | Least surprising for old files, undercuts new shape. | |

**User's choice:** Nested wins.
**Notes:** New shape is canonical.

### ENV And Config File Precedence

| Option | Description | Selected |
|--------|-------------|----------|
| ENV seeds defaults, config file overrides | Keep current loader behavior. | yes |
| ENV always wins | Twelve-factor style, changes current semantics. | |
| Only CONFIG_FILE wins | Clean file mode, harder deployment overrides. | |

**User's choice:** ENV seeds defaults, config file overrides.
**Notes:** Final precedence: ENV, then main config file, then component files.

---

## Scheduler Execution Hardening

### Executor Concurrency

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed worker slots drain the shared queue | Semaphore/worker-slot model with per-task registry delivery. | yes |
| Only allow concurrent RunOne calls from callers | Minimal, caller-dependent. | |
| Keep request-local execution only | Does not satisfy SCH-05. | |

**User's choice:** Fixed worker slots drain the shared queue.
**Notes:** Concurrency is internal executor capacity, not just caller behavior.

### Redis Idempotency Lock

| Option | Description | Selected |
|--------|-------------|----------|
| Claim before executing, release after delivery | Redis `SET NX` per task ID with TTL; memory/single-node skip. | yes |
| Claim at enqueue time | Earlier duplicate prevention, weaker execution race protection. | |
| Claim and never release until TTL | Simpler, stale locks linger. | |

**User's choice:** Claim before executing, release after delivery.
**Notes:** Lock protects multi-node execution race, not enqueue.

### Lock Claim Failure

| Option | Description | Selected |
|--------|-------------|----------|
| Skip and continue draining | Do not execute/deliver; record lock-skip evidence. | yes |
| Requeue the task | Can churn if another node owns it. | |
| Treat as execution error | Noisy and request-facing. | |

**User's choice:** Skip and continue draining.
**Notes:** Lock-skip is coordination evidence, not a caller error.

### QueueGuard Observability

| Option | Description | Selected |
|--------|-------------|----------|
| In TaskIntake.Submit around Guard.Check | Admission decision has backend, priority, depth, and metrics. | yes |
| Inside QueueGuard.Check | Centralizes metrics but couples pure helper to observability. | |
| Split: guard returns reason, intake records metrics | More code; keeps guard pure. | |

**User's choice:** In `TaskIntake.Submit` around `Guard.Check`.
**Notes:** Keep metric recording near the admission boundary.

---

## Semantic-Neighbor Safeguards

### Input Cap Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Character cap with safe truncation before embed | Provider-agnostic and simple. | yes |
| Token-count cap | More accurate, adds tokenizer complexity. | |
| Reject enrichment for oversized input | Avoids truncation bias, loses signal. | |

**User's choice:** Character cap with safe truncation before embed.
**Notes:** No new tokenizer dependency for Phase 20.

### Default Cap

| Option | Description | Selected |
|--------|-------------|----------|
| 16,000 characters | Useful context with lower token-limit risk. | yes |
| 8,000 characters | Safer/cheaper, loses more context. | |
| 32,000 characters | Preserves more, closer to provider limits. | |

**User's choice:** 16,000 characters.
**Notes:** Use a named default.

### Qdrant Startup Collection

| Option | Description | Selected |
|--------|-------------|----------|
| Ensure collection when semantic neighbors are enabled | Startup check/create with configured dimension; fail open. | yes |
| Keep lazy creation on first insert, add dimension check | Smaller, but misses QDR-06. | |
| Require collection pre-created by operators | Strict, less self-healing. | |

**User's choice:** Ensure collection when semantic neighbors are enabled.
**Notes:** Failure disables semantic neighbors, not gateway startup.

### Vector Dimension Source

| Option | Description | Selected |
|--------|-------------|----------|
| cache.vector_dimension | Shared knob for cache and semantic neighbors. | yes |
| scheduler.semantic_neighbors_vector_dimension | More explicit, more config. | |
| Infer from first embedding | Avoids config, conflicts with startup creation. | |

**User's choice:** `cache.vector_dimension`.
**Notes:** Reuse existing semantic-cache dimension in Phase 20.

---

## Agent Discretion

- Exact struct/helper names.
- Metric names and label values, as long as labels are sanitized and low-cardinality.
- Redis lock TTL.
- Minimal example file layout.

## Deferred Ideas

- Full operator docs and runbook remain Phase 22.
- Semantic-neighbor embedding model configuration remains Phase 21.
- Precise completed-sample hydration by IDs remains Phase 21.
