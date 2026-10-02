---
phases: [27, 28, 29]
tested_local_date: 2026-10-01
status: gaps_found
merge_allowed: false
source_revision: 1b58f04f70d957ec2b21549b47ac356df8a115cc
branch: codex/phase-27-stream-terminal-settlement
scope: real-application-real-components
mock_services_used: false
---

# Phase 27 / 28 / 29 真实链路与压力测试报告

**未达到合并条件，当前分支未合并到 main。** 本轮发现 OpenAI-compatible 流式帧处理、Gemini 工具续接兼容性问题，Phase 29 延迟门槛也未通过。真实本地 embedding 压测通过，不代表三个 Phase 的所有功能均通过。

本报告独立记录本次结果，不覆盖已有 VERIFICATION、历史压测或此前的通过结论。测试时间跨越 UTC 2026-10-02，温哥华本地日期为 2026-10-01。

## 实际范围与组件

代码来源是上述 HEAD 加本轮工作区的验收代码。产品实现未修改；新增真实测试、压测/分析脚本，并调整既有验收 runner 和 provider 选择辅助函数。没有提交、推送、PR 或部署。

所有网关用例通过真实 `App.New()` 装配，再使用 HTTP 客户端调用真实 Router 的 `/v1/chat/completions`：认证 → durable provider/config → gateway → 实际 provider adapter → 真实模型服务 → HTTP/SSE → Usage/余额持久化。`httptest.NewServer` 仅提供 HTTP 监听，不替代应用、模型或组件。

语义缓存链路额外包含真实本地 embedding adapter、实际 Qdrant、磁盘 SQLite、实际 Redis 配置订阅及异步缓存写入。每例创建独立数据库、非管理员 key、Redis namespace、测试缓存配置/白名单；不修改生产 profile。PostgreSQL 测试容器由既有 runner 启动并检查就绪，但本轮应用使用 SQLite，**不计为 PostgreSQL 应用链路验收**。

本地接口仅用于 embedding，严格选择用户指定模型。其他模型来自 `.env.local` 的三个 provider 的默认模型：

| 选择 | 实际协议 / 模型 | 本轮用途 |
| --- | --- | --- |
| SANS | OpenAI-compatible / `oc/big-pickle` | 网关、工具、缓存、后续限流记录 |
| GEM | Gemini 原生 SDK / `gemini-3.1-flash-lite` | 网关、工具、普通/缓冲流式、原生续接诊断 |
| GPT | OpenAI-compatible / `gpt-6-luna` | 网关、工具、Fusion、缓存、负载对照 |
| 本地 embedding | `http://127.0.0.1:1234/v1/embeddings` / `text-embedding-embedder_collection` | 768 维探测、缓存真实调用、批量与并发压力 |

隔离机通过 SSH reverse loopback 转发访问本机 embedding，故应用侧测试地址为 `http://127.0.0.1:11234/v1`。没有调用本地聊天模型、其他 inventory embedding 模型或在线 embedding provider。凭据由 runner 读取环境文件并通过 SSH stdin 传入内存，不复制环境文件到证据目录。

所有执行的应用用例、负载及模型诊断均使用真实服务。测试问题、真实创建的 key/配置、客户端实际执行的 `19 + 23` 计算是测试输入，不是伪造模型响应。未执行既有 mock/stub 全量单测、注入 503 的旧故障测试、使用构造答案/StubMetrics 的旧队列测试。

## 功能结果

下表以已修正测试准备步骤后的证据为准；全部日志保留在 [ship-e2e-20261001](measurements/ship-e2e-20261001/)。

