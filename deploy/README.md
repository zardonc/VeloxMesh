# VeloxMesh single-host Docker deployment

This guide is for a host that only has Docker and Docker Compose installed.
The deployment branch is currently `main`.

## One-command install

Use this path when the server does not have a local checkout:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sh
```

Choose a profile:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sh -s -- --profile full
```

The installer creates `./VeloxMesh` under the directory where you run the
command, downloads the deployment files, uses the GitHub repository as the
Docker build context, generates local config, and starts Docker Compose.
Do not run it with `sudo`; Docker access should be granted to the current user
so generated config remains editable.
Existing local env/config files are kept on rerun for the same profile. To
change profiles, use a different `--install-dir`, edit `env/veloxmesh.env`, or
uninstall first. Override the location with `--install-dir` or
`VELOXMESH_INSTALL_DIR` when needed.
The Docker Compose project name defaults to `veloxmesh`; override it with
`--project-name` or `VELOXMESH_PROJECT_NAME` and use the same value when
uninstalling.
By default, only the gateway API port `8080` binds to all host interfaces.
Admin, data, and observability ports bind to `127.0.0.1`.

If omitted, the installer generates random values for `DEV_API_KEY`,
`ADMIN_API_KEY`, `GRAFANA_ADMIN_PASSWORD`, `POSTGRES_PASSWORD`, and the
control-state encryption key. It does not generate a real provider API key;
pass `--provider-api-key` or edit `env/veloxmesh.env` before making live model
calls.

## One-command uninstall

Uninstall the default `./VeloxMesh` installation from the directory where you
run the command:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- --yes
```

Remove Docker Compose named volumes too:

```bash
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- --yes --volumes
```

Use `--install-dir` or `VELOXMESH_INSTALL_DIR` if the install path was changed.
Use `--project-name` or `VELOXMESH_PROJECT_NAME` if the Compose project name was
changed.
If Docker Compose is unavailable or shutdown fails, the uninstall script stops
without deleting local files.

If an older install was created with `sudo`, fix ownership once before editing
or uninstalling:

```bash
sudo chown -R "$(id -u):$(id -g)" ./VeloxMesh
```

## One-command deployment quick reference / 一键部署速查

### 中文

#### 1. 部署步骤

1. 准备一台已安装 Docker Engine 或 Docker Desktop 的机器，并确认当前用户可直接运行 `docker compose`，不要使用 `sudo`。
2. 在希望安装系统的目录执行 one-command 安装；脚本会创建当前目录下的 `./VeloxMesh`。
3. 选择部署版本：默认 `simple`，可通过 `--profile full|compare|postgres` 切换。
4. 传入 provider 信息，或安装后编辑 `./VeloxMesh/env/veloxmesh.env` 和 `./VeloxMesh/config/app.<profile>.json`。
5. 脚本下载部署文件、生成本地配置、构建镜像并启动 Docker Compose。
6. 用健康检查、模型列表、测试脚本验证部署结果。

```bash
# simple: default profile, creates ./VeloxMesh
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sh -s -- \
  --provider-api-key "<provider-api-key>" \
  --provider-base-url "https://api.example.com/v1" \
  --provider-model "example-model"

# full / compare / postgres: choose one profile
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sh -s -- \
  --profile full \
  --provider-api-key "<provider-api-key>" \
  --provider-base-url "https://api.example.com/v1" \
  --provider-model "example-model"
