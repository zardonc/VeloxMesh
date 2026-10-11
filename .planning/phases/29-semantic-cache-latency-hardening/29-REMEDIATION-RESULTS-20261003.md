# Phase 29 遗留问题修复与真实验证

日期：2026-10-03（America/Vancouver）。基线：`b2eb95ff`；分支：`codex/phase-27-stream-terminal-settlement`。本轮按 `$gsd-debug` 执行，并读取了引用任务「分析 Phase 29 遗留问题」。

**结论：四组已确认的实现缺陷已修复并通过对应验证，但 Phase 29 整体发布条件仍未满足。未合并 main，未启用生产缓存或修改生产白名单。**

## 1. 范围、环境与方法

- 修复前先测 VM 中真实 Redis、Qdrant、PostgreSQL，以及运行在 VM 中的实际 SQLite 仓库。修复后补测真实 pgvector，确认控制状态迁移与 768 维向量表共存。
- 编译 Linux Go 测试二进制，运行真实 `App.New()`、HTTP Router、认证、路由、admission、Provider adapter、缓存、SQL/Redis/Qdrant、usage 结算。观测器记录真实事件，透明 TCP 代理仅延迟或断开实际字节；无 mock 模型、响应或服务。
- 本地服务供 embedding：`text-embedding-embedder_collection`；第二 embedding：`text-embedding-nomic-embed-text-v1.5`，均实际返回 768 维。`qwen2.5-0.5b-instruct` 用于控制变量性能实验，不作为所有业务答案质量的保证。
- 云端配置来自 `.env.local`。先以真实完成与持久化结算确认 GPT `gpt-6-luna`、OR `inclusionai/ling-3.0-flash-sante:free`、OR `nvidia/nemotron-3.5-lightning:free` 可用，再分配协议与业务回归。Gemini/SANS 按用户指示跳过。
- 每个后端测试同时使用 Go `-test.timeout 60s` 与 VM 外层 60s 截止；本机包测试也有外层 60s。测试后 runner 日志均确认三个任务专用容器停止，SSH/代理连接结束；用户本地模型服务保留。
- 日志、失败样本、CPU/mutex/block/trace、二进制与源码 SHA 见 [证据目录](measurements/remediation-20261003/) 和 [manifest.json](measurements/remediation-20261003/manifest.json)。最终文件清单保留历史失败，不以最近成功覆盖旧日志。

## 2. 修复前组件性能验证

每个操作分别跑 C1/C4/C8/C16，每窗口 500 样本；下表为 P95，单位 ms。客户端在 VM loopback。仓库操作包含真实 cache entry 写入与读回、字节一致性校验；向量来自真实 embedding。

| 组件及操作 | C4 | C16 | 结果与限制 |
| --- | ---: | ---: | --- |
| Redis health SET+GET | 0.896 | 1.529 | 2,000/2,000 成功 |
| Redis limiter Lua | 0.479 | 1.049 | 2,000/2,000 成功 |
| Qdrant 查询 | 3.287 | 9.088 | 2,000/2,000 成功；查询时仅一个真实种子点 |
| Qdrant 确认写入 | 6.704 | 19.370 | 2,000/2,000 成功；与小集合查询不能推断生产 ANN 大库容量 |
| PostgreSQL cache Store+Read | 1.764 | 5.284 | 2,000/2,000 成功 |
| SQLite cache Store+Read，修复前 | 存在失败 | 存在失败 | 第二组 C4/C8/C16 分别 125/281/382 个 SQLITE_BUSY；首次 C4 为 137/500 |
| SQLite cache Store+Read，修复后 | 1.767 | 12.807 | 2,000/2,000 成功；C16 P99 57.534、max 91.819，锁竞争容量长尾仍存在 |
| pgvector 确认写入，补测 | 3.293 | 7.979 | 2,000/2,000 成功 |
| pgvector 查询，补测 | 6.026 | 18.459 | 2,000/2,000 成功；500 个实际向量行，向量内容相同，不能代表生产召回难度 |

pgvector 首次补测失败来自测试参数：仅设置维度，HNSW `m` 被默认到 1，服务器返回 `value 1 out of bounds for option "m"`。随后使用应用默认的 `m=16 / ef_construction=64 / search_ef=40` 复测通过；未将测试配置错误计为组件性能缺陷，也未修改生产参数。

