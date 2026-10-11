# Phase 29：embedding 复用及 Qdrant 并发创建修复复测

日期：2026-10-03。分支：`codex/phase-27-stream-terminal-settlement`。基线提交：`dc947268`。两项代码缺陷已修复，真实功能回归通过；低命中完整响应 P95 门槛仍失败，未合并 `main`。

## 修复范围

1. `LookupWithVector` 返回当前问题的有效 embedding，网关将它随 `CacheWrite` 入队，worker 直接复用。入队复制向量和 usage ID，避免后台读取调用方修改的数据。旧 `Lookup`/`Store` API 保留兼容；未获得向量时仍调用真实 embedding 服务，显式提供的无效向量报错。
2. 有效 embedding 完成后，通过容量为 1 的通道发布向量。如果后续 Qdrant 查询超时，仍可非阻塞取回已完成的向量。读取许可和 100ms 截止保留，不读取仍在运行的 goroutine 中的共享变量。
3. Qdrant 创建返回 `AlreadyExists` 时重新读取集合，只有确认维度与 Cosine 距离匹配才成功。已有集合也验证 schema；连接、权限、超时及 schema 错误继续返回。未使用本地锁遮掩多实例竞态。
4. `liveRequest` 补充真实 transport/read 错误文本，避免只记录 `Status:0`。测试 runner 增加可选 CPU/heap/block/mutex/trace 采集。

Lookup 的 goroutine/channel 模式保留。正常读路径中，扣除 embedding、向量搜索和仓库读取后的剩余开销 P95 约 **0.119ms**，它包含调度和检查，并不是纯 goroutine 成本；目前缺少优化此处的性能依据。

实际调用链为：HTTP → 请求规则 → 缓存 embedding/向量查询 → 路由 → admission → 主模型 → 响应规则 → 结算 → 写入入队。后台链路为：复用同一请求向量 → SQLite 写入 → Qdrant 建集合/Upsert。前后台及网络分段通过请求 ID 关联。

## 测试边界与可用性

使用实际 `App.New`、HTTP Router、身份认证/余额/用量结算、SQLite、Redis、Qdrant、Provider adapter 和模型服务。仅注入观测器及 HTTP trace，没有 mock 模型、mock 响应或 mock 存储服务。队列突发的种子回复来自真实主模型。

本地接口 `http://127.0.0.1:1234` 提供 `text-embedding-embedder_collection`（实际维度 768）及 `qwen2.5-0.5b-instruct`。VM 通过 SSH `11234` 转发到该接口。每次运行先检查 Redis PING、Postgres readiness、Qdrant `/readyz`，之后应用构造器再执行真实 embedding 探测。Qdrant 启动期间的连接重置日志保留，readiness 成功后才进入测试。

本地小请求可用性通过；SANS `oc/space-bunny-free` 的 256-token 小请求可用性通过。主要压力由本地模型承担；远程免费模型只做少量第二模型生命周期和向量复用请求。Gemini 及其他历史存在资源/协议问题的模型未重新纳入本轮放行。

## 真实回归结果

| 测试 | 结果与证据 |
|---|---|
| 单请求 miss → 后台写入 → hit → 结算 | 修复前 embedding HTTP=2、store_embedding=1，回归 FAIL；最终 embedding HTTP=1、store_embedding=0、vector_insert=1，随后命中、回复一致、命中零 token、仅一条真实结算，PASS |
| Qdrant 冷集合并发创建 | 修复前出现 `AlreadyExists`，且错误维度被接受；最终 4 个冷集合 × 16 个同时创建者全部成功，错误维度均拒绝，PASS |
| 队列突发与旧 Store fallback | 128 个候选，接收 33、queue_full 丢弃 95；入队 0.059ms、关闭 419.732ms；无创建冲突，关闭后拒绝入队，PASS。计数包含种子请求的异步写入，故 `store/stored=34` |
| 语义准确性及版本隔离 | 本地与 SANS 均通过真实 paraphrase 命中、不同答案 2/2 miss、相同账户/模型/仓库的 faq-v1→faq-v2 旧条目立即 miss |
| 分段计时与结算 | `TestLiveStageAccounting` PASS；前后台 request ID 与阶段记录完整 |
| embedding 压力 | 最终 100 个、并发上限 4、尽快发送；100/100 有效 768 维非零有限向量，P95 **35.934ms**、P99 **39.536ms**，全部在每调用 2 秒截止内 |

