# Phase 29 可用性与尾延迟继续排查 — 2026-10-04

**已确认并修复：健康状态读取失败被误报为上游不健康。** 真实 Redis 延迟注入在修复前稳定复现；修复后 HTTP、五种路由、恢复及健康 peer 测试通过。自然 Redis 超时的根因和历史 LM Studio 400 尚未确定。新诊断窗口 residual P95 回到约 4.4–4.6ms，说明上一轮 13–27ms 不能视为网关固定下限；单窗口也不能证明性能验收或“1.05 物理上绝对不可能”。

本报告续接 [上一轮策略与保护结果](29-POLICY-EXECUTION-RESULTS-20261004.md)，保留历史失败与其代码指纹。HEAD 仍为 `780e12e`，本轮 binary 使用有未提交修改的工作区；具体来源、binary 和原始证据 SHA-256 见新证据目录的 [manifest](measurements/availability-investigation-20261004/manifest.json)。未部署、未启用生产缓存、未调整生产白名单、未删除既有 collections/volumes、未卸载共享模型。

## 确认的问题与修正

真实 Redis store 原本在 GET 超时或 JSON 解码失败时返回 `StatusUnhealthy`，路由把它当成模型健康事实，向调用方报告 `no_healthy_provider`。读截止是既有 50ms；注入 60/100ms 回复延迟时，健康和不健康状态均在约 51ms 被拒绝，移除延迟后恢复原状态，未改变 pending/failure 计数。40ms 注入仍成功。

本轮先写真实 Redis TCP 延迟和认证 HTTP 测试，再修正实现；`baseline-network` 保留原实现的失败。五路测试首次存在缺少 Combo.Name 的 fixture 错误，原失败保留于 `routes-red`；修正 fixture 后，通过 Go overlay 编译保存的旧实现，`routes-corrected-red` 再次证明 normal、override、round-robin、capacity、Fusion 五路均误分类。overlay 不撤销工作区修改。

修正通过 `ProviderSnapshot.ReadError` 区分读取失败；该字段不序列化、不持久化、不污染模型错误或失败计数。没有可读取的健康候选且存在读取失败时，返回 **503 `health_state_unavailable`**；可读取的健康 peer 仍可选用，真实不健康仍拒绝。原日志保留底层原因，公共响应只暴露稳定类别。错误不触发 provider 自动重试或健康降级。50ms deadline 和 fail-closed 行为保留，因此这是错误分类修复，**并未消除依赖停顿造成的 503**。

## 本轮验证

| 证据 | 结果 | 覆盖及限制 |
| --- | --- | --- |
| `baseline-network` / `routes-corrected-red` | 预期 RED | 健康状态可恢复，但旧实现把依赖读取失败当成 provider unhealthy；五路均复现 |
| `green` 的 11 项选定真实测试 | 全部 PASS，无 SKIP | 延迟矩阵、HTTP、五路、健康 peer、Redis 隔离/并发/旧发布、SSE/取消/缓冲/Fusion |
| `final-backend` | 595 顶层测试 / 含 subtests 954 条 PASS，38 有测试 package PASS，0 FAIL | 2 项既有 Plan4 opt-in 测试 SKIP；另 4 个 package 无测试，不计作通过的测试 |
| 三项真实 Phase 29 acceptance | 全部 PASS | 正常缓存/version、embedding 故障、真实队列及关闭 |
| `go vet -tags phase29preflight ./...`、`go build ./...` | PASS | Go 格式和 diff 检查通过；后端执行均有 60 秒硬上限 |
| `workload` off/on | 200 请求，0 HTTP 失败 | 两个初始 Qdrant NotFound 查找错误仍保留，见下文 |
| LM Studio 直连重放 | 160 正常请求 0 失败，8 主动取消单列 | 未复现历史 400；不证明间歇性错误不存在 |

真实 HTTP 与协议保护使用已有隔离测试配置：first-byte/content/idle 各 5s、total 30s、主模型 capacity 4。它们不是新增生产默认。隔离 runner 每次恢复原先停止的 Redis/Qdrant/PostgreSQL 状态，最后 `workload/.../runner.log` 再次确认三者 `false`；SSH/tunnel 和测试进程退出，既有本地 LM Studio 保留运行。

