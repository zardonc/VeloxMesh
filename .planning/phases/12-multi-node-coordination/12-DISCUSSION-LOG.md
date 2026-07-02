# Phase 12: Multi-Node Coordination - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-07-02
**Phase:** 12-multi-node-coordination
**Areas discussed:** Leader and write behavior, Replication and recovery boundary, Health and readiness surface, Multi-node test scenarios, Security surface

---

## Leader and Write Behavior

| Option | Description | Selected |
| --- | --- | --- |
| Fail fast with retryable error | Non-leader writes fail immediately; simplest and safest. | ✓ |
| Forward to leader | Smoother client experience but requires internal forwarding, timeouts, and header/auth preservation. | |
| Queue locally until leader returns | Stronger apparent tolerance but risks ordering, duplicate writes, and recovery complexity. | |

**User's choice:** Fail fast with retryable error.
**Notes:** Planner should not implement follower write forwarding or local write queueing.

| Option | Description | Selected |
| --- | --- | --- |
| 503 + retry hint | Generic retryable outage during failover/no leader. | ✓ |
| 409/423 write fenced | More precise but exposes more implementation semantics. | |
| 200 read-only degraded response | Misleading for writes. | |

**User's choice:** 503 + retry hint.
**Notes:** Later security clarification requires this response to stay generic for ordinary users.

| Option | Description | Selected |
| --- | --- | --- |
| Allow reads with stale marker | Followers can serve reads, with lag/topology internally visible. | ✓ |
| Leader-only reads | Strong consistency but reduces multi-node value. | |
| Configurable per route | Flexible but expands Phase 12. | |

**User's choice:** Allow follower reads, but ordinary users must not see topology details.
**Notes:** Internal/admin surfaces may expose role and lag; ordinary data-plane responses must not.

| Option | Description | Selected |
| --- | --- | --- |
| TTL 10s + heartbeat 3s | Roadmap default; clear and testable. | ✓ |
| TTL 5s + heartbeat 1s | Faster failover, more false positives during Redis jitter. | |
| Configurable TTL/heartbeat | More flexible but adds config/test matrix. | |

**User's choice:** TTL 10s + heartbeat 3s.
**Notes:** Do not add Phase 12 config matrix for these intervals.

---

## Replication and Recovery Boundary

| Option | Description | Selected |
| --- | --- | --- |
| All repository writes under controlstate | Uniform write events, least likely to miss sync. | ✓ |
| Only security/config/accounting writes | Smaller scope but requires subjective selection. | |
| Only new Phase 12 metadata | Minimal but does not prove Plan 2 relational sync. | |

**User's choice:** All `controlstate` repository writes.
**Notes:** Planner should avoid guessing which repositories are important.

| Option | Description | Selected |
| --- | --- | --- |
| Time + stream distance dual metrics | More robust lag health signal. | ✓ |
| Time only | Simple but can miss backlog. | |
| Stream distance/pending only | Precise to Redis Stream but less intuitive on low traffic. | |

**User's choice:** Time + stream distance/pending.
**Notes:** Named constants should carry default thresholds.

| Option | Description | Selected |
| --- | --- | --- |
| Fallback log + bounded retry | Recoverable without infinite loops. | ✓ |
| Infinite retry until success | Can hang and amplify failures. | |
| Failure stops node | Strong safety, poor availability. | |

**User's choice:** Fallback log + bounded retry.
**Notes:** Exhausted retries should mark records failed for manual/later handling.

---

## Health and Readiness Surface

| Option | Description | Selected |
| --- | --- | --- |
| Role + node + leader + lag + writable | Sufficient for admin/test diagnosis. | ✓ |
| Minimal role + lag | Less useful for debugging. | |
| Full debug payload | Too heavy for ordinary health. | |

**User's choice:** Detailed fields are needed, but only internal/admin visible.
**Notes:** Fields include `node_id`, `role`, `leader_id`, `wal_lag`, `writable`, and `degraded_reason`.

| Option | Description | Selected |
| --- | --- | --- |
| Only when node cannot serve its role | Follower is healthy if it can safely serve reads. | ✓ |
| Any degraded state fails readiness | Too sensitive for deployment platforms. | |
| Always pass if process alive | Too weak for routing around bad nodes. | |

**User's choice:** Fail readiness only when the node cannot perform its role.
**Notes:** Redis coordination unknown, leader write inability, or follower unsafe read/lag should fail readiness.

---

## Multi-Node Test Scenarios

| Option | Description | Selected |
| --- | --- | --- |
| Main + likely abnormal | Covers high-value paths without full chaos matrix. | ✓ |
| Main path only | Too weak for requested abnormal coverage. | |
| Full chaos matrix | Too heavy for Phase 12. | |

**User's choice:** Main + likely abnormal.
**Notes:** Include 2-3 nodes, leader kill/failover, graceful shutdown, Redis outage/recovery, replica lag/degraded, and non-leader write rejection.

| Option | Description | Selected |
| --- | --- | --- |
| In-process multi-node harness | Fast and CI-friendly. | ✓ |
| Docker Compose integration test | More realistic but slower and heavier. | |
| Both | Better coverage but expands scope. | |

**User's choice:** In-process multi-node harness.
**Notes:** Multiple app/server instances in one Go test process, independent SQLite DSNs, shared Redis test instance or fake.

---

## Security Surface

| Option | Description | Selected |
| --- | --- | --- |
| Internal/admin-only endpoint | Keeps topology details behind internal/admin auth. | ✓ |
| Ordinary `/healthz` expands under admin token | Endpoint fewer, but higher leak risk. | |
| Logs/metrics only | Smaller surface, less useful for tests/operators. | |

**User's choice:** Internal/admin-only endpoint.
**Notes:** Ordinary users must never receive leader/follower/node/lag/failover/topology details.

## Agent Discretion

- The planner may name internal/admin routes and threshold constants.
- The planner may choose where debug-only stream offsets/retry counters live, provided ordinary user surfaces stay topology-blind.

## Deferred Ideas

- Docker Compose or full external multi-node deployment tests.
- Full chaos matrix.
- BFF/Admin Console topology UI.
- PostgreSQL/pgvector extension.
