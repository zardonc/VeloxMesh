# Scheduler 1.0 Operator Runbook

## Purpose

Scheduler 1.0 is an optional scoring layer for queued gateway work. VeloxMesh still owns queueing, task state, execution, semantic lookup, SLA promotion, fallback, and all sensitive-payload boundaries. The Scheduler receives safe scalar features and returns scores or prediction metadata.

The default is off: `SCHEDULER_ENABLED=false`. When disabled or unhealthy, the gateway starts and forwards requests through its FIFO/fallback path.

## Deployment

Start with the gateway only:

```bash
cp .env.example .env
make run
```

Enable Scheduler only after the gateway path is healthy:

```env
SCHEDULER_ENABLED=true
SCHEDULER_MODE=heuristic
SCHEDULER_CONFIG_FILE=config.scheduler.example.json
```

For ONNX scoring, keep rollout at zero until the artifact and worker are healthy:

```env
SCHEDULER_MODE=onnx
SCHEDULER_ONNX_ENDPOINT=localhost:50051
SCHEDULER_ONNX_ARTIFACT_DIR=artifacts/scheduler-p70-v1
SCHEDULER_ONNX_ROLLOUT_PERCENT=0
```

Local proof commands:

```bash
go test -timeout 60s ./internal/scheduler
go test -timeout 60s ./cmd/scheduler ./internal/scheduler/onnx
```

## Monitoring

Prometheus scrape configuration lives in `docker/observability/prometheus.yml`.
Scheduler alert rules live in `docker/observability/scheduler-alerts.yml` and cover queue hard-limit rejection, soft-limit backpressure, ONNX MAPE/error rollout alerts, and an open scheduler circuit breaker.

No Grafana dashboard template is shipped for 1.0. Build dashboards from the shipped Prometheus metrics and alert rules when a deployment needs a visual console.

## Configuration

Use environment variables for local development and JSON files for structured deployments.

| Area | Env or JSON | Default |
| --- | --- | --- |
| Scheduler enabled | `SCHEDULER_ENABLED` / `scheduler.enabled` | `false` |
| Scheduler mode | `SCHEDULER_MODE` / `scheduler.mode` | `heuristic` |
| ONNX rollout | `SCHEDULER_ONNX_ROLLOUT_PERCENT` / `scheduler.onnx_rollout_percent` | `0` |
| Scheduler component file | `SCHEDULER_CONFIG_FILE` / `scheduler_config_file` | unset |
| Cache component file | `CACHE_CONFIG_FILE` / `cache_config_file` | unset |
| Heuristic overrides | `SCHEDULER_HEURISTIC_CONFIG_FILE` / `scheduler.heuristic_config_file` | unset |
| Scorer max concurrency | `SCHEDULER_SCORER_MAX_CONCURRENCY` / `scheduler.scorer_max_concurrency` | `4` |
| Scorer slow threshold | `SCHEDULER_SCORER_SLOW_THRESHOLD` / `scheduler.scorer_slow_threshold` | `scheduler.timeout` |
| Quality sample window | `SCHEDULER_QUALITY_SAMPLE_WINDOW` / `scheduler.quality_sample_window` | `100` |
| Queue backend | `SCHEDULER_QUEUE_BACKEND` / `scheduler.queue_backend` | `auto` |
| Queue soft limit | `SCHEDULER_QUEUE_SOFT_LIMIT` / `scheduler.queue_soft_limit` | `0` |
| Queue hard limit | `SCHEDULER_QUEUE_HARD_LIMIT` / `scheduler.queue_hard_limit` | `0` |
| Soft-limit wait | `SCHEDULER_QUEUE_POP_TIMEOUT` / `scheduler.queue_pop_timeout` | `100ms` |

Local scheduler-enabled example:

```env
SCHEDULER_ENABLED=true
SCHEDULER_MODE=heuristic
SCHEDULER_QUEUE_BACKEND=memory
SCHEDULER_EXECUTOR_CONCURRENCY=1
SCHEDULER_SCORER_MAX_CONCURRENCY=4
SCHEDULER_SCORER_SLOW_THRESHOLD=15ms
SCHEDULER_QUALITY_SAMPLE_WINDOW=100
```

