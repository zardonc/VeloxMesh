---
phase: 23
slug: scheduler-queue-hardening
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-08
verified: 2026-07-08
register_authored_at_plan_time: false
---

# Phase 23 - Security

Retroactive STRIDE register for Scheduler queue hardening.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Gateway process -> Redis Scheduler queue | Optional Redis queue stores queued task ordering data. | Task ID and score only |
| Gateway process -> memory fallback queue | Local fallback queue receives tasks when Redis is unavailable. | Task ID, score, safe scheduler feature snapshot |
| Multiple gateway nodes -> Redis lock keys | Nodes coordinate task ownership through Redis locks. | Task ID lock marker with TTL |
| Operator config -> queue backend selection | Env/JSON config selects memory or explicit Redis queueing. | Queue backend, Redis addr/namespace, node ID |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-23-01 | Information Disclosure | RedisQueue | mitigate | `RedisQueue.Push` writes only `QueueItem.TaskID` as the ZSET member and score as ordering data; `TestRedisQueueStoresOnlyTaskIDMember` and `TestSchedulerPrivacyContractFieldNames` verify scheduler fields exclude prompt/message/api_key/authorization/secret/embedding/payload/raw terms. | closed |
| T-23-02 | Tampering | RedisTaskLocker | mitigate | `RedisTaskLocker.Claim` uses Redis `SET NX` with `redisTaskLockTTL`; `TestRedisTaskLockerUsesSetNXAndTTL` verifies duplicate claims fail and TTL is set. | closed |
| T-23-03 | Repudiation | Scheduler execution | mitigate | `TestExecutorSkipsRedisLockedTaskWithoutDelivery` verifies an already-locked task is not delivered to the handler, preserving a deterministic ownership boundary. | closed |
| T-23-04 | Denial of Service | Queue backend selection | mitigate | `newSchedulerQueue` defaults to memory unless Redis is explicit and reachable; `TestApp_SchedulerRedisQueueFailureUsesMemory` and `TestNewSchedulerQueueDefaultsToMemoryWhenRedisIsEnabled` verify fail-open startup. | closed |
| T-23-05 | Tampering | Cross-node Redis queue naming | mitigate | `schedulerRedisQueueName` scopes the Redis queue to `gateway-<nodeID>` with `local` fallback; `TestNewSchedulerQueueExplicitRedisIsNodeScoped` verifies node scoping. | closed |
| T-23-06 | Denial of Service | FallbackQueue recovery | mitigate | `FallbackQueue` merges primary and memory fallback reads after primary recovery; `TestFallbackQueuePopMinReadsMemoryWhenPrimaryEmptyAfterRecovery`, `TestFallbackQueuePopMinMergesPrimaryAndFallback`, and real Redis `TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure` verify fallback tasks are not stranded. | closed |

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-23-01 | T-23-01 | Real Redis validation ran against the local `.env` service and may use plaintext Redis transport. This is acceptable for local validation only; production transport hardening remains an operator deployment concern. | project owner via v7.7 local validation scope | 2026-07-08 |

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-08 | 6 | 6 | 0 | Codex inline retroactive STRIDE |

## Verification Evidence

- `go test -v -timeout 60s ./internal/scheduler -run 'TestSchedulerPrivacyContractFieldNames|TestRedisQueueStoresOnlyTaskIDMember|TestRedisTaskLockerUsesSetNXAndTTL|TestExecutorSkipsRedisLockedTaskWithoutDelivery|TestSemanticNeighborIndexerWritesSafeMetadata' -count=1` passed.
- `go test -v -timeout 60s ./internal/scheduler -run 'TestRedisQueue|TestRedisTaskLocker|TestExecutorSkipsRedisLockedTask|TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure' -count=1` passed against real Redis.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.

## Sign-Off

- [x] All threats have a disposition.
- [x] Accepted risks documented in Accepted Risks Log.
- [x] `threats_open: 0` confirmed.
- [x] `status: verified` set in frontmatter.

Approval: verified 2026-07-08