| Phase / 用例 | 结果 | 证据与限定 |
| --- | --- | --- |
| 27 普通流式、完成及结算 | GPT 失败；GEM 通过 | GPT 返回 SSE `provider_bad_response`；此前观测无成功 Usage 行。GEM 有真实内容、一个 DONE、一次结算，16 tokens / 16 测试 credits。见 `final-refactor-gpt/`、`gem-final/` |
| 27 客户端取消 | GPT 通过；GEM 未确认 | GPT 收到非空片段后断开，无 Usage 结算、无 provider 健康惩罚。GEM 断开后已存在结算；可能上游已完成，缺少取消先于完成的证据，不能判定为已证实错误扣费。见 `gpt-final/`、`gem-final/` |
| 27 响应规则触发的缓冲流式 | GEM 通过；GPT 失败 | 规则经 durable repository 保存、reload 并确认启用。GEM 有内容、DONE 和一次结算；GPT HTTP 502 `provider_bad_response`。见 `final-refactor-gem/`、`gem-buffer-final/`、`gpt-buffer-final/` |
| 27 / 28 Fusion 流式 | 失败 | 持久化配置的单成员 + 真实 judge 路径：GPT SSE `provider_bad_response`；GEM HTTP 502 `fusion_failed`。未声称完成多 provider Fusion 容量或所有工具/Fusion 策略。见 `gpt-root-cause/`、`gem-confirm/` |
| 28 非流式 `required` / named / `auto` / omitted | GPT 四项通过；GEM 四项失败 | 模型真实生成工具参数 → 客户端计算 → 同一 tool_call_id 传回 JSON 对象结果 → 最终回答。GPT 每项两次正常结算；GEM 第二轮 HTTP 400 `provider_invalid_request`。见 `gpt-final/`、`gem-final/` |
| 28 `tool_choice=none` | GPT / GEM 通过 | 未返回工具调用，有文本及一次正常结算。见 `gpt-final/`、`gem-final/` |
| 28 工具 SSE + 工具结果续接 | GPT / GEM 失败 | GPT 第一轮 SSE 报错；GEM 第一轮工具事件能重组，第二轮 HTTP 400。见 `gpt-root-cause/`、`gem-final/` |
| 28 Gemini 直接原生 SDK 诊断 | 通过，并复现网关缺陷机制 | 保留完整真实 model Content 可续接；仅移除真实 opaque signature 后同一 provider 返回 400。见 `gem-confirm/`、`final-refactor-gem/TestLiveGeminiNativeContinuation.log` |
| 29 缓存完整调用链 | GPT 通过；SANS 首轮通过；GEM 失败 | 首次 miss → 实际主模型 → 异步 embedding/Qdrant 写入 → 改写 1/1 hit 且答案相同 → 不同答案负例 2/2 miss → 同 key/model/repository 版本切换立即 miss。GPT 命中完整响应 51.726 ms；GEM 首个主模型请求超时。见 `gpt/TestPhase29LocalGatewayAcceptance.log`、`real-chain/`、`gem-final/` |
| 实际 HTTP 认证 | GPT / GEM 通过 | 缺失及无效凭据被真实应用拒绝。见 `gpt-features/`、`gem-diagnostics/` |
| SANS 后续真实调用 | 未通过 | 实际 `provider_rate_limit`；负载 warmup 失败，没有有效的延迟对照。首次个别成功不能消除后续限流。见 `sans/` |

最终测试辅助函数按仓库复杂度限制整理后，再执行 GPT 认证、上游流诊断、required 工具续接、取消及普通流式确认，结果记录在 `final-refactor-gpt/`，4 通过 / 1 失败；Gemini 原生签名对照及缓冲流式复核记录在 `final-refactor-gem/`，2 通过。其他矩阵项使用表中对应轮次；只做辅助函数拆分不把旧日志改写成新执行。

## 本地 embedding 压测

直接调用用户指定的真实 embedding 接口，每请求输入两段不同文本。逐请求验证输出数量/索引、相同且非零维度、有限数值、非零向量以及不同输入的向量不同。日志只保存检查结果、耗时和元数据，不保存向量/原始响应。

