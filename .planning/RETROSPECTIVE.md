# Project Retrospective

## Cross-Milestone Trends

| Milestone | Shipped | Phases |
|-----------|---------|--------|
| v7.0      | 2026-06-30 | 3      |
| v4        | 2026-06-23 | 4      |

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
