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
| Queue soft limit | `SCHEDULER_QUEUE_SOFT_LIMIT` / `scheduler.queue_soft_limit` | `0` |
| Queue hard limit | `SCHEDULER_QUEUE_HARD_LIMIT` / `scheduler.queue_hard_limit` | `0` |
| Soft-limit wait | `SCHEDULER_QUEUE_POP_TIMEOUT` / `scheduler.queue_pop_timeout` | `100ms` |

Local scheduler-enabled example:

```env
SCHEDULER_ENABLED=true
SCHEDULER_MODE=heuristic
SCHEDULER_QUEUE_BACKEND=auto
SCHEDULER_EXECUTOR_CONCURRENCY=1
```

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
| Redis unavailable | Queue path degrades to memory where configured. Redis-queued tasks from before the failure are not recovered from memory fallback. | `go test -timeout 60s ./internal/scheduler -run TestFallbackQueue` |
| Memory queue node crash | In-memory pending tasks are non-durable; use Redis queueing when pending work must survive a node restart. | `go test -timeout 60s ./internal/gateway -run TestService_HandleChatCompletionSchedulerQueueUnavailable` |
| Qdrant unavailable | Semantic neighbors fail open and keep default feature values. | `go test -timeout 60s ./internal/scheduler -run TestSemanticNeighbor` |
| ONNX predictor unhealthy | Predictive path falls back to heuristic scoring. | `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/predictive` |
| Admin API validation failure | Invalid admin writes return 400 and preserve current runtime state. | `go test -timeout 60s ./internal/http/handlers -run TestAdminScheduler` |

## Vector Backends

Semantic-neighbor lookup is gateway-owned for both backends; scheduler receives only aggregate scalar features.

| Backend | Best fit | Config location | UAT command |
| --- | --- | --- | --- |
| Qdrant | Default vector service for local and multi-node deployments. | `cache.vector_store="qdrant"` and `cache.qdrant.addr`. | `go test -timeout 60s ./tests/integration -run TestSemanticCache -count=1` |
| pgvector | Plan 4 PostgreSQL deployments that want relational and vector data together. | `cache.vector_store="pgvector"` with the Plan 4 PostgreSQL DSN in control-state config. | `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1` |

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
| `PATCH /admin/scheduler/rollout` | Roll ONNX traffic back to heuristic by setting rollout to `0`. |

Admin changes to ONNX rollout and SLA promotion rules affect the running process only. Put durable values back into `config.scheduler.example.json`, the deployment config file, or environment management before restart.

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
go test -timeout 60s ./internal/http/handlers ./internal/scheduler
```

Privacy contract checks for scheduler DTO/protobuf field names are included in `go test -timeout 60s ./internal/scheduler -run TestSchedulerPrivacyContractFieldNames`.

Run gated service checks only when their required environment variables are present. Real-provider UAT uses `.env.local`; prefer non-Gemini provider resources for routine checks and reserve Gemini resources for Gemini-specific scenarios.
