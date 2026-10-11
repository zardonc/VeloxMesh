# 网关协议兼容性修复与真实回归

执行时间：2026-10-02 UTC / 2026-10-01 PDT。源基线：`c8849c98aae1a57c9c0b04e21e19896e82c33cd8`；分支：`codex/phase-27-stream-terminal-settlement`；修复保留在工作区。

**两项已确认协议缺陷已修复，并有真实调用链成功证据。整体验收仍不通过，不合并 main。** 本轮 23 个不同的 gateway/provider 用例均有成功记录，但重复执行中还出现一次 12 秒超时和一次 Gemini `provider_error`；不能把这些失败删除后宣称全通过。

## 修复范围

1. OpenAI-compatible 适配器把生成结束误当作传输结束。现在允许 `finish_reason` 后的一帧合法 usage-only 数据，再接受 DONE；拒绝重复 usage、负数或不一致统计、结束后的 content/tool choices。流读取逻辑移入 `internal/providers/openai/stream.go`，保持错误传播、取消和生命周期职责。没有放宽为忽略结束后所有数据。
2. Gemini 的 `Part.ThoughtSignature` 在通用 ToolCall、流式片段和客户端回传时丢失。现在使用 `extra_content.google.thought_signature` 携带原始签名的 base64 表示，经过共享 toolstream 状态、完整响应和续接请求重建原生 Part。对 metadata 做 copy-on-write，校验非空、严格 base64 与大小边界；日志不输出签名。

共享改动覆盖 `llm` DTO/协议规范化、OpenAI 完整响应规范化、Gemini 完整/流式转换、toolstream。没有增加工具执行或会话运行时。客户端必须原样保留并回传该字段；会丢弃扩展字段的第三方客户端没有在本轮验证。

测试入口另外修复两处可观测性问题：临时 PostgreSQL 启动后等待真实就绪（10 秒限额，失败逐次输出）；显式选择的 live 用例若 SKIP，runner 返回失败。所有新增/修改函数均维持仓库的长度、参数和复杂度限制；原有未触及函数不作为本次重构范围。

## 真实测试环境

使用 `App.New`、真实路由与认证、HTTP/SSE、配置发布、Redis、Qdrant、SQLite Usage/余额持久化，以及 `.env.local` 的 GPT/GEM provider。`httptest.NewServer` 承载真实 Router，不替换业务服务。PostgreSQL 仅检查测试环境就绪，不冒充此应用链路的存储后端。

本地模型仅用于 embedding：用户提供的 `http://127.0.0.1:1234`、`text-embedding-embedder_collection`，实测维度 768；远程测试经 SSH reverse forwarding 访问，未把本地服务用作聊天模型。聊天分别使用配置中的 `gpt-6-luna`、`gemini-3.1-flash-lite`。

本轮执行的用例没有 mock provider 或 mock 返回数据。非法签名用例先取得真实 Gemini 工具调用、完成真实客户端计算，再提交损坏的客户端字段测试 HTTP 400。没有调用返回伪造 embedding 结果的故障注入测试，也没有运行包含 mock 的全量后端套件。既有本地 embedding 压测见 Phase 29 历史报告，本轮未重复该负载测试。

## 修复前后对照

使用 Go build overlay 读取基线的三个适配/状态文件并隐藏新增 stream 文件，编译原解析器；不恢复、暂存或覆盖工作区文件。两种 build 均保留同一组真实 E2E 断言和共享 DTO，overlay 详情、源文件与 binary 哈希见 [source-manifest.json](compatibility-20261002/source-manifest.json)。

| 对照 | 原解析器 | 修复版 |
| --- | --- | --- |
| GPT 普通 SSE | `provider_bad_response` | 1 个 DONE、有效 content、Usage 一次结算 |
| GPT 工具 SSE + 续接 | `provider_bad_response` | 工具参数 → 客户端 19+23 → tool_call_id → 答案 42，结算两次 |
| Gemini 完整工具响应 | 签名从 HTTP 输出丢失 | 签名可回传，真实第二轮答案 42 |
| Gemini 工具 SSE | 签名从流式输出丢失 | 签名可组装回传，真实第二轮答案 42 |

原生 Gemini 控制组保留签名可以续接；仅移除签名时，同一真实上游返回 HTTP 400，确认这两项修复针对网关协议转换。原解析器的四条失败记录是预期的因果对照，未当作产品修复后的失败隐藏或覆盖。

## 用例与结果

| 范围 | GPT | Gemini | 验证内容 |
| --- | --- | --- | --- |
| 普通流式 | 通过 | 通过 | content、DONE、实际 tokens/credits/余额，一次结算 |
| 工具 required/named/auto/omitted/none | 5/5 通过 | 5/5 通过 | 实际模型工具参数、客户端计算、真实续接与两轮计费；none 无工具 |
| 工具流式续接 | 通过 | 成功记录 2 次，另有失败 2 次 | 签名保留、答案与结算；失败时序见下节 |
| buffered / Fusion | 2/2 通过 | 2/2 通过 | 真实规则启用、member/judge、SSE、Usage；Fusion 无配置费率时记录 missing_rate 且不扣余额 |
| 取消 / 认证 | 2/2 通过 | 本轮未重复取消/认证 | GPT 客户端取消不扣费且不损伤健康；真实认证拒绝 |
| 缓存完整链路 | 通过 | 通过 | 本地 embedding → 向量查询/异步持久化 → paraphrase 命中；两个异答案 miss；版本切换 miss |
| 非法签名续接 | 不适用 | 通过 | HTTP 400 invalid_request；仅保留第一轮 Usage，无额外扣费 |

