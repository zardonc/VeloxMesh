# VeloxMesh single-host Docker deployment

This guide is for a host that only has Docker and Docker Compose installed.
The deployment branch is currently `main`.

## Prerequisites

- Docker Engine or Docker Desktop with Docker Compose v2.
- Network access from the host to the selected upstream model provider.
- A provider API key stored in a local env file, not in Git.
- An ONNX scheduler artifact at `deploy/models/current/model.onnx` plus `deploy/models/current/manifest.json` when running ONNX mode.

## Directory layout

```text
deploy/
  compose/veloxmesh.yml          Main single-host Compose file
  env/*.example.env              Copy to *.env; local secrets stay ignored
  config/*.example.json          Safe application, scheduler, and cache examples
  models/                        Local ONNX artifacts; ignored except README
  observability/                 Prometheus, Grafana, Promtail, and OTel config
  reports/                       Benchmark output; ignored
  data/                          Local SQLite and runtime data; ignored
```

Root-level `.env.example`, `.env.postgres.example`, and `config.*.example.json`
remain as compatibility examples. New deployments should prefer `deploy/`.

## Get the code

Preferred path when Git is available:

```bash
git clone --branch main <your-repo-url> VeloxMesh
cd VeloxMesh
```

If Git is not installed on the host, let Docker download the `main` branch
during image build:

```bash
export VELOXMESH_BUILD_CONTEXT=https://github.com/your-org/VeloxMesh.git#main
```

You still need this `deploy/` directory on the host, either from a release
bundle or any copied checkout. The build context above tells Docker where to
download the application source from.

## Prepare local configuration

Full stack with Redis and Qdrant:

```bash
cp deploy/env/full.example.env deploy/env/full.env
cp deploy/config/app.full.example.json deploy/config/app.full.json
cp deploy/config/scheduler.full.example.json deploy/config/scheduler.full.json
cp deploy/config/cache.full.example.json deploy/config/cache.full.json
```

Simple stack without Redis and Qdrant:

```bash
cp deploy/env/simple.example.env deploy/env/simple.env
cp deploy/config/app.simple.example.json deploy/config/app.simple.json
cp deploy/config/scheduler.simple.example.json deploy/config/scheduler.simple.json
cp deploy/config/cache.simple.example.json deploy/config/cache.simple.json
```

PostgreSQL profile:

```bash
cp deploy/env/postgres.example.env deploy/env/postgres.env
```

ONNX versus heuristic comparison:

```bash
cp deploy/env/compare.example.env deploy/env/compare.env
cp deploy/config/app.compare.example.json deploy/config/app.compare.json
cp deploy/config/scheduler.compare.example.json deploy/config/scheduler.compare.json
cp deploy/config/cache.compare.example.json deploy/config/cache.compare.json
```

Then edit the copied `.env` and `.json` files and replace every `replace-with-*`
value plus the example provider URL/model names. Do not edit the `*.example.*`
files with real secrets or real account details.

For ONNX mode, copy or publish model files to:

```text
deploy/models/current/model.onnx
deploy/models/current/manifest.json
```

## Start the services

One-command path from the repository root:

```bash
sh deploy/scripts/veloxmesh-up.sh simple
```

Supported modes:

```text
simple    Gateway + ONNX scheduler + observability, no Redis/Qdrant
full      simple + Redis/RedisInsight + Qdrant
compare   ONNX scheduler and heuristic scheduler side by side
postgres  full + PostgreSQL/pgvector + Adminer
```

The script copies any missing local config files from their examples. If it
created files, it stops before Docker Compose starts. Edit the generated
`deploy/env/*.env` and `deploy/config/*.json` files, then run the same command
again.

Simple ONNX scheduler, no Redis/Qdrant:

```bash
docker compose --env-file deploy/env/simple.env -f deploy/compose/veloxmesh.yml --profile simple up -d --build
```

Full local stack with Redis and Qdrant:

```bash
docker compose --env-file deploy/env/full.env -f deploy/compose/veloxmesh.yml --profile full up -d --build
```

Add PostgreSQL/pgvector and Adminer:

```bash
docker compose --env-file deploy/env/full.env --env-file deploy/env/postgres.env -f deploy/compose/veloxmesh.yml --profile full --profile postgres up -d --build
```

ONNX versus heuristic comparison:

```bash
docker compose --env-file deploy/env/compare.env -f deploy/compose/veloxmesh.yml --profile compare up -d --build
```

The `simple` and `full` profiles run the ONNX scheduler only. The `compare`
profile is the only profile that starts `scheduler-heuristic`; it also uses
`deploy/observability/prometheus.compare.yml` so Prometheus does not scrape a
missing heuristic service in normal deployments.

