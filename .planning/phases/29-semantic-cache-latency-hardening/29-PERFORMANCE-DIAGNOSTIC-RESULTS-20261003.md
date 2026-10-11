---
phase: 29
tested_date: 2026-10-03
status: real_e2e_completed_with_open_findings
performance_signoff: false
merge_allowed: false
---

# Phase 27–29 真实链路与分段性能复测

本轮完成模型预检、真实组件验证、分段采集、本地重复性能对照、远程模型分发及协议验证。**不合并 main**：低命中 P95 门槛未通过，并确认了网关侧 Qdrant 冷集合并发创建竞态；部分上游模型还存在协议、限流和响应超时问题。

原始日志与可复算数据位于 [diagnostic-20261003](measurements/diagnostic-20261003/)。核心结果：[稳定对照](measurements/diagnostic-20261003/stable-summary.json)、[各轮结果](measurements/diagnostic-20261003/summary.json)、[网关延迟尖峰](measurements/diagnostic-20261003/gateway-outliers.json)、[执行清单](measurements/diagnostic-20261003/manifest.json)。

## 范围与真实环境

使用真实 `App.New()`、HTTP router、鉴权、路由、admission、Provider adapter、SQLite 结算、Redis、Qdrant 和本地模型。网关运行于隔离测试主机；本地模型经 SSH 反向转发调用，直接调用和网关调用使用相同拓扑。本地对照排除了公网聊天 Provider，但仍包含 VM/SSH 网络，不能称为零网络开销。

Embedding 使用用户指定的 `text-embedding-embedder_collection`，实测 768 维；本地聊天模型为 `qwen2.5-0.5b-instruct`。其他模型与凭据来自 `.env.local`。未改动 `.env.local` 或生产配置，缓存仍只对临时非管理员测试身份和静态 FAQ 场景启用。PostgreSQL 做了 readiness 检查，本轮业务持久化使用 SQLite，不能把 `pg_isready` 当作 PostgreSQL 业务链路验收。

Redis PING、PostgreSQL readiness、带鉴权的 Qdrant `/readyz` 及真实 embedding 维度探测均在执行入口检查。原 runner 只检查容器 Running，部分早期用例因此切换到真实 Redis VSS fallback。已补 Qdrant readiness 并复跑相关用例；最终稳定性能组、第二模型生命周期和最终诊断未触发 Qdrant fallback。早期降级日志保留并明确标记。

所有后端实际测试采用 `-test.timeout 60s`，远程进程另有 60 秒硬限制。性能窗口单次 100 样本，不以超长单进程绕过时限。runner 每次退出停止本次启动的隔离组件、关闭转发，并验证三个容器均为停止状态；未停止用户本地模型服务。

## 模型预检

共实际探测 19 个候选。首次短输出预检 15 个通过；两个推理模型在提高输出预算后也通过真实网关及结算检查，最终 **17 个通过、2 个受阻**。可用只代表该请求配置在检查时可完成，不代表持续容量或所有工具/流式能力均可用。

| Provider | 实际通过的模型 | 限制/失败 |
| --- | --- | --- |
| LOCAL | `qwen2.5-0.5b-instruct` | 用于主要性能负载，减少远程额度使用 |
| OR | `stealth/space-bunny-alpha`、`nvidia/nemotron-3-ultra-550b-a55b:free`、`poolside/laguna-s-2.1:free`、`nvidia/nemotron-3.5-lightning:free`、`inclusionai/ling-3.0-flash-sante:free`、`dots-studio/dots-3-note-preview:free` | `thinkingmachines/inkling:free` 直接 403，仅对指定 Agent harness 开放 |
| SANS | `oc/big-pickle`、`oc/mimo-v2.6-flash-free`、`oc/nemotron-3.5-lightning-free`、`oc/nemotron-3-ultra-free`、`oc/space-bunny-free`、`oc/longcat-2.5-preview-free`、`oc/mimo-v2.5-free`、`oc/fledge-alpha-free`、`oc/ling-3.1-flash-free` | 部分模型在较复杂场景有超时或流式协议问题，见下文 |
| GPT | `gpt-6-luna` | 主要用于少量工具和流式正确性验证 |
| GEM | 无 | 默认 `gemini-3.1-flash-lite` 网关等待响应头超时；直接 native SDK 返回 504 `DEADLINE_EXCEEDED` |