```

#### 2. 各版本区别

| 版本 | 组件 | 适用场景 | 部署差异 |
| --- | --- | --- | --- |
| `simple` | Gateway、ONNX scheduler、ONNX worker、Prometheus、Grafana、Loki、cAdvisor | 最小可运行验证 | 默认版本，不启动 Redis/Qdrant/Postgres |
| `full` | `simple` + Redis/RedisInsight + Qdrant | 缓存、向量组件和完整本地栈验证 | 使用 `full` 配置文件和 Compose profile |
| `compare` | Gateway + ONNX scheduler + heuristic scheduler + ONNX worker + observability | 对比 `onnx_rollout_percent=100/0/50` | Gateway 服务名为 `gateway-compare`，同时启动两个 scheduler |
| `postgres` | `full` + PostgreSQL/pgvector + Adminer | pgvector / PostgreSQL 后端验证 | 同时启用 `full` 和 `postgres` profile |

#### 3. 本地必要配置文件

| 文件 | 位置 | 用途 | 密钥来源 |
| --- | --- | --- | --- |
| `veloxmesh.env` | `./VeloxMesh/env/veloxmesh.env` | Compose 环境变量、端口绑定、API key、配置文件路径 | `DEV_API_KEY` 自动生成；`OPENAI_PRIMARY_API_KEY` 需用户填写或通过 `--provider-api-key` 传入 |
| `app.<profile>.json` | `./VeloxMesh/config/` | Gateway provider、routing、control-state、scheduler 总配置 | `admin_api_key`、`encryption_key` 自动生成；provider URL/model/key env 需用户确认 |
| `scheduler.<profile>.json` | `./VeloxMesh/config/` | scheduler endpoint、ONNX rollout、队列和质量参数 | 无密钥 |
| `cache.<profile>.json` | `./VeloxMesh/config/` | Redis/Qdrant/Postgres/semantic cache 配置 | 外部服务密码需用户确认；本地 Postgres 密码默认自动生成 |
| `pipeline.yaml` | `./VeloxMesh/config/pipeline.yaml` | 统一输入/输出 pipeline 配置 | 无密钥 |
| `heuristic.example.json` | `./VeloxMesh/config/heuristic.example.json` | `compare` 中 heuristic scheduler 配置 | 无密钥 |

脚本自动生成：`DEV_API_KEY`、`ADMIN_API_KEY`、`GRAFANA_ADMIN_PASSWORD`、`POSTGRES_PASSWORD`、control-state `encryption_key`。
用户必须提供或编辑：真实 provider API key、provider base URL、model 名称；多 provider 时，每个 `providers[].auth.api_key_env` 必须在 `veloxmesh.env` 中有同名变量。

#### 4. 各版本关键参数

| 版本 | 文件 | 参数 | 示例 | 注意事项 |
| --- | --- | --- | --- | --- |
| all | `veloxmesh.env` | `VELOXMESH_PROJECT_NAME` | `veloxmesh` | 卸载时需使用同一个 project name |
| all | `veloxmesh.env` | `VELOXMESH_GATEWAY_BIND_ADDR` | `0.0.0.0` | 网关可被非本机 client 调用 |
| all | `veloxmesh.env` | `VELOXMESH_ADMIN_BIND_ADDR` | `127.0.0.1` | Admin API 默认仅本机访问 |
| all | `veloxmesh.env` | `VELOXMESH_LOCAL_BIND_ADDR` | `127.0.0.1` | Grafana、Prometheus、RedisInsight、Adminer 等默认仅本机访问 |
| all | `app.<profile>.json` | `providers[].base_url` | `https://api.example.com/v1` | 必须是 OpenAI-compatible API 地址 |
| all | `app.<profile>.json` | `providers[].auth.api_key_env` | `OPENAI_PRIMARY_API_KEY` | 变量名必须存在于 `veloxmesh.env` |
| all | `app.<profile>.json` | `providers[].models[]` / `default_model` | `example-model` | 测试请求中的 model 应在列表中 |
| simple/full/postgres | `scheduler.<profile>.json` | `onnx_rollout_percent` | `100` | `100` 表示全部走 ONNX scheduler |
| compare | `scheduler.compare.json` | `onnx_rollout_percent` | `100` / `0` / `50` | 用于 ONNX/heuristic 对比测试 |
| full | `cache.full.json` | Redis/Qdrant/cache 参数 | `redis:6379`, `qdrant:6333` | 仅 `full`/`postgres` 需要关注 |
| postgres | `veloxmesh.env` | `POSTGRES_USER/PASSWORD/DB/PORT` | `veloxmesh` / `5432` | `POSTGRES_PASSWORD` 默认自动生成 |

#### 5. 连接组件 Client UI

本机访问：

