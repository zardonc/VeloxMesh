# VeloxMesh single-host Docker deployment

This guide is for a host that only has Docker and Docker Compose installed.
The deployment branch is currently `main`.

## One-command install

Use this path when the server does not have a local checkout:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sudo sh
```

Choose a profile:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sudo sh -s -- --profile full
```

The installer creates `./VeloxMesh` under the directory where you run the
command, downloads the deployment files, uses the GitHub repository as the
Docker build context, generates local config, and starts Docker Compose.
Existing local env/config files are kept on rerun for the same profile. To
change profiles, use a different `--install-dir`, edit `env/veloxmesh.env`, or
uninstall first. Override the location with `--install-dir` or
`VELOXMESH_INSTALL_DIR` when needed.

If omitted, the installer generates random values for `DEV_API_KEY`,
`ADMIN_API_KEY`, `GRAFANA_ADMIN_PASSWORD`, `POSTGRES_PASSWORD`, and the
control-state encryption key. It does not generate a real provider API key;
pass `--provider-api-key` or edit `env/veloxmesh.env` before making live model
calls.

## One-command uninstall

Uninstall the default `./VeloxMesh` installation from the directory where you
run the command:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sudo sh -s -- --yes
```

Remove Docker Compose named volumes too:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sudo sh -s -- --yes --volumes
```

Use `--install-dir` or `VELOXMESH_INSTALL_DIR` if the install path was changed.
If Docker Compose is unavailable or shutdown fails, the uninstall script stops
without deleting local files.

## Prerequisites

- Docker Engine or Docker Desktop with Docker Compose v2.
- Network access from the host to the selected upstream model provider.
- A provider API key stored in a local env file, not in Git.
- Optional custom ONNX scheduler artifact at `deploy/models/current/model.onnx` plus `deploy/models/current/manifest.json`.
  If either file is missing, the ONNX worker creates a default scheduler artifact at startup.

## Directory layout

```text
deploy/
  compose/veloxmesh.yml          Main single-host Compose file
  env/*.example.env              Copy to *.env; local secrets stay ignored
  config/*.example.json          Safe application, scheduler, and cache examples
  config/pipeline.example.yaml   Shared input/output pipeline example
  models/                        Local ONNX artifacts; ignored except README
  observability/                 Prometheus, Grafana, Promtail, and OTel config
  reports/                       Benchmark output; ignored
  data/                          Local SQLite and runtime data; ignored
```

All deployment Dockerfiles, Compose files, env examples, and config examples live
under `deploy/` so local deployment state has one home.

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

Shared pipeline configuration:

```bash
cp deploy/config/pipeline.example.yaml deploy/config/pipeline.yaml
```

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
cp deploy/config/app.postgres.example.json deploy/config/app.postgres.json
cp deploy/config/cache.postgres.example.json deploy/config/cache.postgres.json
```

ONNX versus heuristic comparison:

```bash
cp deploy/env/compare.example.env deploy/env/compare.env
cp deploy/config/app.compare.example.json deploy/config/app.compare.json
cp deploy/config/scheduler.compare.example.json deploy/config/scheduler.compare.json
cp deploy/config/cache.compare.example.json deploy/config/cache.compare.json
```

Then edit the copied `.env`, `.json`, and `.yaml` files and replace every `replace-with-*`
value plus the example provider URL/model names. Do not edit the `*.example.*`
files with real secrets or real account details.

For ONNX mode, copy or publish custom model files to:

```text
deploy/models/current/model.onnx
deploy/models/current/manifest.json
```

If these files do not exist, `onnx-worker` creates a default local artifact in
the same directory before starting.

## Provider, routing, and combo examples

Configure multiple models on one provider by listing them in `models` and
choosing one `default_model`:

```json
{
  "id": "openai-primary",
  "type": "openai-compatible",
  "base_url": "https://api.example.invalid/v1",
  "auth": {"api_key_env": "OPENAI_PRIMARY_API_KEY"},
  "models": ["gpt-4o-mini", "gpt-4o"],
  "default_model": "gpt-4o-mini",
  "timeout": "30s"
}
```

Add multiple providers by adding entries under `providers`. Providers that
serve the same model become routing candidates for that model:

```json
{
  "routing_strategy": "least-latency",
  "default_provider": "openai-primary",
  "fallback_enabled": true,
  "max_attempts": 2,
  "providers": [
    {
      "id": "openai-primary",
      "type": "openai-compatible",
      "base_url": "https://api.example.invalid/v1",
      "auth": {"api_key_env": "OPENAI_PRIMARY_API_KEY"},
      "models": ["gpt-4o-mini"],
      "default_model": "gpt-4o-mini"
    },
    {
      "id": "openai-backup",
      "type": "openai-compatible",
      "base_url": "https://backup.example.invalid/v1",
      "auth": {"api_key_env": "OPENAI_PRIMARY_API_KEY"},
      "models": ["gpt-4o-mini"],
      "default_model": "gpt-4o-mini"
    }
  ]
}
```

Provider routing controls:

```json
{
  "routing_strategy": "least-latency",
  "fallback_enabled": true,
  "max_attempts": 2
}
```

Use `routing_strategy: "round-robin"` for simple rotation, `"least-latency"`
for health-latency selection, or durable routing config with
`"composite-score"` when the control-state repository is enabled. For one
request, force a provider with:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <DEV_API_KEY>" \
  -H "X-Route-To: openai-backup" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
```