| 并发 | 请求数 | 成功 | 用时 s | 请求/s | P50 ms | P95 ms | P99 ms | 原始记录 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 80 | 80 | 2.755 | 29.04 | 25.957 | 57.949 | 245.488 | `embedding-c1.jsonl` |
| 4 | 160 | 160 | 2.894 | 55.28 | 62.347 | 80.991 | 380.124 | `embedding-c4.jsonl` |
| 8 | 160 | 160 | 2.994 | 53.45 | 124.682 | 292.646 | 574.298 | `embedding-c8.jsonl` |
| 4，较长批次 | 2048 | 2048 | 43.560 | 47.02 | 83.543 | 103.426 | 116.997 | `embedding-c4-sustained.jsonl` |
| 16 | 512 | 512 | 10.755 | 47.60 | 329.916 | 388.498 | 712.260 | `embedding-c16.jsonl` |

合计 **2960 请求 / 5920 向量，0 失败，全部 768 维**。短批次到较长批次的吞吐不同，不能将 55.28 请求/s 宣称为持续容量。并发 16 没有提高实测吞吐，反而增加延迟。最长批次约 44 秒，本轮不提供长时间 soak、硬件利用率或生产容量结论。

## Phase 29 真实应用负载对照

使用真实 GPT 主模型、同一实际 App 装配方式和本地 embedding。每轮独立实例，先真实 warmup，再按 1 请求/s 调度 32 请求，客户端并发上限 4。执行次序为 off → on → on repeat → off repeat；输入大多为不同问题，仅每第 21 请求重复前一个问题。实际四轮命中均为 0，为 miss-heavy 低命中场景。客户端耗时包含完整 HTTP body。

测试隔离参数：read deadline 100 ms、read concurrency 4、write timeout 2 s、workers 2、queue 32、shutdown grace 1 s；沿用测试 threshold 0.99999。这些不是批准的生产参数。

| 顺序 / 模式 | 成功/尝试 | 实际请求/s | 观察最大并发 | 调度延迟 P95 ms | 成功响应 P50 / P95 / P99 ms |
| --- | ---: | ---: | ---: | ---: | --- |
| 1 off | 32/32 | 0.970 | 3 | 1.444 | 1594.745 / 2973.632 / 3732.824 |
| 2 on | 31/32 | 0.970 | 4 | 740.666 | 1610.606 / 6950.832 / 8271.227 |
| 3 on repeat | 32/32 | 0.980 | 3 | 1.581 | 1668.024 / 3166.033 / 3223.562 |
| 4 off repeat | 32/32 | 0.985 | 3 | 1.883 | 1612.021 / 2694.077 / 3327.957 |

首组存在 1 个 12 秒客户端超时及调度积压，**不能作为全部成功的对照**。其成功响应 P95 比值为 2.337；保留失败请求后的全部尝试 P95/P99 为 8271.227/12001.063 ms。未删除失败请求来宣称通过。

第二组全部 64 个请求成功，实际吞吐接近、无明显调度积压，但 **P95 比值 3166.033 / 2694.077 = 1.17518 > 1.05，门槛失败**。主模型在线延迟仍有波动；这证明本轮未达门槛，不能精确归因全部差异为缓存开销，也不能据此批准生产 bounds。

原始每请求日志及解析 JSONL 位于 `gpt/TestLiveCacheLoad*.log` / `.jsonl`；可重算统计见 [load-summary.json](measurements/ship-e2e-20261001/gpt/load-summary.json)。127/128 请求成功不等于性能验收通过。

## 未解决问题及定位