| 组件 | URL | 说明 |
| --- | --- | --- |
| Gateway | `http://localhost:8080` | OpenAI-compatible API |
| Admin API | `http://localhost:8081` | 需 `ADMIN_API_KEY`，默认仅本机 |
| Grafana | `http://localhost:3000` | 用户 `admin`，密码见 `GRAFANA_ADMIN_PASSWORD` |
| Prometheus | `http://localhost:9090` | 指标查询 |
| cAdvisor | `http://localhost:8083` | 容器资源 |
| RedisInsight | `http://localhost:5540` | 仅 `full`/`postgres` |
| Qdrant Dashboard | `http://localhost:6333/dashboard` | 仅 `full`/`postgres` |
| Adminer | `http://localhost:8082` | 仅 `postgres` |

远程机器查看本地绑定的 UI，建议使用 SSH tunnel：

```bash
# 将远端 Grafana 映射到本机 3000；8080 可直接按网关绑定策略访问
ssh -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 -L 8081:127.0.0.1:8081 user@host
```

#### 6. 基础测试

```bash
cd ./VeloxMesh

# 查看容器状态；-p 使用 veloxmesh.env 中的 Compose project name
docker compose -p veloxmesh --env-file env/veloxmesh.env -f compose/veloxmesh.yml ps

# 网关健康检查；8080 是 Gateway API 端口
curl http://localhost:8080/healthz

# 查看可用模型；DEV_API_KEY 来自 env/veloxmesh.env
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer <DEV_API_KEY>"

# 调用网关聊天接口；model 必须匹配 app.<profile>.json 中的配置
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <DEV_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"model":"example-model","messages":[{"role":"user","content":"reply ok"}],"max_tokens":8}'

# simple/full/postgres: 查看 gateway、ONNX scheduler、ONNX worker 日志
docker compose -p veloxmesh --env-file env/veloxmesh.env -f compose/veloxmesh.yml logs -f gateway scheduler-onnx onnx-worker

# compare: 查看 gateway-compare 和两个 scheduler 日志
docker compose -p veloxmesh --env-file env/veloxmesh.env -f compose/veloxmesh.yml logs -f gateway-compare scheduler-onnx scheduler-heuristic onnx-worker
```

#### 7. 使用脚本运行测试数据

```bash
cd ./VeloxMesh

# simple: 运行 3 条 smoke 数据并生成 reports/simple-smoke-*
sh scripts/test-simple-smoke.sh

# full: 使用并发 8 运行 full dataset，并生成 reports/full-concurrent-*
sh scripts/test-full-concurrent.sh

# compare: 按顺序测试 onnx_rollout_percent=100、0、50，并生成 reports/compare-rollout-*
sh scripts/test-compare-rollout.sh
```

#### 8. 安全卸载

```bash
# 停止容器并删除 ./VeloxMesh，本地 Compose volumes 保留
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- --yes

# 同时删除 Compose named volumes；会删除本地持久化数据
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- --yes --volumes

# 如果安装路径或 project name 改过，卸载时必须传同样的值
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- \
  --install-dir ./VeloxMesh \
  --project-name veloxmesh \
  --yes
```

### English

#### 1. Deployment steps

1. Prepare a host with Docker Engine or Docker Desktop, and make sure the current user can run `docker compose` without `sudo`.
2. Run the one-command installer in the directory where `./VeloxMesh` should be created.
3. Pick a profile: default is `simple`; use `--profile full|compare|postgres` when needed.
4. Pass provider settings on the command line, or edit `./VeloxMesh/env/veloxmesh.env` and `./VeloxMesh/config/app.<profile>.json` after installation.
5. The script downloads deployment files, generates local config, builds images, and starts Docker Compose.
6. Verify with health checks, model listing, and the bundled dataset scripts.

```bash
# simple: default profile, creates ./VeloxMesh
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sh -s -- \
  --provider-api-key "<provider-api-key>" \
  --provider-base-url "https://api.example.com/v1" \
  --provider-model "example-model"

# full / compare / postgres: choose one profile
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/install.sh | sh -s -- \
  --profile full \
  --provider-api-key "<provider-api-key>" \
  --provider-base-url "https://api.example.com/v1" \
  --provider-model "example-model"
```

#### 2. Profile differences