最终回归证据：[verified-correctness](measurements/fix-retest-20261003/verified-correctness/LOCAL-qwen2.5-0.5b-instruct/runner.log)。原始失败：[red](measurements/fix-retest-20261003/red/LOCAL-qwen2.5-0.5b-instruct/runner.log)。

第二模型首轮暴露了 embedding 已完成但搜索超时后向量丢失的边界：embedding 66.333ms，随后 vector_search 35.309ms，cache_read 101.810ms，后台再次 embedding。补上向量发布通道后，第二模型复用及完整生命周期复测通过：[second-model-final](measurements/fix-retest-20261003/second-model-final/SANS-oc_space-bunny-free/runner.log)。该组在去除内部冗余 enabled 检查及测试错误文本补充前执行；最终本地回归、性能和 profiling 使用同一个最终二进制。

SANS 同轮另有一次完整 FAQ 请求达到 10 秒客户端截止，网关记录 499。后来相同生命周期通过。该次失败没有上游完整分段或直连配对证据，**不能单凭后续成功断言根因在 Provider**；保留为间歇超时待跟进，不按成功覆盖历史失败。

## 最终匹配负载

设置与修复前稳定测试相同：8 RPS（每 125ms 发起），并发上限 4，每窗口 100 个，三组 `direct → off → on → on → off`。每个后端测试有 Go `-test.timeout 60s` 和外部 60 秒硬截止。真实输入、temperature=0、max_tokens=256、问题集、命中规则和隔离参数保持一致。

仅 `verified-block-1..3` 纳入最终统计：direct 300、cache-off 600、cache-on 600，**1,500/1,500 成功**；cache-on 命中 24/600（4%）。每组最差窗口调度延迟 P95 分别为 1.590/1.677/1.772ms，没有之前高负载的客户端积压现象。中间迭代及 profiling 不混入门槛。

| 指标 | 修复前稳定负载 | 最终复测 |
|---|---:|---:|
| cache-on embedding 调用次数 | 1,176（600 Lookup + 576 Store） | **600（后台 0）** |
| cache-off 完整响应 P95 | 124.289ms | 128.595ms |
| cache-on 完整响应 P95 | 159.482ms | **152.496ms** |
| on/off P95 比值 | 1.2832 | **1.1859，FAIL** |
| 前台 embedding P95 | 27.503ms | **21.458ms** |
| 总缓存读取 P95 | 29.485ms | **23.732ms** |
| 后台 Store 总耗时 P95 | 32.352ms | **5.458ms** |
| 网关耗时，扣除主模型及缓存读取，P95 | 5.398ms | **4.730ms** |
| 同一剩余耗时 >15ms 的数量 | 12/600，最高 93.014ms | **0/600** |

embedding 调用量下降 **49.0%**，Store P95 下降约 **83.1%**，cache-on 完整响应 P95 下降 **4.38%**。向量复用及调用量减少有直接请求级证据；跨轮主模型/机器波动存在，不能将全部耗时差或尖峰消失因果归于 P1。

最终三组比值分别为 **1.1408 / 1.1992 / 1.2182**，全部高于 1.05。以整组 ABBA 进行探索性 bootstrap 得到区间约 **[1.1408, 1.2182]**；只有三个独立组，不能把该区间当作充分统计保证。

## 剩余性能开销归因

