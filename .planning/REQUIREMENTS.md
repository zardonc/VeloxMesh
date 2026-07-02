# Requirements: VeloxMesh

**Defined:** 2026-07-02
**Core Value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## v7.2 Requirements

### Coordination

- [ ] **COORD-01**: Operators can run multiple gateway nodes with Redis-backed node registration and role discovery.
- [ ] **COORD-02**: Gateway nodes can elect a single leader with TTL-based Redis locks and heartbeat renewal.
- [ ] **COORD-03**: Gateway nodes release leadership cleanly during graceful shutdown.

### SQLite Replication

- [ ] **REPL-01**: Replica nodes can consume a Redis Stream of SQLite relational changes from the leader.
- [ ] **REPL-02**: Replica nodes can report WAL lag so operators can detect stale nodes.
- [ ] **REPL-03**: Vector data stays outside WAL replication because Qdrant owns vector storage and replication.

### Write Fencing

- [ ] **FENCE-01**: Only the active leader can perform SQLite relational writes in multi-node mode.
- [ ] **FENCE-02**: Non-leader write attempts fail with a clear retryable error instead of silently diverging state.

### Recovery

- [ ] **RECOV-01**: Failed relational sync operations are recorded in the fallback log.
- [ ] **RECOV-02**: A recovery worker can replay failed relational sync operations after Redis or replica disruption.

### Health and Verification

- [ ] **HLTH-01**: Health output includes node role, node identity, leader identity, and WAL lag.
- [ ] **TEST-01**: Chaos tests cover leader loss, node shutdown, and Redis/network disruption without corrupting SQLite state.

## Future Requirements

### Admin Console

- **ADMIN-01**: BFF/Admin Console can display cluster topology after Phase 11 exists.

### PostgreSQL Extension

- **PG-01**: PostgreSQL and pgvector adapters can replace SQLite/Qdrant paths for enterprise deployments after Phase 12.
- **PG-02**: SQLite to PostgreSQL migration tooling can move relational state after Phase 12 stabilizes write boundaries.

## Out of Scope

| Feature | Reason |
| --- | --- |
| BFF/Admin Console UI | Phase 11 is intentionally skipped for now. Phase 12 exposes runtime health/topology data only. |
| PostgreSQL/pgvector implementation | Phase 13 depends on Phase 12 coordination and recovery boundaries. |
| Vector WAL replication | Qdrant owns vector storage and replication for Plans 1/2. |
| Redis as source of truth | SQLite remains authoritative for relational state. Redis coordinates hot state and replication transport. |

## Traceability

| Requirement | Phase | Status |
| --- | --- | --- |
| COORD-01 | Phase 12 | Pending |
| COORD-02 | Phase 12 | Pending |
| COORD-03 | Phase 12 | Pending |
| REPL-01 | Phase 12 | Pending |
| REPL-02 | Phase 12 | Pending |
| REPL-03 | Phase 12 | Pending |
| FENCE-01 | Phase 12 | Pending |
| FENCE-02 | Phase 12 | Pending |
| RECOV-01 | Phase 12 | Pending |
| RECOV-02 | Phase 12 | Pending |
| HLTH-01 | Phase 12 | Pending |
| TEST-01 | Phase 12 | Pending |

**Coverage:**
- v7.2 requirements: 12 total
- Mapped to phases: 12
- Unmapped: 0

---
*Requirements defined: 2026-07-02*
*Last updated: 2026-07-02 after starting v7.2 Multi-Node Coordination*