| Profile | Components | Best for | Deployment difference |
| --- | --- | --- | --- |
| `simple` | Gateway, ONNX scheduler, ONNX worker, Prometheus, Grafana, Loki, cAdvisor | Minimum live deployment | Default profile; no Redis/Qdrant/Postgres |
| `full` | `simple` + Redis/RedisInsight + Qdrant | Cache, vector store, full local stack | Uses `full` config and Compose profile |
| `compare` | Gateway + ONNX scheduler + heuristic scheduler + ONNX worker + observability | Testing `onnx_rollout_percent=100/0/50` | Gateway service is `gateway-compare`; both schedulers run |
| `postgres` | `full` + PostgreSQL/pgvector + Adminer | pgvector / PostgreSQL backend testing | Enables both `full` and `postgres` profiles |

#### 3. Required local config files

| File | Path | Purpose | Secret source |
| --- | --- | --- | --- |
| `veloxmesh.env` | `./VeloxMesh/env/veloxmesh.env` | Compose env, bind addresses, API keys, config paths | `DEV_API_KEY` is generated; `OPENAI_PRIMARY_API_KEY` must be passed or edited |
| `app.<profile>.json` | `./VeloxMesh/config/` | Gateway providers, routing, control-state, scheduler block | `admin_api_key` and `encryption_key` are generated; provider URL/model/key env must be checked |
| `scheduler.<profile>.json` | `./VeloxMesh/config/` | Scheduler endpoints, ONNX rollout, queue and quality settings | No secret |
| `cache.<profile>.json` | `./VeloxMesh/config/` | Redis/Qdrant/Postgres/semantic cache settings | External service secrets are user-owned; local Postgres password is generated |
| `pipeline.yaml` | `./VeloxMesh/config/pipeline.yaml` | Shared input/output pipeline config | No secret |
| `heuristic.example.json` | `./VeloxMesh/config/heuristic.example.json` | Heuristic scheduler config for `compare` | No secret |

Generated by the installer: `DEV_API_KEY`, `ADMIN_API_KEY`, `GRAFANA_ADMIN_PASSWORD`, `POSTGRES_PASSWORD`, and control-state `encryption_key`.
User-provided or edited: real provider API key, provider base URL, and model name. For multiple providers, every `providers[].auth.api_key_env` value must exist in `veloxmesh.env`.

#### 4. Key profile parameters

| Profile | File | Parameter | Example | Notes |
| --- | --- | --- | --- | --- |
| all | `veloxmesh.env` | `VELOXMESH_PROJECT_NAME` | `veloxmesh` | Use the same project name when uninstalling |
| all | `veloxmesh.env` | `VELOXMESH_GATEWAY_BIND_ADDR` | `0.0.0.0` | Lets non-local clients call the gateway |
| all | `veloxmesh.env` | `VELOXMESH_ADMIN_BIND_ADDR` | `127.0.0.1` | Admin API is local-only by default |
| all | `veloxmesh.env` | `VELOXMESH_LOCAL_BIND_ADDR` | `127.0.0.1` | Grafana, Prometheus, RedisInsight, Adminer, and similar UIs stay local |
| all | `app.<profile>.json` | `providers[].base_url` | `https://api.example.com/v1` | Must be an OpenAI-compatible API base URL |
| all | `app.<profile>.json` | `providers[].auth.api_key_env` | `OPENAI_PRIMARY_API_KEY` | Must exist in `veloxmesh.env` |
| all | `app.<profile>.json` | `providers[].models[]` / `default_model` | `example-model` | Test request model must match the configured model |
| simple/full/postgres | `scheduler.<profile>.json` | `onnx_rollout_percent` | `100` | `100` means all scheduler traffic uses ONNX |
| compare | `scheduler.compare.json` | `onnx_rollout_percent` | `100` / `0` / `50` | Used for ONNX vs heuristic comparison |
| full | `cache.full.json` | Redis/Qdrant/cache settings | `redis:6379`, `qdrant:6333` | Relevant only for `full`/`postgres` |
| postgres | `veloxmesh.env` | `POSTGRES_USER/PASSWORD/DB/PORT` | `veloxmesh` / `5432` | `POSTGRES_PASSWORD` is generated by default |

#### 5. Client UI access

Local access:

| Component | URL | Notes |
| --- | --- | --- |
| Gateway | `http://localhost:8080` | OpenAI-compatible API |
| Admin API | `http://localhost:8081` | Requires `ADMIN_API_KEY`; local-only by default |
| Grafana | `http://localhost:3000` | User `admin`; password is `GRAFANA_ADMIN_PASSWORD` |
| Prometheus | `http://localhost:9090` | Metrics query UI |
| cAdvisor | `http://localhost:8083` | Container resources |
| RedisInsight | `http://localhost:5540` | `full`/`postgres` only |
| Qdrant Dashboard | `http://localhost:6333/dashboard` | `full`/`postgres` only |
| Adminer | `http://localhost:8082` | `postgres` only |

For remote UI access, prefer SSH tunnels:

```bash
# Map remote Grafana, Prometheus, and Admin API to local ports.
ssh -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 -L 8081:127.0.0.1:8081 user@host
```

#### 6. Basic tests

```bash
cd ./VeloxMesh

# Show container status; -p is the Compose project name from veloxmesh.env.
docker compose -p veloxmesh --env-file env/veloxmesh.env -f compose/veloxmesh.yml ps

# Gateway health check; 8080 is the Gateway API port.
curl http://localhost:8080/healthz

# List configured models; DEV_API_KEY is in env/veloxmesh.env.
curl http://localhost:8080/v1/models \
  -H "Authorization: Bearer <DEV_API_KEY>"

# Call chat completions; model must match app.<profile>.json.
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <DEV_API_KEY>" \
  -H "Content-Type: application/json" \
  -d '{"model":"example-model","messages":[{"role":"user","content":"reply ok"}],"max_tokens":8}'

# simple/full/postgres: show gateway, ONNX scheduler, and ONNX worker logs.
docker compose -p veloxmesh --env-file env/veloxmesh.env -f compose/veloxmesh.yml logs -f gateway scheduler-onnx onnx-worker

# compare: show gateway-compare and both scheduler logs.
docker compose -p veloxmesh --env-file env/veloxmesh.env -f compose/veloxmesh.yml logs -f gateway-compare scheduler-onnx scheduler-heuristic onnx-worker
```

#### 7. Run test datasets with scripts

```bash
cd ./VeloxMesh

# simple: run 3 smoke samples and write reports/simple-smoke-*
sh scripts/test-simple-smoke.sh

# full: run the full dataset with concurrency 8 and write reports/full-concurrent-*
sh scripts/test-full-concurrent.sh

# compare: run onnx_rollout_percent=100, 0, and 50 in order and write reports/compare-rollout-*
sh scripts/test-compare-rollout.sh
```

#### 8. Safe uninstall

```bash
# Stop containers and delete ./VeloxMesh; keep local Compose volumes.
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- --yes

# Also delete Compose named volumes; this removes local persisted data.
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- --yes --volumes

# If install dir or project name changed, pass the same values during uninstall.
curl -fsSL https://raw.githubusercontent.com/zardonc/VeloxMesh/main/deploy/uninstall.sh | sh -s -- \
  --install-dir ./VeloxMesh \
  --project-name veloxmesh \
  --yes
```

## Prerequisites

- Docker Engine or Docker Desktop with Docker Compose v2.
- Docker must be usable by the current user without `sudo`.
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

## Network exposure

Default bind addresses:

```env
VELOXMESH_GATEWAY_BIND_ADDR=0.0.0.0
VELOXMESH_ADMIN_BIND_ADDR=127.0.0.1
VELOXMESH_LOCAL_BIND_ADDR=127.0.0.1
```

This allows non-local clients to call the gateway on `http://<host>:8080` while
keeping the admin API, RedisInsight, Adminer, Prometheus, Grafana, Loki,
OpenTelemetry, cAdvisor, Redis, Qdrant, and PostgreSQL reachable only from the
host. For remote administration, prefer an SSH tunnel:

```bash
ssh -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 user@<host>
```

Only set `VELOXMESH_ADMIN_BIND_ADDR=0.0.0.0` or
`VELOXMESH_LOCAL_BIND_ADDR=0.0.0.0` behind a firewall, VPN, or reverse proxy
with authentication.

Promtail reads Docker JSON log files from `/var/lib/docker/containers` and does
not mount `/var/run/docker.sock`. If Docker Desktop or host permissions prevent
that read, use `docker compose logs` for container logs.

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

These management URLs are intentionally local-only unless
`VELOXMESH_LOCAL_BIND_ADDR` is changed.

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