本轮可以确认这些组件在上述负载与小规模数据集下的能力；不能证明生产最大吞吐、任意数据库规模、长时间稳定性或所有组件均不存在容量问题。部分 Qdrant 重启 readiness 出现 curl 52/56，重试后成功；启动证据与稳定窗口分开记录。runner 等待上限由 10s 改为 30s，后端测试截止仍为 60s。

## 3. 已修复的实现缺陷

| 缺陷与根因 | 修改 | 真实验证 |
| --- | --- | --- |
| SQLite 只对池中一次选中的连接执行 PRAGMA，新连接 `foreign_keys=0 / busy_timeout=0 / synchronous=2` | 在 driver DSN 中配置每个新连接的 PRAGMA，保留 WAL 初始化 | 修复前失败；修复后八个实际连接均正确，2,000 次写读零失败 |
| cache row 先启用，向量写失败留下不可用状态；Qdrant 随机点 ID 使同 entry 重放重复 | 先保存不可读快照，向量确认后单独 activation；仅语义缓存使用稳定 point ID | 向量断连期间没有 enabled entry；三个同 ID 写入只保留一个点；正常 hit 答案不变、零新增计费 |
| pending/过期数据没有完整终态 | 既有一个写 worker 每 10s、最多 32 项，在 write deadline 内先确认删除向量，再条件删除 SQL；失败保留依据并记录原始错误 | activation 前中断、应用重启、清理断连后重试、TTL miss 与两端清理全部通过；实际 PostgreSQL 清理也保留仍启用的行 |
| Redis health 在全局锁内无界网络同步，忽略错误；超时旧命令还能晚到覆盖新快照 | 短锁复制本地状态；每 key 有界同步；网络遵循 context；原子 Lua 按版本拒绝旧命令；读回只接受更新的快照 | 200ms 真实延迟导致的 post-provider 空档从约 202ms 降到 52–54ms；无关 Provider 从约 200ms 降到 5.49ms；C8 200 成功计数与晚到命令保护通过 |
| OpenAI-compatible adapter 拒绝 OpenRouter 的最终 usage choice | 允许唯一、无内容、无工具增量、与原终止原因一致的 accounting choice，输出 usage 而不重复终止 | OR 流式结算、Fusion、工具流/续接、原始尾帧诊断全部通过；GPT 标准流式与工具续接也通过 |

上表的“缓存生命周期”是一组修复，Redis 是另一组；合计四组实现问题（SQLite、缓存生命周期、Redis、流式兼容）。已有重复 embedding 修复与 Qdrant 冷集合创建幂等修复保留，本轮 `TestLiveCacheMissEmbeddingReuse` 和真实跨进程集合创建再次通过。

