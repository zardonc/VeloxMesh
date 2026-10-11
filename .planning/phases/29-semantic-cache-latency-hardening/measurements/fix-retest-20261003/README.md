# Phase 29 修复复测复现

从仓库根目录运行。依赖 Go、Git Bash、`uv`；SSH 和 Provider 凭据由 runner 从未入库的 `.env.local` 读取。模型、接口和缓存隔离参数都是本次测试输入，不是生产配置。

## 构建

```bash
export GOCACHE="$PWD/.tmp/diagnostic-cache"
export GOTMPDIR="$PWD/.tmp/diagnostic-build"
mkdir -p "$GOCACHE" "$GOTMPDIR"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags phase29preflight -c -o .tmp/phase29-diagnostic-20261003/app-linux.test ./internal/app
go build -o .tmp/phase29-diagnostic-20261003/runner-profile.exe ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
```

## 真实请求

runner 会启动已有的隔离 Redis、Qdrant、Postgres 容器，检查 readiness，并通过 SSH 把 VM 的 `11234` 端口转发到本机 `1234`。本机模型服务必须已运行。下面的模型名称与本轮一致；本地模型的 API key 如有配置，请先通过环境变量设置，勿写入源码。

```bash
export SHIP_BINARY=.tmp/phase29-diagnostic-20261003/app-linux.test
export SHIP_PROVIDER=LOCAL
export SHIP_LOCAL_BASE_URL=http://127.0.0.1:11234/v1
export SHIP_LOCAL_MODEL=qwen2.5-0.5b-instruct
export SHIP_LOCAL_API_KEY="$PHASE29_EMBEDDING_API_KEY"
export PHASE29_MODEL=text-embedding-embedder_collection
export PHASE29_EMBEDDING_BASE_URL=http://127.0.0.1:11234/v1
export PHASE29_READ_TIMEOUT=100ms PHASE29_READ_CONCURRENCY=4
export PHASE29_WRITE_WORKERS=2 PHASE29_QUEUE_CAPACITY=32
export PHASE29_WRITE_TIMEOUT=2s PHASE29_SHUTDOWN_GRACE=1s
export PHASE29_COUNT=100 PHASE29_INTERVAL_MS=125 PHASE29_CLIENT_CONCURRENCY=4
export SHIP_TESTS=TestLiveDirectLoad,TestLiveCacheLoadOff,TestLiveCacheLoadOn,TestLiveCacheLoadOnRepeat,TestLiveCacheLoadOffRepeat
export SHIP_ARTIFACTS=.planning/phases/29-semantic-cache-latency-hardening/measurements/fix-retest-20261003/replay-block-1/LOCAL-qwen2.5-0.5b-instruct
.tmp/phase29-diagnostic-20261003/runner-profile.exe
```

将输出目录中的 `replay-block-1` 依次改为 `replay-block-2`、`replay-block-3`，重复运行。每个窗口 100 个请求，三组各含 direct/off/on/on/off，共 1,500 个请求。命中率约 4%；固定问题集、temperature=0、max_tokens=256；每个后端测试都有 Go timeout 和外部硬截止 60 秒。

改用远程免费模型时，设置 `SHIP_PROVIDER=SANS SHIP_MODEL=oc/space-bunny-free`。先用 `SHIP_TESTS=TestLiveModelAvailabilityBudget` 做小请求可用性检查，再执行 `TestLiveCacheMissEmbeddingReuse,TestPhase29LocalGatewayAcceptance`。不要将不同模型的耗时混入本地模型的 P95 门槛。

独立 embedding 压力测试：`SHIP_TESTS=TestLiveEmbeddingStress`，`PHASE29_COUNT=100`、`PHASE29_CLIENT_CONCURRENCY=4`；该测试尽快发送请求，忽略 interval，每次 embedding 截止 2 秒。并发创建测试：`TestLiveQdrantConcurrentCollection`。队列突发测试：`TestPhase29LocalQueueBurst`。

## 统计及 profiling

```bash
uv run python .planning/phases/29-semantic-cache-latency-hardening/measurements/fix-retest-20261003/analyze-stable.py replay-block
```

本轮最终统计使用 `verified-block-1..3`，已提供 `verified-block-summary.json` 和 `comparison.json`。P95 使用 nearest-rank；置信区间以整组 ABBA 窗口为重采样单位，只有三组，作为探索性证据。

在单独输出目录设置 `SHIP_PROFILE=true`、`SHIP_TESTS=TestLiveCacheLoadOn`、`PHASE29_COUNT=200` 运行，即可保存 CPU、heap、block、mutex 和 execution trace。profiling 样本不纳入门槛。当前 Go 的 trace 文本解析参数为 `-d=parsed`。

```bash
go tool pprof -top .tmp/phase29-diagnostic-20261003/app-linux.test PATH/TestLiveCacheLoadOn.cpuprofile
go tool trace -pprof=sched PATH/TestLiveCacheLoadOn.trace > PATH/trace-sched.pprof
go tool pprof -top PATH/trace-sched.pprof
uv run python .planning/phases/29-semantic-cache-latency-hardening/measurements/fix-retest-20261003/analyze-profile.py profiling-final
```

`SHIP_ARTIFACTS=本次输出根目录 runner-profile.exe audit` 可扫描日志/JSON 中是否包含 `.env.local` 的凭据。runner 会退出时停止自己启动的容器，关闭 SSH 转发；用户的本地模型服务保持运行。

`red` 是修复前的失败证据；`second-model` 保留第二模型首次失败；`green`、`stable-block-*`、`final-block-1`、`profiling` 是开发中间结果。最终门槛只采用 `verified-block-*`，不将历史失败隐藏或与最终样本混合。
