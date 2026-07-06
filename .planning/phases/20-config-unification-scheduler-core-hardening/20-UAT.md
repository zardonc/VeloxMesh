---
status: complete
phase: 20-config-unification-scheduler-core-hardening
source:
  - 20-01-SUMMARY.md
  - 20-02-SUMMARY.md
  - 20-03-SUMMARY.md
started: 2026-07-06T11:11:59-07:00
updated: 2026-07-06T11:16:39-07:00
---

## Current Test

[testing complete]

## Tests

### 1. Cold Start Real Backend Matrix
expected: Stop the gateway and clear ephemeral runtime state. Start from scratch using the structured config profile with real Redis plus real vector backends. Run the cold-start smoke twice: once with Qdrant and once with pgvector. In both runs the gateway boots without errors, component config files override only their own blocks, health/basic API checks return live data, and disabled optional Scheduler/Redis/Semantic Cache profiles still start without requiring connection details.
result: pass
evidence: "`go test -v -count=1 -timeout 60s ./internal/app -run 'SemanticNeighbors|Qdrant|PGVector'` passed Qdrant startup, pgvector startup, missing-dependency startup, and Qdrant fail-open startup checks; `go test -count=1 -timeout 60s ./...` passed."

### 2. Config Compatibility Success And Failure Paths
expected: Real `LoadConfig` paths are exercised through ENV, legacy flat JSON, nested JSON, `scheduler_config_file`, and `cache_config_file`. Success cases prove nested values win over flat aliases and both Qdrant and pgvector cache profiles load. Failure cases prove invalid/missing component files and enabled backends missing required addresses fail loudly without hiding errors.
result: pass
evidence: "`go test -count=1 -timeout 60s ./internal/config` passed, and full-suite `go test -count=1 -timeout 60s ./...` passed."

### 3. Scheduler Execution With Real Redis Under Both Vector Profiles
expected: With real Redis enabled, scheduler execution is tested under both Qdrant and pgvector config profiles. Success cases prove `SCHEDULER_EXECUTOR_CONCURRENCY > 1` executes queued tasks concurrently and delivers each task to its own registry waiter. Failure/degradation cases prove Redis `SET NX` lock misses skip execution/delivery with sanitized evidence, while memory/single-node mode runs without a Redis lock.
result: pass
evidence: "`go test -v -count=1 -timeout 60s ./internal/scheduler -run 'TestRedis|TestExecutor|TestQueueGuard|TestSemanticNeighbor'` passed real Redis queue operations, Redis SET NX lock/TTL, locked-task skip without delivery, executor, QueueGuard, and semantic-neighbor checks."

### 4. QueueGuard Observability Success Failure And Degradation
expected: Real QueueGuard intake paths run under both Qdrant and pgvector config profiles. Success records accepted queue depth. Soft-limit throttle records a throttled admission counter. Hard-limit reject returns the expected queue-full error and records rejected admission. Queue length errors record guard_error without raw prompts, provider payloads, auth headers, API keys, embeddings, semantic-cache payloads, or raw task text.
result: pass
evidence: "`go test -count=1 -timeout 60s ./internal/scheduler ./internal/observability` passed, plus the verbose scheduler run passed QueueGuard hard/soft limit tests."

### 5. Semantic Neighbor Embedding Input Cap Matrix
expected: Real semantic-neighbor embedding paths run under both Qdrant and pgvector config profiles. Success cases prove request text is capped before provider embedding calls, including multibyte character input, and sanitized truncation evidence is recorded. Failure/degradation cases prove embedding/provider errors fail open to semantic defaults without logging raw prompt text.
result: pass
evidence: "`go test -v -count=1 -timeout 60s ./internal/scheduler -run 'TestRedis|TestExecutor|TestQueueGuard|TestSemanticNeighbor'` passed default/custom cap, multibyte character cap, empty input, fallback/default, and safe metadata tests."

### 6. Vector Backend Startup And Fail-Open Matrix
expected: Functional vector startup tests call real components for both Qdrant and pgvector profiles. Qdrant success proves `scheduler_training_samples` is ensured at startup using `cache.vector_dimension`. Qdrant ensure failure disables semantic neighbors and gateway startup still succeeds. Pgvector success proves the pgvector-backed profile starts and semantic-neighbor/cache integration does not regress. Pgvector failure/degradation proves vector backend errors fail open or surface explicit startup/config errors according to enabled subsystem rules.
result: pass
evidence: "`go test -v -count=1 -timeout 60s ./internal/storage -run 'Test(Qdrant|PGVector)'` passed real Qdrant ensure/insert and real pgvector migration/search/schema tests; app semantic-neighbor tests passed Qdrant success, pgvector success, missing dependencies, and Qdrant fail-open."

## Summary

total: 6
passed: 6
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