`dots-3-note-preview` 和 `fledge-alpha` 的 32-token 请求在直接上游同样返回空正文、`finish_reason=length`；256-token 请求有正文，随后真实网关及结算验证通过。因此首次失败属于测试输出预算不足，不能归为网关丢正文。

按先前暂跳 Gemini 的约定，本轮仅对默认模型做小范围复查，其余 10 个 Gemini 配置项未调用，不能宣称可用。当前直接 504 证据也不能单独证明是额度耗尽。

## 分段测量

采用可选观测接口、测试侧 HTTP middleware/`RoundTripper` 装饰器和标准库 `httptrace`。记录器只在显式启用的真实测试中安装；生产默认 metrics 不收集分段样本。无需引入 Java AOP、编译器织入或新的采集服务。

同一 `request_id` 关联 HTTP handler、请求规则、缓存读取、embedding、向量搜索、路由、admission、主模型调用、响应规则、结算和入队。异步 worker 使用原请求 ID，单独记录入队入口至 worker 开始、store embedding、SQLite 写入和向量写入。异步总耗时不加进前台响应耗时。

网络事件区分连接获取、DNS、TCP、TLS、请求写出、首字节、响应头和正文消费；正文消费包含解码器读取期间的时间，不能直接称为纯网络传输。TTFB 中还包含上游排队、推理及网络等待。缓存读取是 embedding/向量/仓储子阶段的父区间，不能重复相加；组件 P95 也不能相加或从端到端 P95 直接相减。

流式另记录客户端首个非空内容/工具事件、finish、usage 和 `[DONE]`，区分响应头与实际有意义输出。GPT 普通 SSE 实例：上游响应头 1423.9 ms、客户端有意义输出 1765.0 ms、DONE 1876.2 ms；这几个时钟起点不同，不能直接相减当作网关缓冲时间。流式完整终止与结算按原断言独立验证。

真实请求的分段关联及前台区间不重叠校验通过。现有 gateway OTel span 的结束时机和 latency 属性未作为本轮总耗时依据。尚未独立量化诊断采集本身的开销，本轮所有开关对照使用同一采集方式；报告不是未埋点生产 SLA。

## 本地性能对照

先执行 12.5 请求/s、每轮 100 样本、客户端上限 4、三个 `direct → off → on → on repeat → off repeat` 组，1500/1500 完成。但第三组 direct 与 on 出现 P95 调度滞后约 610/384 ms，不能拿这两轮做固定到达率性能验收；其余原始结果保留。

随后降低到 **8 请求/s**，保持问题集、模型、缓存参数、客户端上限 4 与顺序不变，每组每窗口 100 样本，共三组。direct 300/300、off 600/600、on 600/600 成功；on 每窗口 4 次命中，整体 4% 命中率。实际吞吐与调度信息可从 JSON 复算，三组最大窗口调度滞后 P95 分别约 5.676、1.728、1.649 ms，未出现持续积压。

| 组 | off 200 样本 P95 ms | on 200 样本 P95 ms | on/off |
| --- | ---: | ---: | ---: |
| 1 | 128.670 | 211.290 | 1.6421 |
| 2 | 123.523 | 150.388 | 1.2175 |
| 3 | 122.472 | 150.739 | 1.2308 |
| 合并 600/600 | 124.289 | 159.482 | **1.2832** |

原门槛仍为 `on P95 <= 1.05 × off P95`，三组和合并结果均未通过。第一组有明显局部尖峰；后两组仍失败，结论不依赖第一组。分块 bootstrap 仅辅助描述不确定性，独立环境块只有三个，不能替代容量验证。

稳定组 cache on 的分段 P95（按每个请求先计算阶段/余量，再求分位数）：

| 阶段 | P95 ms | 解释 |
| --- | ---: | --- |
| 网关处理，排除主模型及缓存读取 | 5.398 | 包括鉴权/解析、规则、路由、admission、结算、响应处理及 handler 剩余工作 |
| 缓存读取 | 29.485 | 真实同步关键路径 |
| lookup embedding | 27.503 | 是读取成本的主要部分 |
| 向量搜索 | 2.780 | 相对较小 |
| admission | 0.377 | 非主模型并发许可排队；本轮 Scheduler 关闭 |
| 路由 / 结算 | 1.499 / 0.401 | 普通请求开销较小 |
| 异步写 worker 等待 / store total | 0.241 / 32.352 | 不计前台时间；store embedding 为主要后台成本 |