| 编号 | 问题 / 预期与实际 | 定位 / 证据 | 当前状态 |
| --- | --- | --- | --- |
| E2E-01 | 流式结束原因后仍应接收 usage-only 帧并完成一次结算；实际被判坏响应 | [OpenAI handleStreamData](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/providers/openai/adapter.go:280) 在 JSON 解码前拒绝 `state.terminal` 后的非 DONE 数据。实际 GPT 工具流共 13 帧，finish 在第 12 帧、usage 在第 13 帧，然后 DONE；直接调用成功，网关 SSE 报错。见 `gpt-root-cause/`、`final-refactor-gpt/` | 未修复；影响普通/工具/Fusion 流式及 GPT 缓冲请求 |
| E2E-02 | Gemini 工具结果续接应保留 provider 所需的 opaque 元数据；实际重建时丢失 signature | [geminiFunctionCallParts](C:/Users/inthe/IdeaProjects/VeloxMesh/internal/providers/gemini/tool_protocol.go:115) 仅重建 ID/Name/Args；直接原生诊断保留签名成功、仅删签名真实 400。见 `gem-confirm/`、`final-refactor-gem/` | 未修复；四类工具选择及工具流续接失败 |
| E2E-03 | 低命中缓存完整响应 P95 ≤ off 的 1.05 倍；实际第二组为 1.17518 倍 | 四轮真实完整响应样本、超时及调度延迟全部保留 | 门槛失败，不能合并或批准生产参数 |
| E2E-04 | 各配置 provider 的链路均可完成；SANS 实际限流，GEM 部分请求超时/Fusion 失败 | `sans/`、`gem-final/`、`gem-confirm/` | 外部失败/未定位项仍存在，不以外部原因豁免全通过条件 |
| E2E-05 | 取消应发生在上游完成前；GEM 用例缺少可靠时序证据 | `gem-final/TestLiveStreamCancellation.log` | 验收未完成，不能当作通过或已证明重复扣费缺陷 |

SSE 的 error event 后也可能出现 `[DONE]`，因此 DONE 本身不足以证明成功。最终测试显式拒绝 SSE error，早期未显式解析 error 的日志仍保留，其结算断言失败有效，但不拿 DONE 当作通过依据。

