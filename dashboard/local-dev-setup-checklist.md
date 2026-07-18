# 本地开发环境启动清单

根据 `AI Gateway Dashboard + BFF` 开发说明、`.env2.local` 和 `docker-compose.yml`，本地开发分成三层：

1. 基础依赖服务：Redis、RedisInsight、Qdrant，后续可加 Prometheus 和 OTel collector。
2. Go 后端：Gateway / BFF，预期入口是 `go run ./cmd/gateway`。
3. 前端：`web/admin-console`，预期用 pnpm 启动开发服务。

## 当前环境检查结果

当前 `C:\Users\USER\Desktop\capstone\dashboard` 里已有最小可运行的 Go Gateway/BFF、React AI Gateway Dashboard、本地环境配置、Docker Compose 配置和 observability 配置。

已确认：

| 项目 | 状态 |
|---|---|
| Node.js | 已安装，`node --version` 可用 |
| pnpm | 已安装，`pnpm --version` 可用 |
| npm | `npm.cmd` 可用；PowerShell 直接跑 `npm` 会被脚本执行策略拦住 |
| Docker / Docker Compose | 已安装，本地容器依赖服务可运行 |
| Go | 已安装，`go version` 可用 |
| WSL | 已安装 |

因此基础开发工具、本地容器依赖服务、Gateway / BFF 和 AI Gateway Dashboard 都可以在本地运行。

## 工具复查

如需在新终端复查：

```powershell
docker --version
docker compose version
go version
node --version
pnpm --version
```

## 基础依赖服务启动方式

进入配置目录：

```powershell
cd C:\Users\USER\Desktop\capstone\dashboard
```

`.env2.local` 不是 Docker Compose 默认读取的 `.env` 文件名，所以启动时要显式传入：

```powershell
docker compose --env-file .env2.local up -d
```

说明：

- `.env2.local` 已包含开发 API key、默认 provider 和外部模型 provider 配置。
- `docker-compose.yml` 里引用了 `${QDRANT_API_KEY}`，已经在 `.env2.local` 里补了本地值。
- Prometheus 和 OTel collector 需要的配置文件已经补齐：
  - `./docker/observability/prometheus.yml`
  - `./docker/observability/otel-collector-config.yaml`

验证依赖服务：

```powershell
docker compose --env-file .env2.local ps
docker exec veloxmesh_redis_stack redis-cli ping
curl -H "api-key: local-dev-qdrant" http://localhost:6333/healthz
curl http://localhost:9090/-/ready
curl http://localhost:8889/metrics
```

RedisInsight 地址：

```text
http://localhost:5540
```

Qdrant 地址：

```text
http://localhost:6333
```

Prometheus 地址：

```text
http://localhost:9090
```

OTel collector：

```text
OTLP gRPC: http://localhost:4317
OTLP HTTP: http://localhost:4318
Prometheus metrics exporter: http://localhost:8889/metrics
```

## Gateway / BFF 启动方式

在仓库根目录执行：

```powershell
go mod download
go run ./cmd/gateway
```

按说明文档，预期健康检查是：

```powershell
curl http://localhost:8080/health
curl http://localhost:8080/bff/health
```

如果真实仓库入口不是 `./cmd/gateway`，以仓库里的 `go.mod`、`cmd/*` 和 README 为准。

## AI Gateway Dashboard 前端启动方式

```powershell
cd web/admin-console
pnpm install
pnpm dev
```

预期访问：

```text
http://localhost:5173
```

前端应该只调用 BFF，不直接调用 Gateway 内部接口。

## 一键启动

从 `C:\Users\USER\Desktop\capstone\dashboard` 执行：

```powershell
.\scripts\start-dev.ps1
```

这会启动本地容器依赖、Gateway / BFF 和 AI Gateway Dashboard。

## 目前的关键缺口

1. 当前实现是最小本地开发骨架，数据为 BFF mock DTO，还没有接入真实持久化 DB。
2. AI Gateway Dashboard 已经只通过 BFF 读取数据，后续可继续扩展租户、API Key、Provider、路由策略等真实 CRUD。
3. 本地容器依赖服务已经启动并通过健康检查。