OpenRouter 实际帧：先发 `finish_reason`，随后发 content 为空、相同 `finish_reason`、带 usage 的 choice，最后 `[DONE]`。修复前表现为 HTTP 流已开始后 SSE `provider_bad_response`，对应三项失败。其 [官方 streaming 文档](https://github.com/OpenRouterTeam/docs/blob/main/api_reference/streaming.mdx)明确说明此 accounting 帧形式。重复 usage、新 content/tool delta、不同终止原因仍被拒绝；原有 OpenAI adapter 的错误路径包测试通过。

健康同步默认 50ms，同时用于读快照与单次 key 同步；Provider 读失败仍 fail-closed，错误写入日志。本轮未证明多节点全局计数合并、时钟回拨、混用旧版非版本化 writer 或 Redis Cluster 行为；当前使用 standalone Redis。清理重启测试是同 OS 进程内重建真实 App，不等价于 OS 强杀、机器掉电或跨节点重放。内置 SQLite/PostgreSQL 支持清理接口；自定义仓库须实现该可选接口。实际 App 的缓存绑定底层仓库，不受后续 replication wrapper 隐藏接口影响。

## 4. 分场景 P95 合同与结果

**诊断预算不替代生产 SLO，也不修改原 1.05 门槛。** gateway/application 指标定义为同一请求的 `http_handler - provider_complete - cache_read`，包含认证、路由、admission、健康同步、SQL/Redis 等控制状态 I/O及结算；它不是纯 CPU 时间。先逐请求相减，再计算分位数，不做“P95 相减”。cache_read 包含真实 embedding/Qdrant 等待，另外完整报告。

六个平衡时序区组，每个 direct/off/on 各 100 请求，交替执行顺序，共 1,800 请求。相同模型/问题规则，125ms 到达间隔，客户端并发上限 4；每组实际最大在途为 2。此窗口约 8 RPS，不是实际 C4 饱和容量证明。全部 HTTP 成功，on 总计 24/600 命中（4%）。

| 场景 | 完整响应 P95/P99 ms | 网关处理 P95/P99 ms | 本轮判据与结果 |
| --- | ---: | ---: | --- |
| direct，600 样本 | 123.972 / 157.192 | 不适用 | 同问题上游往返基线，保留波动 |
| cache-off，600 样本 | 129.034 / 159.276 | 4.774 / 5.861 | 正常处理诊断预算 P95≤10、P99≤15：通过 |
| cache-on，600 样本、4% hit | 157.763 / 184.106 | 4.950 / 7.298 | 本体诊断预算通过；**完整响应原 1.05 门槛失败** |
| cache bypass，实际 C4、100 样本 | 314.464 / 395.613 | 7.235 / 8.916 | 无缓存 I/O/无 hit；本体诊断预算通过 |
| warm hit，实际 C4、100 样本 | 37.532 / 46.558 | 0.952 / 2.920 | 同问题 off P95=258.148，收益门槛 min(110ms,0.5×off P95) 通过；零重复计费 |
| miss burst，实际 C8、100 样本 | 563.757 / 656.840 | 6.942 / 10.221 | 主响应全部成功；饱和完整响应指标单列，正式容量 SLO 待定 |
| bypass burst，实际 C8、100 样本 | 587.688 / 608.337 | 15.098 / 17.389 | HTTP 全成功，但不能套正常窗口预算判“性能通过”；本体长尾需容量目标约束 |
| bypass burst，C8 且全 profile、100 样本 | 396.695 / 413.557 | 10.634 / 12.751 | instrumentation 与时序不同，不能替换上行未采样窗口 |
| warmed embedding 单独压力，C4、100 样本 | 51.024 / 56.076 | 不适用 | 100/100 成功、768 维一致；先串行实际探测，再 16 个并发预热，不是冷加载测试 |

低命中 pooled P95 比值 **1.2226467**。六组分别为 1.254/1.048/1.162/1.318/1.226/1.149；2,000 次联合时序区组 bootstrap、seed=29，探索性 95% 区间约 **[1.103,1.253]**。仅六个独立区组，区间精度有限，且请求不是同时发出的随机配对；因此既不声称严谨因果效应，也不选一个成功区组改判总体结果。

正式评测建议：off/bypass 用完整响应 SLO+本体预算；eligible miss 用完整响应 SLO+真实查找成本+成功率；warm hit 用完整响应收益+答案一致+零新增计费；冷启动/故障用截止、降级和恢复期限；饱和/突发用实际到达率、在途、发送积压、背压和成功率；SSE 用首个有效事件与完整终止、usage；工具用每轮与全流程耗时。目标 RPS、业务完整响应 SLO、最低收益及召回目标仍需明确，未自定生产标准。

## 5. 请求分段与瓶颈证据

同 request ID 将 HTTP handler、Provider、cache 操作、HTTP transport 的连接/写请求/首字节/读完，以及后台 entry/write ID 关联。新增 health provider、circuit result、health model 段；使用可注入 observer、HTTP RoundTripper/httptrace 与有限业务边界标记，避免侵入各业务算法。

| 稳定 cache-on 段 | P95 ms | 含义 |
| --- | ---: | --- |
| cache_read | 23.643 | 前台总查找，包括 embedding 与向量查询 |
| lookup_embedding | 21.906 | 模型 HTTP 往返与适配，包含网络、服务排队、推理；不是纯推理计时 |
| vector_search | 2.316 | 真实 Qdrant 查询 |
| repo_read（24 hits） | 0.277 | 候选 SQL 验证 |
| admission（576 misses） | 0.373 | 本轮请求准入操作；缓存读许可为 fail-fast，不是等待队列 |
| health_provider_sync | 0.943 | Provider outcome 同步 |
| health_model_sync | 0.642 | 模型 outcome 同步 |
| settlement | 0.390 | usage/余额结算 |
| provider_complete（576 misses） | 136.752 | 主模型往返，包含上游内部等待及传输 |

各段是独立分位数，不能相加等于端到端 P95。600 个 on 请求的本体 max=10.769ms，未再次出现 >15ms 尖峰；这不证明历史自然尖峰唯一来自 Redis。正常 off post-provider P95=1.602ms，on miss=1.512ms。

C8 profile 窗口：4.68s 内采到 390ms CPU；mutex 等待累计约 12.24ms，database/sql QueryContext 路径约 8.58ms；scheduler delay profile 主要在测试到达/取消调度，GC 样本较小。聚合 profile 支持继续查看数据库池/调度，**不足以把某个历史尖峰归因为 GC、SQLite 锁或 OS 调度**。block 的几百秒是多个 goroutine 等待之和，不能解读为单请求卡住几百秒。CPU、mutex、block、trace 原件及导出文本保留在证据目录。

当前低命中增量不能全部归为网关 CPU：真实 lookup 有串行模型往返，且主模型完整响应存在跨轮波动。也不能直接把真实 embedding 往返等同于 Provider 推理或归因网络；尚无模型服务端排队/推理/GPU 资源计时。代码修复没有让 1.05 门槛通过，不以降低命中或吞掉错误达成门槛。

## 6. 多 Provider 分流与错误归属

实际三个模型、每模型并发上限 1、总上限 3，off/on 各 24 请求。三模型分别预热确认成功后发压。off 23/24 成功，on 24/24 成功。目标 4 RPS，但实际完成窗口约 31.81s/28.91s，发送受模型许可与客户端上限背压；样本不足、负载未达目标，不能作为网关最大吞吐或全场景 P95通过依据。

- Nemotron 的 off 样本 index=3：客户端约 12,002ms 超时等待网关 headers；关联网关 handler=12,004.836ms，Provider=11,999.778ms，本体约 5.058ms。连接复用取得约 0.023ms，写请求约 0.225ms，上游首字节=117.798ms，完整 body=11,999.637ms。等待定位在上游 body 路径，不是认证、路由、admission 或健康锁；上游推理/排队与 body 传输之间仍需 Provider 后台证据。
- 最初 Nemotron required-tool 第二轮也等待完整上游 body：首字节约 113ms、Provider 约 12,000ms、本体约 2.882ms。直连 required-tool 全流程 9.24s 成功，网关复测 7.81s 成功；未发现需修复的协议不兼容，但原超时保留。
- 针对失败 FAQ 的 direct 对照被该 Provider 的 **HTTP 429** 阻断，发生在 warmup，未获得三个正式 direct 样本，也未捕获其错误 body 的具体限额说明。确认是上游直接状态；不猜测是并发、分钟额度还是每日资源上限，也不宣称直连复现了相同 12 秒超时。
- OR Ling 三项 SSE 失败已由原始尾帧和官方协议确认是网关缺陷，并修复验证；与上述 Provider 等待/429 区分记录。

因此，多 Provider 分流方向合理，但共享同一上游聚合平台、模型单请求长尾、全局限速和客户端 semaphore 都可能限制压力；必须记录实际负载，并按 Provider/model 分层，不能仅降低每模型并发便认定网关已承受目标压力。后续资源恢复后，先看后台 request ID/队列与响应生成，再用有界真实对照。

## 7. 语义与业务准确性

- collection 现有策略/0.92：等价问法 **2/50 hit**，100 个不同答案/不适用问题 **0 false hit**；50 个响应均成功，miss 主模型按请求结算一次。正例诊断失败。
- 两 embedding 的实际 App 写入→命中→切换新 scope→旧模型恢复旧 hit 生命周期通过，答案与零 token cache usage 保持。二者都 768 维，不代表实际跨维度模型切换。
- OR Ling 实际直连业务 FAQ 与网关业务 FAQ/cache 计费通过。本地 qwen 先前直接也答错的取消政策不因缓存基础设施修复自动变好；本轮不据其性能成功签全业务质量。
- Nomic 前缀只读对照 453 次真实向量调用，使用已见 50 正例/100 负例；没有修改生产 embedding 输入或阈值。其 [模型卡](https://huggingface.co/nomic-ai/nomic-embed-text-v1.5)规定任务前缀，clustering 用于包括语义去重的聚类；但本地服务是否已自动加前缀、实际模型文件与 pooling 仍未取得服务端证据。

| Nomic 输入，阈值 | 正例 hit /50 | false hit /100 |
| --- | ---: | ---: |
| raw /0.92 | 9 | 0 |
| clustering /0.92 | 32 | 0 |
| search_query /0.92 | 17 | 0 |
| raw /0.86 | 38 | 0 |
| clustering /0.86 | 49 | 18 |
| search_query /0.86 | 38 | 0 |
| search_query /0.84 | 42 | 0 |

这说明输入策略改变分布，原阈值不能机械复用；即便已见负例零误命中，也不是独立留出集结论。未确定业务召回目标，未批准将候选前缀/阈值写入生产。后续应确认服务实际输入，按基础意图划分校准/留出，加入多 FAQ 种子、近邻混淆和歧义，冻结 representation/version 后再测真实网关，避免混用旧向量。

## 8. 最终状态与复现

最新 61 个“测试名×目标模型”记录中 58 通过，3 未通过：collection 正例诊断、三模型 off 分流（Nemotron 超时）、Nemotron direct（HTTP 429）。另一个独立发布条件是 pooled 完整响应比值，仍失败。以上统计不是全部 Phase 27–29 需求覆盖计数，也不将历史 expected failure 排除后写成全绿。

最终八项边界回归均通过：Redis 晚到写入/并发计数/跨 Provider 隔离/Provider 后延迟、缓存 pending 可见性/同 ID 重放/重启清理/过期清理；embedding/Search 网络截止、真实 Qdrant 跨进程、缓存向量复用、认证与原始错误分类均通过。队列 128 候选 accepted=33、drop=95，关闭约 443ms，未发生集合创建冲突。Phase 27/28 已分配的 streaming、取消、buffered、Fusion 与工具模式均有最新通过记录；Gemini/SANS 不在本轮复测范围。

`go vet -tags phase29preflight ./...`、`go build ./...`、SQLite/health/hotstate/OpenAI 既有包检查通过。包检查是辅助回归，不代替真实 E2E。`git diff --check` 无 whitespace error。新拆分 helper 通过本地复杂度检查；原有构造器/Store 接口的四参数契约、OpenAI Complete/Embed 的既有复杂度与仓库旧大文件未在本轮扩大为全库重构。

剩余事项：原低命中成本门槛；语义召回与独立业务验收；上游 body 长尾/429；高并发数据库与网关容量 SLO；真正冷加载与进程强杀/多节点场景证据。各项有现象和边界，不能把所有问题笼统归为 Provider。**当前保持不合并 main。**

复现入口（Git Bash；配置/凭证继续来自本地 env）：

```bash
export GOCACHE="$PWD/.tmp/diagnostic-cache"
export GOTMPDIR="$PWD/.tmp/diagnostic-build"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags phase29preflight -c \
  -o .tmp/phase29-remediation-20261003/delivery-linux.test ./internal/app
go build -o .tmp/phase29-remediation-20261003/runner-wait.exe \
  ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
export SHIP_RUNNER=.tmp/phase29-remediation-20261003/runner-wait.exe
export SHIP_BINARY=.tmp/phase29-remediation-20261003/delivery-linux.test
export SHIP_RESULTS_ROOT=.planning/phases/29-semantic-cache-latency-hardening/measurements/remediation-20261003/replay
node .planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/run.mjs \
  LOCAL qwen2.5-0.5b-instruct TestLiveSQLiteConnectionPragmas,TestLiveSQLiteCapacity component 100 125 4
uv run python .planning/phases/29-semantic-cache-latency-hardening/measurements/remediation-20261003/analyze.py
uv run python .planning/phases/29-semantic-cache-latency-hardening/measurements/remediation-20261003/evaluate.py
```

runner 串行执行，避免测试容器与隧道端口互相影响。其他场景/输入/协议用例和精确参数见 `final-suite.mjs`、`mixed.mjs` 与对应 dispatch/runner 日志。原 final-suite 调度器退出码曾未汇总各批次失败；已补正为任一批次失败即非零，报告依据每项原始 PASS/FAIL而不是该旧退出码。分析脚本按 nearest-rank 分位数重算，保留失败和无效基线，不只挑成功请求。