Build images directly from the remote `main` branch without a local source
checkout:

```bash
docker build -f Dockerfile -t veloxmesh-go:main https://github.com/your-org/VeloxMesh.git#main
docker build -f docker/onnx-worker.Dockerfile -t veloxmesh-onnx-worker:main https://github.com/your-org/VeloxMesh.git#main
```

Or use the explicit remote-build Dockerfiles from a copied deploy bundle:

```bash
docker build -f docker/remote-build.Dockerfile \
  --build-arg VELOXMESH_REPO_URL=https://github.com/your-org/VeloxMesh.git \
  --build-arg VELOXMESH_BRANCH=main \
  -t veloxmesh-go:main .

docker build -f docker/onnx-worker.remote-build.Dockerfile \
  --build-arg VELOXMESH_REPO_URL=https://github.com/your-org/VeloxMesh.git \
  --build-arg VELOXMESH_BRANCH=main \
  -t veloxmesh-onnx-worker:main .
```

## Verify the deployment

```bash
docker compose -f deploy/compose/veloxmesh.yml ps
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:9090/-/ready
curl http://localhost:3000/api/health
```

Scheduler checks:

```bash
docker compose -f deploy/compose/veloxmesh.yml logs --tail=100 scheduler-onnx
docker compose -f deploy/compose/veloxmesh.yml exec scheduler-onnx wget -qO- http://localhost:9091/status
docker compose -f deploy/compose/veloxmesh.yml exec scheduler-onnx wget -qO- http://localhost:9091/metrics
```

Gateway model list:

```bash
curl http://localhost:8080/v1/models -H "Authorization: Bearer <DEV_API_KEY>"
```

## Data service management

| Service | URL | Use |
| --- | --- | --- |
| RedisInsight | `http://localhost:5540` | Inspect Redis keys, queues, and hot state. |
| Qdrant dashboard | `http://localhost:6333/dashboard` | Inspect collections and vector points. |
| Adminer | `http://localhost:8082` | Inspect PostgreSQL when the `postgres` profile is enabled. |
| Prometheus | `http://localhost:9090` | Query raw metrics and alert rules. |
| Grafana | `http://localhost:3000` | View dashboards with Prometheus and Loki datasources. |
| cAdvisor | `http://localhost:8083` | Inspect container CPU and memory. |

## Logs and troubleshooting

Application logs are JSON on stdout. Use Docker logs first:

```bash
docker compose -f deploy/compose/veloxmesh.yml logs -f gateway scheduler-onnx onnx-worker
```

Grafana also has Loki wired as a datasource when Promtail can read Docker
container logs. If Promtail cannot read `/var/lib/docker/containers` on Docker
Desktop, keep using `docker compose logs`.

Common checks:

- `scheduler-onnx /status` should report `ready` for a valid ONNX worker and manifest.
- `gateway_scheduler_errors_total` should not grow during normal traffic.
- `gateway_circuit_breaker_state{state="open"}` should stay at `0`.
- `scheduler_scoring_errors_total` should not grow after startup.
- Container CPU and memory should be checked in cAdvisor during benchmark runs.

## Benchmark flow

Put datasets under `datasets/` or mount them into a one-off benchmark runner.
Write reports under `deploy/reports/<run-id>/`.

Suggested output files:

```text
deploy/reports/<run-id>/
  summary.md
  summary.json
  samples.failed.jsonl
  latency.csv
  resources.csv
```

Minimum report sections:

- Run metadata: git SHA, image tags, compose profile, model version, dataset name.
- Quality: accuracy, failed samples, timeout rate, error rate.
- Performance: total requests, average latency, p50, p95, p99, throughput.
- Scheduler: backend, version, calls, errors, p95 call latency, MAPE, anomaly status.
- Resources: per-container average/max CPU and memory.
- Failed samples: sample id, reason, latency, error.

For scheduler comparison, run the same dataset once with `onnx_rollout_percent=100`
and once with `onnx_rollout_percent=0`, or use the `compare` profile to keep both
scheduler services up while routing through the configured rollout value.

## Stop and clean up

Stop services:

```bash
sh deploy/scripts/veloxmesh-down.sh
```

Remove local volumes only when you intentionally want to delete local data:

```bash
sh deploy/scripts/veloxmesh-down.sh -v
```

## Configuration safety

- Commit only `*.example.env` and `*.example.json`.
- Do not commit `deploy/env/*.env`, `deploy/data/`, `deploy/reports/`, or model artifacts.
- Keep provider keys in env files and reference them from JSON through `auth.api_key_env`.
- Replace placeholder admin, Grafana, Qdrant, PostgreSQL, and encryption values before use.
