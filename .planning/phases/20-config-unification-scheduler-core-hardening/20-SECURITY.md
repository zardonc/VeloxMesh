---
phase: 20
slug: config-unification-scheduler-core-hardening
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-06
---

# Phase 20 - Security

Per-phase security verification for config unification, scheduler execution hardening, and semantic-neighbor safeguards.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Operator config -> gateway runtime | ENV, main JSON, and component JSON files affect runtime subsystem wiring. | Runtime config, DSNs, component addresses, optional credentials |
| Redis coordination -> executor | Redis lock state controls whether a popped task may execute. | Task IDs, lock keys, lock TTLs |
| Queue state -> metrics | Queue admission and depth are exported to Prometheus. | Backend, priority, admission outcome, bounded queue depth |
| User request -> embedding provider | Request messages are transformed into embedding input. | Bounded embedding input text |
| Gateway startup -> Qdrant | Startup initializes vector collection state. | Collection name, vector dimension, startup errors |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-20-01-01 | Tampering | LoadConfig merge order | mitigate | `applyFileConfig` applies root/legacy/nested config then component files; config tests cover nested-over-flat and component override behavior. | closed |
| T-20-01-02 | Information Disclosure | examples and config structs | mitigate | Example files keep secrets empty or env-referenced; config tests assert `.env.example` does not contain known secret markers. | closed |
| T-20-01-03 | Denial of Service | validation | mitigate | Validation only requires enabled optional subsystem connection details; disabled Redis/Scheduler/cache profiles remain startable. | closed |
| T-20-02-01 | Tampering | Redis execution lock | mitigate | `RedisTaskLocker.Claim` uses `SET NX` with TTL and namespaced keys; `Executor.RunOne` skips execution/delivery on lock miss. | closed |
| T-20-02-02 | Information Disclosure | queue metrics | mitigate | Queue admission and lock-skip metrics use allowlisted low-cardinality labels only. | closed |
| T-20-02-03 | Denial of Service | executor slots | mitigate | `NewSynchronousRunnerWithConcurrency` clamps concurrency below 1 to one worker slot and preserves context cancellation paths. | closed |
| T-20-03-01 | Denial of Service | semantic-neighbor embedding | mitigate | `requestText` truncates by character count before building embedding requests; default cap is 16000. | closed |
| T-20-03-02 | Information Disclosure | truncation evidence | mitigate | Truncation records fixed reason `input_truncated`; Prometheus labels are allowlisted and do not include prompt text. | closed |
| T-20-03-03 | Denial of Service | app startup | mitigate | `newSemanticNeighborService` disables semantic neighbors fail-open on vector collection ensure failure and continues startup. | closed |

---

## Accepted Risks Log

No accepted risks.

---

## Evidence

| Area | Evidence |
|------|----------|
| Config precedence and examples | `internal/config/config_file.go`, `internal/config/config_validation.go`, `internal/config/config_test.go`, `config.json.example`, `config.scheduler.example.json`, `config.cache.example.json`, `.env.example` |
| Redis task locks | `internal/scheduler/queue_redis.go`, `internal/scheduler/executor.go`, `internal/scheduler/queue_redis_test.go`, `internal/scheduler/executor_test.go` |
| Queue metrics | `internal/scheduler/intake.go`, `internal/observability/prometheus.go`, `internal/observability/prometheus_test.go` |
| Semantic-neighbor safeguards | `internal/scheduler/semantic_neighbors.go`, `internal/app/semantic_neighbors.go`, `internal/storage/qdrant.go`, `internal/scheduler/semantic_neighbors_test.go`, `internal/app/app_test.go`, `internal/storage/qdrant_test.go` |
| Phase verification | `.planning/phases/20-config-unification-scheduler-core-hardening/20-VERIFICATION.md` reports `status: passed`, `gaps_found: false` |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-06 | 9 | 9 | 0 | Codex |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-07-06