Gateway 用例累计：**23 个不同 provider/用例有成功记录，25 次通过、2 次失败、1 次跳过**。跳过源于测试在读取 stdin provider 配置前检查类型，已修正并补跑通过；历史 SKIP 保留，不计为成功。

另有完全绕过网关的原生 SDK 控制：签名保留/移除对照通过；新增 native tool stream 的首响应 935.821 ms、完整响应 978.099 ms，通过，含一个真实带签名 FunctionCall。其后的网关工具流与续接耗时 1.71 秒，通过，两条结算记录共 201 tokens / 201 credits。

逐项状态、执行时间与日志哈希见 [test-results.json](compatibility-20261002/test-results.json)。关键日志：

- [GPT 原解析器失败](compatibility-20261002/gpt-baseline-ready/TestLiveStreamSettlement.log)、[修复版 GPT 汇总](compatibility-20261002/gpt-fixed/runner.log)。
- [Gemini 原解析器签名丢失](compatibility-20261002/gem-baseline/TestLiveToolStreaming.log)、[原生签名对照](compatibility-20261002/gem-baseline/TestLiveGeminiNativeContinuation.log)。
- [Gemini 全范围回归](compatibility-20261002/gem-fixed/runner.log)、[非法签名补测](compatibility-20261002/gem-signature-final/TestLiveToolInvalidSignature.log)。
- [最终原生流式对照](compatibility-20261002/gem-native-stream/TestLiveGeminiNativeToolStream.log)、[最终网关工具流续接](compatibility-20261002/gem-native-stream/TestLiveToolStreaming.log)。

## 保留的失败与限制

| 记录 | 结果与归属边界 |
| --- | --- |
| gpt-baseline/runner.log | 第一次环境启动在 PostgreSQL pg_isready 阶段退出，未执行模型请求；已修复有界就绪等待，后续真实组件启动与清理成功 |
| gem-signature-final/TestLiveToolStreaming.log | 第一轮流式请求在等待 HTTP 响应头时触及既有 12 秒客户端限额；未进入工具签名续接，不能把它归为原签名丢失问题。未记录完整阶段耗时，不能断言具体网关/网络/provider 责任 |
| gem-recheck/TestLiveToolStreaming.log | 第一轮 SSE 返回 provider_error；现有代码仅从原生 SDK APIError 状态映射到此码，本次签名/协议验证错误使用 provider_bad_response。没有记录原 API 状态，不能进一步断言配额或某个上游故障节点 |
| 最后一轮两项通过 | 证明两条调用链在该时刻可用，不能证明之前超时或 API 错误已消失；失败记录仍算未解决的可靠性问题 |

Qdrant 的 loopback 非 TLS 和 SDK clientVersion=Unknown 警告完整保留，没有关闭兼容性检查。真实向量链路通过不等于这些日志被消除。本轮不声称覆盖所有恶意上游尾帧、并行多工具或所有会过滤扩展字段的客户端。

历史 Phase 29 低命中性能门槛仍为 P95 on/off = 1.17518 > 1.05；本轮未重新测量或批准生产参数。SANS 的既有 429、Gemini 取消时序以及其他未完成验收项也未被本次协议修复关闭。**full_acceptance_passed=false，merge_allowed=false。**

## 静态验证与复现

通过：`go build ./...`、`go vet -tags phase29preflight ./...`、runner vet/build、tagged Linux test binary 编译、gofmt、`git diff --check`、新增/改动函数限制检查。没有适用的前端 package.json，不声称运行 pnpm build/typecheck。

复现时在仓库根用 Git Bash 构建，使用现有 runner 从 `.env.local` 读取 provider/SSH 凭据，不把密钥写入命令或报告：

```bash
GOCACHE="$PWD/.tmp/go-cache" CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go test -c -tags phase29preflight -o /tmp/veloxmesh-compat.test ./internal/app
GOCACHE="$PWD/.tmp/go-cache" go build -o /tmp/veloxmesh-acceptance-runner.exe \
  ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
mkdir -p .tmp/compat-recheck
SHIP_PROVIDER=GEM SHIP_ARTIFACTS=.tmp/compat-recheck \
SHIP_BINARY="$(cygpath -m /tmp/veloxmesh-compat.test)" \
SHIP_TESTS=TestLiveToolRequired,TestLiveToolStreaming,TestLiveToolInvalidSignature \
PHASE29_MODEL=text-embedding-embedder_collection PHASE29_EMBEDDING_API_KEY=local-test \
PHASE29_EMBEDDING_BASE_URL=http://127.0.0.1:11234/v1 \
PHASE29_READ_TIMEOUT=100ms PHASE29_READ_CONCURRENCY=4 \
PHASE29_WRITE_TIMEOUT=2s PHASE29_WRITE_WORKERS=2 PHASE29_QUEUE_CAPACITY=32 \
PHASE29_SHUTDOWN_GRACE=1s /tmp/veloxmesh-acceptance-runner.exe
```

所有应用用例内部 `-test.timeout 60s`，外部 GNU timeout 60 秒；HTTP 客户端仍为 12 秒。重现 GPT 时使用 `SHIP_PROVIDER=GPT` 并选择 `TestLiveStreamSettlement,TestLiveToolStreaming,TestLiveBufferedStream,TestLiveFusionStream`，不选仅适用于 Gemini 的非法签名用例。

每轮关闭真实应用、HTTP server、数据库、SSH 转发和连接；runner 只停止它临时启动的三个既有测试容器，保留 volumes。最后 runner 确认 Redis/Qdrant/PostgreSQL 均为 false；用户原本的本地 embedding 服务保留。证据目录凭据扫描通过。没有修改生产配置、提交、推送或合并。

协议修复的自动验证完成。按 gsd-debug 的人工作业确认要求，会话等待用户在真实客户端确认后再归档；这不解除原始整体验收与合并门槛。