前台 embedding 发出请求后的首字节等待 P95 26.868 ms，响应头后正文消费 P95 0.299 ms。第一组 12.5/s 对照中主模型连接获取 P95 < 0.3 ms、正文消费 P95 < 0.14 ms，耗时主要在请求发出后等待响应。数据不支持把“正文网络传输慢”作为主要解释，但未拆出上游内部推理与排队，也未量化 SSH 路径的 RTT。

**不能据此把全部失败归给 Provider。** 稳定组有 12 个网关剩余处理区间超过 15 ms；最高 93.014 ms（`stable-block-1/.../TestLiveCacheLoadOnRepeat.log` 的 `diag-79`），其中请求规则开始前约 45.451 ms；另有路由阶段约 31 ms 的尖峰。原因可能涉及进程/VM 调度、资源争用、鉴权/仓储访问或观测开销，尚无 CPU/GC/调度剖析证据。两次 lookup 超时同时出现在这个局部窗口。

每个 cache miss 在读取与异步写入各调用一次相同问题的 embedding，这一点已由同请求 ID 的实测调用确认。后台重复推理可能增加本地服务压力；embedding 与聊天模型是否共享 GPU/CPU及具体竞争程度尚未证实。后续可以验证复用读取向量、独立 embedding 资源，以及同步读取预算与端到端门槛的合理性，不能直接据此次测量更改生产参数。

## Embedding 压力与缓存准确性

| 真实 embedding 压力 | 成功/尝试 | P95 ms | 说明 |
| --- | ---: | ---: | --- |
| 并发 4，首次 | 200/200 | 37.225 | 768 维，有限值且非零向量 |
| 并发 8 | 200/200 | 189.693 | 全部在 2 秒单请求截止内完成，不能当作通过 100 ms 读取预算 |
| 并发 4，后续 | 100/100 | 91.287 | 证明跨轮延迟会变动；不是与并发 8 的配对因果实验 |

本地 Qwen 和 SANS `oc/space-bunny-free` 均通过真实缓存生命周期：语义改写命中并保持真实答复一致、两个不同答案问题不误命中、同身份/模型/仓储在知识版本切换后旧条目失效。各自 warmed-hit 100/100 成功，不能替代低命中性能门槛。这验证的是网关缓存行为，不是通用模型知识准确率评测。

真实答复生成的 128 次写队列突发：33 次接收、95 次按容量拒绝，入队约 0.088–0.092 ms，关闭约 413–456 ms。**发现一个向量写入失败，最新回归用例明确 FAIL**，见下一节。

初次复用的旧故障注入测试、旧固定答复 burst 及相关早期 PASS 日志不计本轮纯真实验收。固定答复已改为从实际网关/主模型获得；故障注入日志仅作附加历史诊断，不能充当真实上游异常证明。

## 已确认问题及归因

| 问题 | 证据与归因 | 当前状态 |
| --- | --- | --- |
| Qdrant 冷集合并发创建 | 真实模型答复、真实 embedding/Qdrant下，`vector_insert` 返回 `AlreadyExists`。`QdrantVectorAdapter.EnsureCollection` 的存在检查与创建分离，两个 worker 可同时检查“不存在”，随后一个创建失败。属于网关存储并发缺陷，不是模型问题 | **未修复，新增真实回归 FAIL**；[失败日志](measurements/diagnostic-20261003/real-burst-regression/LOCAL-qwen2.5-0.5b-instruct/TestPhase29LocalQueueBurst.log) |
| 冷集合读取错误分类 | readiness 已通过时，首次 vector search 确认返回 `NotFound`，网关 fail-open 后真实主模型成功。与服务未就绪是两件事 | 未处理；需要区分预期冷 miss 与真正向量故障 |
| 本地低命中 P95 | 8/s、600/600 对照比值 1.2832；同步 embedding 是主要新增阶段，并有网关剩余处理尖峰 | 原性能门槛未通过；尖峰原因未确定 |
| SANS Mimo 工具 SSE | 直接上游两次请求均出现 `done=2`；诊断记录 16 个 frame、两次 `tool_calls` finish、两次 usage。网关报 `provider_bad_response` 有直接协议证据 | 上游重复终止问题未解决；不能放宽网关校验来隐去 |
| Poolside | 直接 named-tool HTTP 429，返回 `limit_source=upstream_provider_shared_pool`，共享池暂时限流 | 上游资源限制，分散到不同模型仍不能保证规避共享池限制 |
| Inkling | 直接 403，错误说明只对指定 Agent harness 开放 | 上游访问策略，当前普通网关场景受限 |
| Gemini 默认模型 | 网关响应头超时，直接 native SDK 504 `Deadline expired before operation could complete` | 上游 native 操作未完成；具体资源原因未确定 |
| 远程长尾 | OR Nemotron 完整 FAQ/工具场景直接调用及网关均有 12 秒超时；SANS Mimo 2.5 24 样本组存在多次超时、一次 60 秒进程截止和显著客户端积压 | 远程独立性能窗口无效，不能用只成功样本的 P95宣布通过 |