本机 observer 重放 helper 首次使用默认 Go cache 时遇到 `trim.txt` 权限错误；改为工作区 GOCACHE 后 vet/build 与复杂度检查通过，未因此重跑或替换实测。早期 SSH known_hosts 沙箱读取失败也保留；使用本会话已授权的隔离测试权限后通过原有 host-key 校验，没有关闭该校验。

## 新性能窗口：原 1.05 仍失败，候选仅点估计通过

使用原 FAQ、相同计划发送间隔 125ms、100 次/条件、inflight ceiling 4；实际两窗约 8RPS，峰值均为 **2**。这与上一轮实测峰值 4 不同，不可把两轮直接当成同一负载。仅一个时间块、off→on 固定顺序，未运行 memo 条件，未完成新的六块平衡验证或置信区间。

| 完整响应指标 | off | semantic on / memo off |
| --- | ---: | ---: |
| 请求 / HTTP 失败 / hits | 100 / 0 / 0 | 100 / 0 / 4 |
| 实际 RPS / 峰值并发 | 8.008 / 2 | 7.998 / 2 |
| 整体 P95 ms | 142.143 | 153.191 |
| Application Residual P95 / P99 ms | 4.393 / 5.188 | 4.641 / 5.292 |
| Provider complete P95 ms | 138.088 | 134.941 |
| 逐请求 health sync 合计 P95 ms | 1.551 | 1.207 |

整体 P95 比值 **1.077725**，增量 **11.048ms**：原 1.05 点估计失败；候选 `on≤min(1.25×off,off+40ms)`、residual P95≤10/P99≤15ms 点估计通过。on 的 96 个 miss P95=156.097ms，对应发送位置 off P95=142.287ms，miss 约 1.097×、+13.810ms，也仅是诊断点估计。on cache-read P95=22.707ms，embedding HTTP P95=20.591ms，vector-search P95=2.169ms；这些单独分位数不能相加作为整体 P95。

Residual 按每条请求先扣除 primary 与前台 cache-read 时间再求分位数，含网关/HTTP/健康同步等剩余成本，不是“全部 CPU 时间”。本轮约 5ms 与前轮 13–27ms 的差异要求继续查负载与环境；它既不支持“当前必然 PASS”，也不支持固定物理下限。禁止用一个成功窗口覆盖上一轮失败。

新 scope 最初两次向量查询返回 collection NotFound，记录为 `lookup/vector_error`；异步写入随后创建 collection，后续 4 次命中，主请求 fail-open 成功。不能把缺失 collection 广泛吞为 miss：需要单独验证仅正常首次创建的 NotFound，同时保留权限、传输、格式和存储错误。当前代码仍暴露原错误。

## Redis / VM：发现启动压力，未完成因果归因

`redis-workload-observation.json` 从 15:27:51Z 观察到 15:28:00Z，直到出现首个 GET；只覆盖依赖启动和早期请求，**不是完整两窗连续观测**。前两个诊断 helper 分别在 cleanup 后采到停止容器、以及使用远端缺少的 rg；错误保留在 `vm-diagnostics*.log`，不以它们证明应用负载。最终 observer 不依赖远端 rg。

启动期间 vmstat 显示部分秒内核 CPU 65–79%，swap-in 约 12–17MiB/s、swap-out 约 8–18MiB/s，块读约 200–340MiB/s，steal 为 0。SLOWLOG 中 INFO 约 10.99–18.45ms、SET 10.329ms、EVAL 14.651ms；首个 GET 批次 28 次均值 20.11µs。慢 INFO 包括观测命令自身，采样也会扰动结果；这些慢命令主要处于启动段，不能解释历史 warm 请求的 50ms 超时。SLOWLOG 仅保留时间、命令名和时长，不保存参数/凭据。

本轮 VM uptime 约 34–36 分钟，与前轮约 11小时40分不同，表明两轮间 VM 已重启；本任务没有执行该重启。环境因此不能假定完全连续。Qdrant 启动日志存在大量历史 `semantic_cache_<digest>` collection 恢复；代码 `vectorCollection(scope, model)` 按 scope/model 创建 collection，持久化和清理沿用该命名。**collection 数量、存活点数及持续增长尚未清点**，不能直接归因其内存/IO，也没有执行删除。

## 上游 400：未复现，暂不修改错误映射