- 前台 embedding P95 21.458ms；请求发送完成至首字节 P95 20.763ms；响应体读取 P95 0.293ms；JSON 解码/校验等相对 HTTP 的额外开销 P95 0.473ms。主要等待在 embedding 服务响应之前，包含 SSH/网络往返、服务排队和推理，**不是已测得的纯推理时间**。
- Qdrant 搜索 P95 2.366ms；路由 1.054ms；admission 0.393ms；结算 0.432ms；后台队列等待 0.231ms。当前负载不支持 admission 或写入积压是主要瓶颈的假设。
- cache-off P95 的 5% 仅约 6.43ms，前台缓存读取 P95 已达 23.73ms。各分段 P95 不能直接相加，但这种串行等待解释了为何删掉后台重复调用后，完整响应门槛仍未通过。
- 剩余等待多数发生在外部 embedding 调用，但将它串行放入请求路径是网关缓存策略产生的端到端开销。可把真实功能正确性及网关内部处理单独标为已验证；**不能据此宣布原有完整响应性能门槛通过**。

冷集合首次 Search 的 `NotFound` 仍按 `lookup/vector_error` 记录（最终六个 cache-on 窗口含 warmup 共 12 次）。主请求正常转发，已完成向量用于创建集合，582 个含 warmup 的写入均成功。该观测分类没有在本轮改为正常 miss，也不是本次 `AlreadyExists` 创建竞态。

## 最终 profiling

最终二进制独立运行 200 个 cache-on 请求，全部成功；不计入性能门槛。CPU/heap/block/mutex/execution trace 原件和 `pprof -top` 报告均已保存。

- 网关剩余耗时最高 **10.199ms**，没有 >15ms 尖峰。
- 21 次 STW（含 trace 启动），单次最高 **0.355ms**，累计 **2.117ms**。
- CPU 样本 1.43 秒，采集窗口 25.53 秒；本轮没有网关 CPU 饱和迹象。此采样不代表本地模型服务器/GPU 的使用率。
- mutex profile 加权累计约 **4.394ms**；trace 中 SQLite/SQL mutex 阻塞累计约 **0.55ms**。SQL connectionOpener 的长 select 等待是后台空闲，不应当作请求数据库锁等待。
- 未重现修复前 93ms 尖峰，故不能确定旧尖峰是 GC、OS/VM 调度还是其他共享资源。若重现，应在同一次请求上关联 trace，并补采 VM steal/CPU 及模型服务队列观测。

证据：[profiling-final-summary.json](measurements/fix-retest-20261003/profiling-final-summary.json)、[CPU](measurements/fix-retest-20261003/profiling-final/LOCAL-qwen2.5-0.5b-instruct/cpuprofile-top.txt)、[SQL blocking](measurements/fix-retest-20261003/profiling-final/LOCAL-qwen2.5-0.5b-instruct/trace-sql-top.txt)。

## 放行状态及复现

**不合并 main。** P1/P2 修复及真实功能回归已通过；原有低命中 P95≤1.05 门槛仍失败。旧尖峰根因未确认，SANS 一次间歇超时仍需关联证据。暂不做 P3 goroutine 优化，应先评估降低或避免前台 embedding 串行等待的策略及适用模型/场景门槛；这些属于后续设计决策，本轮没有自动更改门槛、生产参数或部署。

`go vet -tags phase29preflight ./...`、`go build ./...`、独立 runner 的 `go vet`、`git diff --check` 通过。新函数和文件满足复杂度/行数限制；旧公开兼容签名及既有校验函数的存量限制违例没有在本轮扩散修改。凭据审计通过。每次 runner 的退出日志确认自己启动的 Redis/Qdrant/Postgres 容器停止，SSH 转发和测试进程退出。

复现步骤、原始请求阶段日志、统计脚本、失败记录与二进制/源码校验值见 [复测目录](measurements/fix-retest-20261003/README.md)、[manifest.json](measurements/fix-retest-20261003/manifest.json)、[comparison.json](measurements/fix-retest-20261003/comparison.json)。
