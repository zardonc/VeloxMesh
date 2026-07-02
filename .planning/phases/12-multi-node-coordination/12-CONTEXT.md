# Phase 12: Multi-Node Coordination - Context

**Gathered:** 2026-07-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 12 delivers Plan 2 multi-node runtime coordination for VeloxMesh: Redis-backed node registration and leader election, SQLite relational write fencing, Redis Stream replication for relational control state, recovery through fallback log replay, internal/admin topology health, and focused multi-node tests. It does not change the OpenAI-compatible data-plane contract.

</domain>

<decisions>
## Implementation Decisions

### Leader and Write Behavior
- **D-01:** Non-leader SQLite relational writes must fail fast. Do not implement follower write forwarding or local write queueing in Phase 12.
- **D-02:** When no leader is writable during failover, ordinary client write attempts return a generic `503` with retry guidance.
- **D-03:** Follower nodes may serve reads, but any stale/lag/topology detail is internal/admin-only. Ordinary user responses must not reveal follower status.
- **D-04:** Leader election uses the roadmap default: Redis `SET NX`, 10 second TTL, and 3 second heartbeat. Do not add a Phase 12 TTL/heartbeat configuration matrix.

### Replication and Recovery
- **D-05:** All `controlstate` repository writes enter the Redis Stream replication path. The planner should avoid hand-picking only "important" repositories.
- **D-06:** Replica lag health uses both elapsed lag time and Redis Stream distance/pending metrics. Named constants should define the default thresholds.
- **D-07:** Failed sync/replay operations are written to the fallback log.
- **D-08:** Recovery uses a background worker with bounded retry. Exhausted retries are marked failed for later operator/manual handling; do not retry forever or crash the node for one failed replay.

### Health, Readiness, and Security Surface
- **D-09:** Multi-node details such as `node_id`, `role`, `leader_id`, `wal_lag`, `writable`, and `degraded_reason` are visible only through internal/admin-only health or topology surfaces.
- **D-10:** Ordinary users and ordinary data-plane responses must never expose leader/follower, primary/replica, node identity, lag, failover, or topology details.
- **D-11:** Ordinary `/healthz` remains coarse. If detailed topology is needed, expose it through an internal/admin-only endpoint guarded by existing internal/admin auth patterns.
- **D-12:** Readiness fails only when a node cannot perform its own role: leader cannot write, follower cannot safely read or is past lag threshold, or Redis coordination makes role/leader state unknowable. Being a follower is not itself a readiness failure.

### Multi-Node Testing
- **D-13:** Tests must simulate multiple nodes during development, but cover main scenarios and likely abnormal scenarios only. Do not build a full chaos matrix in Phase 12.
- **D-14:** Minimum test scenarios: 2-3 node startup, leader kill/failover, graceful shutdown, Redis outage/recovery, replica lag/degraded, and non-leader write rejection.
- **D-15:** Use an in-process multi-node Go test harness: multiple app/server instances in one test process, independent SQLite DSNs, and a shared Redis test instance or fake. Docker Compose is not required for Phase 12.

### Agent Discretion
- The planner may choose the exact internal/admin route names and default lag thresholds, as long as ordinary user surfaces do not leak topology and constants are named.
- The planner may choose whether detailed stream offsets/retry counters live in an internal endpoint, debug endpoint, logs, or metrics; they must not be in ordinary health or data-plane responses.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning Scope
- `.planning/ROADMAP.md` — Phase 12 scope, success criteria, and out-of-scope future milestones.
- `.planning/REQUIREMENTS.md` — v7.2 requirement IDs and traceability for coordination, replication, fencing, recovery, health, and tests.
- `.planning/PROJECT.md` — current milestone goals, project constraints, and security/data-plane contract decisions.

### Existing Code Touchpoints
- `internal/controlstate/sqlite/repository.go` — SQLite repository surface, `DBForTest`, and existing fallback log repository.
- `internal/health/redis_store.go` — existing Redis-backed hot-state patterns for health snapshots and local fallback.
- `internal/http/router.go` — existing `/healthz` and `/readyz` route registration.
- `internal/http/handlers/health.go` — existing readiness behavior and sanitization expectations.
- `tests/integration/health_test.go` — existing health/readiness integration tests and secret-safety expectations.
- `internal/storage/qdrant.go` — Qdrant vector adapter; reinforces that vector state is separate from SQLite relational replication.
- `internal/storage/redis_vss.go` — Redis VSS fallback adapter; do not confuse vector fallback with Phase 12 relational WAL replication.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/controlstate/sqlite.Repository`: central SQLite repository wrapper; likely place to wrap or intercept relational writes for replication events.
- `fallbackLogRepo`: existing fallback log insert/list/update behavior can support bounded replay records instead of introducing a parallel dead-letter store.
- `internal/health.RedisStore`: shows existing Redis hot-state pattern with local fallback, TTL handling, and JSON snapshots.
- `DBForTest`: available for focused repository and multi-node harness tests.

### Established Patterns
- Redis is already hot state, not source of truth. Phase 12 should keep SQLite authoritative and Redis as coordination/transport.
- Health/readiness tests already check that sensitive details are not leaked; Phase 12 should extend that pattern to topology secrecy.
- Vector state lives behind Qdrant/Redis VSS adapters and stays out of relational WAL replication.

### Integration Points
- New coordination code should connect to app wiring without changing the public OpenAI-compatible API contract.
- Write fencing should sit close to the SQLite control-state write boundary so sibling callers inherit the same protection.
- Internal/admin topology should be separate from ordinary `/healthz`, `/readyz`, and data-plane responses.

</code_context>

<specifics>
## Specific Ideas

- Multi-node tests should be practical and high-value, not exhaustive: cover main path plus likely abnormal cases.
- Ordinary users must not learn whether they hit a leader, follower, replica, failover window, or lagging node.

</specifics>

<deferred>
## Deferred Ideas

- Docker Compose or full external multi-node deployment tests are optional future hardening, not required for Phase 12.
- Full chaos matrix testing is deferred unless the Phase 12 implementation proves too risky without it.
- BFF/Admin Console topology UI remains Phase 11 or later.
- PostgreSQL/pgvector extension remains Phase 13 or later.

</deferred>

---

*Phase: 12-multi-node-coordination*
*Context gathered: 2026-07-02*