Scorer backpressure rules:

- Scheduler scoring is an optimization path. Gateway intake must prefer quick fallback to heuristic/FIFO over waiting for an unhealthy predictor.
- `SCHEDULER_SCORER_MAX_CONCURRENCY` caps external scheduler/predictor calls. Requests above the cap return local fallback immediately.
- `SCHEDULER_SCORER_SLOW_THRESHOLD` treats slow successes as degraded and records them against the circuit breaker. Keep it at or below `SCHEDULER_TIMEOUT`.
- Keep `SCHEDULER_TIMEOUT=15ms` unless local evidence says otherwise. Treat 50-100ms as an absolute upper bound for unusual deployments, not as a default.
- ONNX MAPE and error-spike alerts use `SCHEDULER_QUALITY_SAMPLE_WINDOW`; one bad prediction no longer trips rollout alerts by itself.

Queue backend behavior:

- `auto`, empty, or `memory` uses the in-memory queue. This is the default and is the preferred single-node path.
- `redis` uses Redis only when Redis is enabled and reachable. The queue key is node-scoped as `scheduler_queue:gateway-<node_id>` under the configured Redis namespace.
- Explicit Redis queueing is for high-concurrency single-node bursts or future extension work. It is not a cross-node task-stealing queue.
- `FallbackQueue` now retries primary writes after a primary failure, merges primary and memory fallback reads, and falls back to memory when primary returns empty.

Queue limit behavior:

- `queue_soft_limit=0` disables soft-limit backpressure.
- `queue_hard_limit=0` disables hard-limit rejection.
- When `soft <= depth < hard`, high-priority requests bypass the soft limit.
- Normal and low-priority requests wait once for `queue_pop_timeout`, then retry admission.
- If the queue is still at the soft limit after that wait, the gateway returns `429 scheduler_backpressure`.
- If the queue reaches the hard limit, the gateway returns `503 scheduler_queue_full`.

Semantic-neighbor example:

```env
SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED=true
SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL=text-embedding-3-small
CACHE_CONFIG_FILE=config.cache.example.json
```

## Degradation Playbooks

| Scenario | Expected behavior | Proof command |
| --- | --- | --- |
| Scheduler down | Gateway falls back instead of blocking forwarding. | `go test -timeout 60s ./internal/scheduler -run TestSchedulerClient` |
| Redis unavailable | Default queueing stays in memory. Explicit Redis queueing falls back to memory at startup if Redis is unreachable. | `go test -timeout 60s ./internal/app -run TestNewSchedulerQueue` |
| Redis recovers after fallback writes | `FallbackQueue` retries primary writes and reads memory fallback entries when primary is empty, avoiding stranded fallback tasks. | `go test -timeout 60s ./internal/scheduler -run TestFallbackQueue` |
| Memory queue node crash | In-memory pending tasks are non-durable; use Redis queueing when pending work must survive a node restart. | `go test -timeout 60s ./internal/gateway -run TestService_HandleChatCompletionSchedulerQueueUnavailable` |
| Qdrant unavailable | Semantic neighbors fail open and keep default feature values. | `go test -timeout 60s ./internal/scheduler -run TestSemanticNeighbor` |
| ONNX predictor unhealthy | Predictive path falls back to heuristic scoring. | `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/predictive` |
| Admin API validation failure | Invalid admin writes return 400 and preserve current runtime state. | `go test -timeout 60s ./internal/http/handlers -run TestAdminScheduler` |

## Vector Backends

Semantic-neighbor lookup is gateway-owned for both backends; scheduler receives only aggregate scalar features.