Google 官方说明 signature 是响应 Part 上的 opaque 元数据；本报告中的 400 因果证据来自实际接口的保留/删除对照，而非仅凭文档推断。参考 [Gemini thinking 文档](https://ai.google.dev/gemini-api/docs/thinking)。

## 排除的准备失败与覆盖边界

初始本地 embedding base 未带 `/v1`，以及缺少测试用非空 auth 占位值导致的装配失败，只归类为测试准备问题。后续始终使用正确接口及运行时占位值，本地服务本身不需要该值做鉴权。根目录早期日志、`v1-chain/` 不计产品失败或成功。

早期 Gemini 工具结果使用标量 `42`，已改为客户端真实计算的 JSON 对象 `{"result":42}`；修正后仍返回 400。缓冲规则最初仅写 Output stage、未被当前 durable schema 激活，以及 provider rates 在 provider seed 前保存引起的外键失败，均不计产品问题。最终 `*-buffer-final/` 确认持久化规则已激活后才计结果。早期 Fusion 空聚合的 NULL 扫描属于测试断言问题，已修正；最终显式 SSE error 证据不受其影响。

本轮没有可用的原生 Anthropic provider 输入，不使用 mock 替代；未完成原生 Anthropic、所有 `.env.local` 备用模型、多 provider Fusion、长时间 soak、故障注入/队列所有边界或生产切换验收。仅测试一款指定 embedding 模型，不把旧双模型计划自动标记完成。已有 Phase 27/28 passed 文件保留为历史记录，Phase 29 原 VERIFICATION 仍为 partial，不能用它们覆盖本次失败结果。

## 复现

从仓库根目录使用 Git Bash；需要既有 `.env.local`、SSH known_hosts 及已授权的隔离组件。以下命令不包含真实密钥。每次复现使用新的证据目录，保留旧日志。

```bash
GOCACHE="$PWD/.tmp/go-cache" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go test -c -tags phase29preflight -o /tmp/veloxmesh-phases-live.test ./internal/app
GOCACHE="$PWD/.tmp/go-cache" go build -o /tmp/veloxmesh-acceptance-runner.exe \
  ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
export SHIP_BINARY="$(cygpath -m /tmp/veloxmesh-phases-live.test)"
export SHIP_ARTIFACTS="$PWD/.tmp/ship-recheck-$(date +%Y%m%d-%H%M%S)"
export SHIP_PROVIDER=GPT
export SHIP_TESTS=TestLiveUpstreamStreamDiagnostic,TestLiveToolRequired,TestLiveStreamSettlement
export PHASE29_MODEL=text-embedding-embedder_collection
export PHASE29_EMBEDDING_BASE_URL=http://127.0.0.1:11234/v1
export PHASE29_EMBEDDING_API_KEY=local-test
export PHASE29_READ_TIMEOUT=100ms PHASE29_READ_CONCURRENCY=4
export PHASE29_WRITE_TIMEOUT=2s PHASE29_WRITE_WORKERS=2
export PHASE29_QUEUE_CAPACITY=32 PHASE29_SHUTDOWN_GRACE=1s
/tmp/veloxmesh-acceptance-runner.exe
```

`SHIP_TESTS` 显式选择真实用例并绕过 legacy full-suite 分支。切换 GEM 用原生 adapter；SANS/GPT 用各自真实 OpenAI-compatible adapter。复现四轮负载时设 `PHASE29_COUNT=32`、`PHASE29_INTERVAL_MS=1000`、`PHASE29_CLIENT_CONCURRENCY=4`，依次选择 `TestLiveCacheLoadOff,TestLiveCacheLoadOn,TestLiveCacheLoadOnRepeat,TestLiveCacheLoadOffRepeat`。提取统计使用 `node scripts/live-ship-analyze.mjs "$SHIP_ARTIFACTS"`。

```bash
uv run --no-project python scripts/live-embedding-check.py \
  --url http://127.0.0.1:1234 --model text-embedding-embedder_collection \
  --concurrency 4 --count 2048 --output .tmp/embedding-recheck.jsonl
```

应用测试使用 Go `-test.timeout 60s` 和外层 `timeout 60s`；embedding 脚本每次总执行上限 60 秒，每请求 10 秒。应用 HTTP 客户端上限 12 秒，不将超时请求计作成功。

## 静态检查、证据与清理

`go vet -tags phase29preflight ./...` 及 runner vet、tagged Linux 测试 binary 编译、runner 构建、Node 语法检查、uv Python AST 语法检查均通过。新增/修改 Go 测试与 runner 已检查文件 ≤500 行、函数非空行 ≤50、参数 ≤3 和分支复杂度 ≤10。仓库没有适用的前端 package.json，未宣称执行 pnpm build/typecheck。未运行含 mock 的全量后端套件。

默认 Go cache 曾因访问权限失败；使用 workspace 内 `.tmp/go-cache` 重编译后通过。此前预检/配置错误日志保留，不隐藏异常。证据目录递归扫描 `.jsonl`、`.json`、`.log` 中来自环境文件的 key/password/secret/token 值，未发现凭据泄漏。

最终 Linux 测试 binary SHA-256：`d37c2094e35c5bae0d3ea6506a9ec8d7c8557b749952d705e7e9c9dab3ebc3f7`；最终 runner SHA-256：`7a06bd248320d389e0bc65451c362f007e4365810f35f0ced8218894f10a12a1`。最终 binary 用于 `final-refactor-gpt/`、`final-refactor-gem/`。其他轮次先于辅助函数整理，日志保留各自实际执行位置，不声称全部由同一 binary 运行。源文件及证据哈希见 [evidence-manifest.json](measurements/ship-e2e-20261001/evidence-manifest.json)。

每轮关闭应用/HTTP server、数据库、SSH listener 和连接；runner 只停止它临时启动的既有测试容器，保留 volumes。最近完整清理输出确认 `veloxmesh-test-redis`、`veloxmesh-test-qdrant`、`veloxmesh-test-postgres` 均为 `false`，可核对各最终 `runner.log`。用户原先运行的本地 embedding 服务保留运行。

依据用户“所有测试通过且没有未解决问题才合并”的明确条件，**merge_allowed=false**。本次新增可复现失败证据与验收脚本，不修改 main，不把产品问题和未完成覆盖写成通过。