冷集合创建的幂等处理应在确认已存在集合的 schema 与维度一致后设计，不能无条件吞掉 `AlreadyExists`。本轮只增加计时、测试与错误暴露，未修改存储创建语义或上游协议容错策略。

## 分发实验与协议流程

从可完成完整 FAQ 请求的候选重新选择五个模型：SANS Mimo 2.6、SANS Space Bunny、SANS Longcat、OR Ling 和本地 Qwen。每模型并发上限 1，总上限 5，off/on各 40 个计划到达请求，间隔 700 ms。

off **40/40**；on **39/40**。失败为 Space Bunny 等待响应头 12 秒超时：同一 `diag-17` 的缓存读取 20.825 ms，主模型调用 11977.595 ms、handler 12001.974 ms，明确不是路由/admission/缓存读取消耗了这 12 秒。它把问题定位到网关发出的上游 HTTP 调用，但没有证据再细分 Provider 内部处理与公网丢包/网络等待。

两个窗口调度滞后 P95 约 2.0/7.0 秒，是测试客户端受每模型限并发和长尾约束的积压，不能标成网关队列等待。分模型每格只有 8 样本，既不使用混合 P95作 cache gate，也不据此声称找到了网关最大吞吐。

分配到不同 Provider/模型的实际业务检查中，GPT 的普通 SSE、required/named/auto/omitted 工具调用、客户端计算 19+23、`tool_call_id` 续接、最终 42、工具 SSE 和结算余额断言通过；SANS Longcat buffered stream、SANS Space Bunny Fusion stream、OR Ling tool-choice none/鉴权通过；本地 Qwen 断连取消且无错误结算、provider health 不受影响的断言通过。其他模型的小请求可用性不自动推导其工具/流式可用性。Gemini signature 续接本轮因资源未成功复验，沿用既有文档要求，不新增服务端签名保存。

## 可重复执行与检查

复用并扩展 [runner](measurements/acceptance-20261001/runner/main.go)：增加 OR、`SHIP_MODEL`、多模型配置输入、Qdrant readiness 和账户标识脱敏。密钥由 `.env.local` 读取，经 SSH stdin 传递，未放到命令参数或源码。raw artifact 凭据审计通过。

本地 `.tmp/phase29-diagnostic-20261003/run.mjs` 是本次无凭据输出的调用包装器；`final-suite.mjs` 保留实际分发与窗口参数。复跑可先用包装器选一个模型执行 `TestLiveModelAvailability`，再执行 `TestLiveDirectLoad,TestLiveCacheLoadOff,TestLiveCacheLoadOn,TestLiveCacheLoadOnRepeat,TestLiveCacheLoadOffRepeat`，参数为 `100 125 4`。跨组需要不同 artifact label，避免覆盖原始失败。

重算命令（从仓库根执行）：

```bash
uv run .planning/phases/29-semantic-cache-latency-hardening/measurements/diagnostic-20261003/analyze.py
uv run .planning/phases/29-semantic-cache-latency-hardening/measurements/diagnostic-20261003/analyze-stable.py
```

源码检查：`go vet -tags phase29preflight ./...`、`go build ./...`、真实测试二进制编译及新增 helper 复杂度检查通过。未使用 mock 单元测试替代本轮 E2E；普通未 opt-in 的 live 测试会 skip。部分早期被替换的二进制未在执行时留存 hash，因此不把最终 binary hash冒充所有历史轮次版本；manifest 保留当前和预检 binary hash及该限制。

**合并状态：禁止。** 两个真实主模型的缓存生命周期已有证据；剩余阻碍包括网关 Qdrant 创建竞态、性能门槛/本地尖峰及适用上游场景的不稳定。不能因为多数耗时在模型调用，就把当前所有测试改判为通过。