| Backend | Best fit | Config location | UAT command |
| --- | --- | --- | --- |
| LanceDB | Plan 3 embedded single-node deployments. It is the default when no vector store is configured, but this build may degrade to noop unless compiled with LanceDB support. | unset `cache.vector_store`, or set `cache.vector_store="lancedb"` for an explicit degraded warning when unavailable. | LanceDB runtime validation is deferred until the development environment supports it. |
| Qdrant | Plan 1/2 service-backed vector store and Plan 3 substitute when explicitly configured. | `cache.vector_store="qdrant"` and `cache.qdrant.addr`. | `go test -timeout 60s ./tests/integration -run TestSemanticCache -count=1` |
| pgvector | Plan 4 PostgreSQL deployments that want relational and vector data together. | `cache.vector_store="pgvector"` with the Plan 4 PostgreSQL DSN in control-state config. | `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1` |

Plan 1 stays `App + SQLite + Redis Stack + Qdrant`. Redis remains part of Plan 1 for hot state, rate/config coordination, aggregation paths, and high-concurrency extension room.

Plan 3 is single-node `App + SQLite + LanceDB/Qdrant`. It defaults to LanceDB when vector-store config is absent. Set Qdrant explicitly when LanceDB is not available or when service-backed vector operations are preferred. LanceDB and Qdrant data are mutually exclusive in v7.7; no migration or shared-read path is provided.

Qdrant example:

```json
{
  "cache": {
    "enabled": true,
    "vector_store": "qdrant",
    "vector_dimension": 1536,
    "qdrant": { "addr": "localhost:6334" }
  }
}
```

Plan 3 Qdrant substitute example:

```json
{
  "control_state": {
    "backend": "sqlite",
    "dsn": "file:veloxmesh.db?cache=shared"
  },
  "cache": {
    "enabled": true,
    "vector_store": "qdrant",
    "vector_dimension": 1536,
    "qdrant": { "addr": "localhost:6334" }
  },
  "scheduler": {
    "enabled": true,
    "queue_backend": "memory"
  }
}
```

pgvector example:

```json
{
  "control_state": {
    "backend": "postgres",
    "dsn": "${POSTGRES_TEST_DSN}"
  },
  "cache": {
    "enabled": true,
    "vector_store": "pgvector",
    "vector_dimension": 1536
  }
}
```

## Admin APIs

Use the existing admin protections for these routes. Send `Authorization: Bearer <ADMIN_API_KEY>`.

| Route | Purpose |
| --- | --- |
| `GET /admin/v1/scheduler/status` | Queue depth, executor slots, breaker state, warnings, and quality rollups. |
| `GET /admin/v1/scheduler/sla-rules` | Read active runtime SLA promotion rules. |
| `PUT /admin/v1/scheduler/sla-rules` | Replace the in-memory rule set after validation. |
| `GET /admin/v1/scheduler/training-samples/export` | Export safe training features and labels as JSON or NDJSON. |
| `PATCH /admin/scheduler/rollout` | Roll ONNX traffic back to heuristic by setting rollout to `0`, or update `quality_sample_window`. |

Admin changes to ONNX rollout, quality sample window, and SLA promotion rules affect the running process only. Put durable values back into `config.scheduler.example.json`, the deployment config file, or environment management before restart.

SLA rule replacement body:

```json
{
  "rules": [
    {
      "policy_id": "gold-code",
      "tenant_class": "gold",
      "model_class": "standard",
      "request_kind": "code_gen",
      "wait_threshold": "2s"
    }
  ]
}
```

## UAT

Run local automated evidence first:

```bash
go test -timeout 60s ./internal/config
go test -timeout 60s ./internal/app ./internal/http/handlers ./internal/scheduler
```

Current v7.7 verification:

```bash
go test -timeout 60s ./...
go build ./...
```

Privacy contract checks for scheduler DTO/protobuf field names are included in `go test -timeout 60s ./internal/scheduler -run TestSchedulerPrivacyContractFieldNames`.

Run gated service checks only when their required environment variables are present. Real-provider UAT uses `.env.local`; prefer non-Gemini provider resources for routine checks and reserve Gemini resources for Gemini-specific scenarios.