本地 app 实测版本 `0.4.25+1`；loaded Qwen context=8192、parallel=4，embedding context=512。Windows 实际 HTTP port=1234，VM 的 11234 是到它的既有 reverse SSH forward；Windows 11234 拒绝不能据此认定配置错。

`upstream-replay.mjs` 使用原 FAQ 和历史涉及的 plan 70/64，在本地 loopback 分别重放 C1/C4、fresh/reused socket、主动取消后的 C4。五个正常窗口各 32 次共 160 次全成功，复用窗口实际复用 31/28/28 次；8 次 10ms 主动取消单列。C1 P95 132–138ms，C4 246–292ms，显示闭环饱和并发会扩大响应尾部，但不是网关相同 8RPS 的 SLO 比较。

上一轮两次 400 时间与 LM Studio `ChannelError` / `Engine protocol predict request failed: fetch failed` 对齐，仅是时间相关。缺少 engine fetch 底层 cause/errno、原始上游 400 body 和贯穿两层的请求关联，无法断言客户端请求无效或连接复用故障。本轮脚本会保留失败 body 的有限脱敏内容与客户端错误 cause/code；没有修改已安装 LM Studio 或泛化重映射所有 HTTP 400。

## 下一步可执行排查

| 优先级 / 依据 | 操作步骤 | 预期结果及判定 |
| --- | --- | --- |
| P1：自然 Redis 50ms 超时仍未归因 | 在同一批次先等依赖恢复完成，再运行至少六个 off/on/memo 平衡块；每块沿用 100 请求/125ms 和相同上限，保留实际 RPS/并发/失败。全过程同步采 vmstat、容器 CPU/RSS、Redis commandstats/脱敏 SLOWLOG 与应用 GET/同步时长，并记录 Windows host CPU/内存/模型日志。启动段与 warm 段分开统计；每个后端 test 保持 60s 硬上限 | 将超时与 CPU/换页/IO/客户端排队按请求时间对齐；若 warm 无超时，仅证明本次窗口，不调整 50ms。若同期出现，得到可重复的压力触发条件，再决定部署隔离或代码优化 |
| P1：collection 初始化错误和可能的增长成本 | 先只读清点总 collection、各 points_count/状态及恢复时间；在新测试 scope 运行 seed→异步落库→重复请求，验证初始缺失与已创建状态。另用同一 scope/version 连续写入验证是否增长；不清旧 volume。必要时在独立新 volume 对照 restart IO，需先具备该隔离资源 | 区分预期首次不存在、异常丢失和恢复放大；获取增长上界。只有确认受控初始化错误后才设计显式初始化，不能无条件把所有 NotFound 隐藏 |
| P2：历史 LM Studio 400 缺少底层证据 | 保留 bounded 直连复用/fresh/C4/取消矩阵；若再现，立即收集脱敏 HTTP status/body、请求内容 hash/ID、engine channel 错误以及 fetch cause/errno，比较 gateway/direct 两条链。每批固定请求数、55s 外层上限；不调用付费远端或卸载共享模型 | 原始原因可区分真实请求错误、engine transport 错误和取消关联；有特定可复现签名后再做窄范围映射/修复。0/160 不是根因关闭 |
| P2：性能标准需要可信样本 | 在上述稳定环境六块完成后，分别计算 whole/miss/residual、失败率及时间块区间；保留原 1.05 与候选 AND 门槛。增加明确预算的独立空闲/饱和场景，不混入 cold 或故障请求 | 可以评估本地快模型适用预算；若结果不足，继续记录未决。证明特定架构/负载不可达需要串行新增成本的逐请求分布及稳定 baseline，不能用均值除以 P95 代替 |

还需补充：历史故障同期 Windows/VM 指标、LM engine 内层 fetch 原因；业务认可的同答案/危险负例语料及 recall 目标；可独占卸载/重载的模型环境及生产 FAQ 发布/版本切换/停止期限 owner。现有少量退款例子和本轮 FAQ 不能替代业务语义批准。

复现命令、目录含义、源码及 binary 指纹、脱敏审计见 [本轮 evidence README](measurements/availability-investigation-20261004/README.md)。本轮先关闭已证明的错误分类缺陷，其余项目保持开放，Phase 29 仍为 `partial`。