Create a combo with the admin API:

```bash
curl -X POST http://localhost:8081/admin/v1/combos \
  -H "Authorization: Bearer <ADMIN_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "fast-chat",
    "name": "fast-chat",
    "enabled": true,
    "strategy": "round-robin",
    "members": ["gpt-4o-mini", "gpt-4o"]
  }'
```

Combo routing strategies:

```json
{"strategy": "round-robin"}
{"strategy": "capacity-auto-switch"}
{"strategy": "fusion", "judge": "gpt-4o"}
```

Choose combo versus single-provider routing per request:

```json
{"model": "fast-chat"}
```

uses the combo named `fast-chat`.

```json
{"model": "gpt-4o-mini"}
```

uses the normal provider routing pool for that model. Add `X-Route-To:
<provider-id>` to force one provider.

## Input and output pipelines

All deployment profiles mount the same pipeline file through
`VELOXMESH_PIPELINE_CONFIG`. Pipeline behavior is independent of profile; the
default example keeps every rule disabled:

```yaml
input:
  rules:
    filter:
      enabled: false
    pii:
      enabled: false
    rewrite:
      enabled: false
    rtk:
      enabled: false
    headroom:
      enabled: false
    caveman:
      enabled: false
    ponytail:
      enabled: false
output:
  rules:
    caveman:
      enabled: false
    ponytail:
      enabled: false
    filter:
      enabled: false
    pii:
      enabled: false
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
docker build -f deploy/docker/gateway.Dockerfile -t veloxmesh-go:main https://github.com/your-org/VeloxMesh.git#main
docker build -f deploy/docker/onnx-worker.Dockerfile -t veloxmesh-onnx-worker:main https://github.com/your-org/VeloxMesh.git#main
```

Or use the explicit remote-build Dockerfiles from a copied deploy bundle:

```bash
docker build -f deploy/docker/remote-build.Dockerfile \
  --build-arg VELOXMESH_REPO_URL=https://github.com/your-org/VeloxMesh.git \
  --build-arg VELOXMESH_BRANCH=main \
  -t veloxmesh-go:main .

docker build -f deploy/docker/onnx-worker.remote-build.Dockerfile \
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
# simple / full / postgres profiles:
docker compose -f deploy/compose/veloxmesh.yml logs -f gateway scheduler-onnx onnx-worker
# compare profile (gateway is named gateway-compare):
docker compose -f deploy/compose/veloxmesh.yml logs -f gateway-compare scheduler-onnx scheduler-heuristic onnx-worker
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

- Commit only `*.example.env`, `*.example.json`, and `*.example.yaml`.
- Do not commit `deploy/env/*.env`, `deploy/data/`, `deploy/reports/`, or model artifacts.
- Keep provider keys in env files and reference them from JSON through `auth.api_key_env`.
- Replace placeholder admin, Grafana, Qdrant, PostgreSQL, and encryption values before use.
