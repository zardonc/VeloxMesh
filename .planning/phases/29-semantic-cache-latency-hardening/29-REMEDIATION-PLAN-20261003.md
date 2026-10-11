# Phase 27–29：现状与后续解决方案

日期：2026-10-03。核对时 HEAD：`b2eb95ff`。状态：仅分析与规划；本次不修改产品实现、不运行新压力测试、不合并、不部署。

依据：已读取聊天 `Test and ship Phases 27–29` 的最新完成轮次，并核对 `29-SCENARIO-VALIDATION-PLAN-20261003.md`、`29-SCENARIO-VALIDATION-RESULTS-20261003.md`、本轮 summary/manifest、相关源码及既有 Phase 27/28 验证记录。此前分析中的缺口以新证据更新，不把旧问题重复当成尚未验证项。

## 1. 已确认事实与证据边界

| 项目 | 当前证据 | 判断 |
| --- | --- | --- |
| Phase 27/28 | 既有 verification 为 passed；最新场景轮未重跑完整协议矩阵 | 不是当前主要修复对象；共享调用链变更后仍需最终回归 |
| embedding 复用、Qdrant 创建竞态 | 正常 miss 单次 embedding；两个独立进程各 16 创建者、错误维度/distance/鉴权/取消已验证 | 已补齐此前对应缺口 |
| 两款 embedding 生命周期 | 同账户、同 SQLite 库完成 collection→nomic→collection，独立 scope，各自写入/hit | 两个真实模型已验证；两者均 768 维，真实跨维度切换尚无证据 |
| 应用处理性能 | off P95 4.640ms、miss 5.077ms、实际 C8 9.299ms；warm hit 完整响应 37.983ms | 通过本轮诊断预算；不等于正式生产 SLO 通过 |
| 低命中完整响应 | off P95 131.741ms、on 160.744ms，4% hit，六组 3,000 请求成功 | 比值 1.22015，原 1.05 门槛失败；按本轮基线目标 138.328ms，仍差约 22.416ms |
| 实际容量 | 12 RPS 能维持到达负载；约 16 RPS 已有发送积压；C8 主请求全部成功 | 不能宣称网关最大吞吐或批准扩大缓存并发 |
| embedding 稳定路径 | 同 Go 客户端、并发预热后，本机/VM C4 P95 35.696/36.716ms | 当前不支持网络是主要稳定瓶颈；首并发窗口瞬态仍开放 |
| 语义诊断 | 阈值 0.92 下两模型正例 2/50、9/50，负例均 0/100；直接余弦与网关命中一致 | 召回诊断失败，未发现网关漏掉满足阈值的候选；组合样本不是独立生产意图 |
| 部分写入 | SQLite 已提交、Qdrant 断连后留下未索引行；新请求恢复不自动修复旧行 | 存在生命周期缺口；当前未观察到未索引行误命中 |
| 网关尖峰 | 18–20ms 尖峰定位到 provider 返回→response_rules；Redis 延迟可复现约 203ms 空档 | Redis 是已证实可产生此形态的路径，尚不是自然尖峰已确认根因 |
| 上游质量/稳定性 | 本地 0.5B 直连也答错取消政策；SANS 直连 12 秒无首字节 | 不应由网关猜测修正答案；上游内部排队、推理与网络仍需后台证据 |

当前生产缓存保持关闭、生产白名单保持为空。原合并条件继续有效；诊断预算不自动取代发布标准。

## 2. 推荐决策与性能合同

先解决语义适配和部分写入生命周期，再针对已定位的 Redis 等待做最小修改。保留现有有界 lookup/写队列；不要以纯本地耗时通过代替端到端成本，也不要仅缩短读取预算来制造“更快但没有命中”的结果。

| 场景 | 建议正式验收形式 | 当前可使用的定位标准 |
| --- | --- | --- |
| cache-off / bypass | 同模型与负载的完整响应 SLO + 网关处理预算；bypass 无缓存 I/O | 网关 P95≤10ms、P99≤15ms，目前仅为诊断预算 |
| eligible miss / 低命中 | 完整响应 SLO + 额外查找成本预算 + 成功率；必须包含 embedding/Qdrant 等待 | 原 on/off≤1.05 仍显示失败；新绝对 SLO 值待业务确认 |
| warm hit | 完整响应 SLO + 同问题/同并发 off 对照收益 + 正确答案/零新增计费 | 本轮 ≤min(110ms, matched-off P95×0.5)，不是生产承诺 |
| 冷启动 / 故障 | 单独的完成/降级截止、许可与队列边界、恢复期限 | 冷启动和故障不混入正常 P95；失败比例、取消和恢复必须保留 |
| 饱和 / 突发 | 实际到达率、实际并发、成功率、背压、恢复；按依赖与 Provider 分层 | 不能用发送积压后的 P95 证明维持了目标负载 |
| SSE / 工具 / Fusion | 首个有效事件、完整终止、每轮耗时、一次结算；并行分支分别计时 | 不将 SSE TTFT 或工具客户端执行时间替代非流式完整响应 |

业务需要指定：目标 RPS/突发长度、允许完整响应时间、miss 查找成本、最低缓存业务收益、事实质量和正例召回目标。缺少这些值时只做诊断，不签生产 SLO。

若保留 1.05 倍完整响应门槛，当前约 132ms 主模型基线只允许约 6.6ms 总体 P95 增量；这是预算数量级，不是分段 P95 可以相减/相加的证明。现有串行 embedding 尚无能满足该预算的证据。可行路线是验证更低等待的 embedding 部署/模型或收窄缓存适用场景；低收益场景保持 bypass。正式改用分场景绝对 SLO 必须明确批准，原结果仍保留，不改判历史测试。

## 3. 工作包与验收

### W0：冻结可复核基线及业务验收集

操作：保留最终二进制、runner、源码/脚本 SHA 和全部失败；整理待测 Provider/模式矩阵。建立带 answer_id、等价问法、不同答案近邻、歧义标签的业务集。按基础意图分组拆分校准集/独立验收集，禁止把同一问法的不同后缀随机拆到两组。

产物：场景 SLO 决策表、模型版本与有效输入策略清单、金标准及留出集、基线清单。

验收：每个样本有明确预期答案或应 miss/bypass 的理由；业务负责人确认判据。原计划要求正例命中率报告，没有未经确认的生产百分比；新诊断 50/50 断言不能自动成为全业务标准。

### W1：模型/输入/阈值适配，优先级 P0

依据：两模型的正负余弦重叠；0.92 下命中数与直接分数一致。Nomic 官方模型卡要求任务指令前缀，当前应用 embed 传入问题原文；模型服务是否自动补前缀尚未知，不能据此直接判定根因。

操作：

1. 记录真实模型文件/版本、量化、pooling、归一化、截断及服务端有效输入，确认前缀由哪一层处理。
2. 在真实模型上比较当前输入与经模型卡确认的问句相似性输入策略。先用相同策略编码查询/已存问题，以保留 lookup 向量复用；改用不同 query/document 编码时，另计后台调用与成本。
3. 在校准集逐模型扫描阈值，优先保证不同答案零误命中，再评估收益。保持不包含共享 system prompt 的问题级语义输入。
4. 若输入表示、底层模型版本或 pooling 改变，生成新 representation/policy 版本并纳入 scope；旧向量不能混用。
5. 冻结策略后用留出集执行多 FAQ 种子、近邻混淆、同义 hit、歧义 miss/bypass、版本及账户隔离。

本次只读重算的候选筛选结果：

| 模型/阈值 | 同义正例满足阈值 | 不同答案满足阈值 |
| --- | --- | --- |
| collection / 0.92 | 2/50 | 0/100 |
| collection / 0.82 | 26/50 | 0/100 |
| collection / 0.80 | 38/50 | 1/100 |
| nomic / 0.92 | 9/50 | 0/100 |
| nomic / 0.86 | 38/50 | 0/100 |
| nomic / 0.84 | 39/50 | 0/100 |
| nomic / 0.83 | 40/50 | 1/100 |

这些是在已见诊断分数上计算的候选，不是实际改阈值后的网关复测、留出集结论或生产推荐。Nomic 是值得先验证的候选，而不是已确定优胜模型。

产物：逐模型分数/误命中分析、有效输入版本、候选策略比较、留出集真实网关报告。

验收：留出集不同答案零误命中；正例召回达到业务确认目标；各模型隔离与单次向量复用保持正确。若无法兼顾，收窄可缓存问答范围，或继续关闭该 profile；不靠全局降阈值掩盖交叠。

### W2：最小部分写入修复，优先级 P0

推荐先采用现有 enabled 字段的提交边界，不立即增加通用持久任务/outbox。当前 SQLite Store 支持 enabled=false，GetCandidate/ListCandidates 只读取 enabled=true；无需为这一边界新增 schema。

操作：

1. 有向量存储时，仓库先保存不可读的不可变 entry 快照；向量写入确认成功后，再用新快照启用 entry。无向量存储的既有路径单独保持其语义。
2. 向量写失败或启用失败均保留原始错误和原因计数；任何补偿失败同时暴露。未确认可用的 entry 不得参与命中。
3. 为不可读/过期 entry 设计有界清理：保留 scope、entry ID 和到期时间；向量删除确认后再移除仓库记录，删除失败保留可重试依据。每批数量、截止及 shutdown 纳入有界配置。
4. 若需要同一 entry 自动回补/重试，先实现语义缓存专用稳定 point ID。当前 qdrantPoints 每次创建随机 UUID，直接重放 Upsert 会新增点；不要全局修改其他向量消费者的 ID 语义。
5. 用真实透明断连测试仓库提交后、向量提交后、启用前、清理中断及进程重启；验证失败条目最终达到明确终态。

产物：提交边界设计、失败/过期生命周期、清理指标与故障恢复报告。清理只针对可重建缓存，不涉及 usage、账户或业务知识内容；实际删除/迁移遵守仓库授权边界。

验收：pending/失败/过期记录无命中；主响应不等待后台工作；不新增主模型调用或计费；过期/失败残留在选定清理期限内消失；恢复不能仅证明新请求成功。重试方案还须证明同一 entry 重放无重复点。

### W3：post-provider 等待的定位和最小修复，优先级 P1

依据：service.go 在 provider 返回后执行 EndRequest、RecordModelOutcome、指标、路由记录和 admission release。RedisStore 的相关写入在全局 mutex 内同步执行，使用 context.Background，部分错误未被调用方处理。这是源码确认的放大路径；自然尖峰具体来源仍需请求级证据。

操作：

1. 用可注入观测器单列 health provider 写入、model outcome 写入、锁等待、metrics、routing record、admission release；关联 Redis 命令时间与同请求 Go trace。
2. 对真实 Redis 单次延迟/断连、同 Provider 和不同 Provider 并发进行复现，量化持锁等待的跨请求影响。
3. 首选保留本地健康更新语义，给远程同步明确调用级截止并暴露错误；缩短全局锁范围，锁内生成不可变快照，网络操作按经验证的顺序/版本规则执行。
4. 验证并发旧快照不会覆盖新快照。若有界同步仍不足，再单列后台同步设计，先确认跨节点健康新鲜度、顺序、丢弃与关闭规则。
5. 单独验证鉴权、限流、余额、Provider health 和结算语义；仅改变健康复制/观测路径，不将其错误扩散成错误扣款或错误响应。

产物：尖峰分段证据、最小变更方案、并发乱序/错误路径及跨节点契约。

验收：真实 Redis 延迟不引起无界锁等待；错误可见；在批准的同步期限内返回；Provider 连续失败与恢复计数正确，快照顺序符合契约。正常诊断处理预算仍通过；未复现自然尖峰时不写成已确定根因。

### W4：embedding 冷启动与主模型质量，优先级 P1

操作：同 Go 客户端分别运行真实冷启动、首次并发、预热 C4、C8；单列并发预热与正式窗口。获取模型加载/排队/推理/CPU/GPU 日志。聊天并行实验采用独立到达计划，不能让等待聊天完成改变 embedding 发送率。

在测试环境评估 readiness 探测、有限并发预热及服务不适配时的明确 bypass。Readiness 不能只看 HTTP 200，还需维度与实际调用；不得在正常入口无限等待模型加载。10ms 全超时/零命中的方案淘汰，50ms 仅作为需覆盖冷启动和语义命中的候选。

对实际可缓存主模型执行业务事实验收；0.5B 作为链路/性能工具仍有价值，但其取消政策答错使它不具备本轮业务质量证据。SANS 保留无首字节失败，补 Provider request ID/后台日志，不以少数成功请求计算 P95 或扩大通用重试。

产物：冷/热能力边界、真实并行资源报告、主模型事实质量和允许场景。

验收：冷态被单独处理且请求/后台工作有界；模型切换隔离；预热收益可重复；选定可缓存主模型满足业务事实和稳定性要求。缺少目标负载/服务端日志时不批准扩并发。

### W5：最后一轮真实 E2E 与发布判断

依赖：W0 的正式合同，以及 W1/W2 完成；W3/W4 的适用修复或经明确批准的范围决策已记录。完整 E2E 只在开发完成后运行；需要隔离验证时先列失败方式，再编写实现。

矩阵：SSE 完成/取消/EOF/一次终止/一次结算，buffered/Fusion；工具 required/named/auto/omitted/none、SSE及 tool_call_id 续接；Gemini signature 按 README 契约；缓存双模型/账户/版本/输入策略隔离；错误向量/缓存答案；冷集合、多进程、部分写入、清理、关闭；同模型 direct/off/miss/hit/bypass、低命中与目标负载。

每项后端运行 Go timeout 加外层 60 秒硬截止，拆短窗口累积。保存 warmup、失败、超时、实际并发、实际到达率、调度滞后、P50/P95/P99、计费断言、SHA及清理确认。采用至少六个平衡区组并增加跨时段样本；所有条件使用相同输入/传输/采集策略。

产物：每场景/每 Provider 判定表、全部失败处理说明、可复算证据、回滚演练。PostgreSQL/pgvector 若在发布支持范围内必须验证真实业务链，readiness 不能替代。

验收：所有适用功能和正式性能条件通过；不可用/未测的 Provider 不记 PASS；任何缩小原发布范围的决定必须明确记录。原全通过合并条件未变，仍有开放问题时不合并 main。代码合并与生产缓存启用分别验收。

## 4. 可直接复跑的入口与需扩展项

| 目标 | 已有入口 | 本方案新增要求 |
| --- | --- | --- |
| 阈值/模型 | TestLiveEmbeddingSemanticScores、TestLiveSemanticDiagnosticPositive/Negative、TestLiveEmbeddingModelSwitch | 有效输入策略开关、按意图拆分数据、多 FAQ 种子、冻结阈值的独立验收；当前诊断测试内部固定 0.92 |
| 部分写入 | TestLiveVectorPartialWriteRecovery | pending/启用失败、同 entry 重放、TTL清理、清理失败及重启；现用例只证明新请求恢复 |
| Redis等待 | TestLivePostProviderRedisDelay | 精细 health/锁/metrics/release 分段、跨 Provider 并发、错误和乱序 |
| embedding | TestLiveEmbeddingStress、已有 host/VM Go 控制脚本 | 独立发送的混合负载、真实冷启动和服务端时间 |
| 性能 | TestLiveDirectLoad、TestLiveCacheLoadOff/On、Bare、TestLiveCacheHitSettlementLoad、TestLiveMissBurst | 正式场景 SLO、跨时段目标负载；统计全部失败 |
| Phase 27/28 | TestLiveStreamSettlement/Cancellation、TestLiveTool* 及现有功能测试 | 最终候选版本完整支持矩阵；外部失败保留单独归因 |

从仓库根目录在 Git Bash 操作。先按 scenario-validation-20261003/README.md 构建带 phase29preflight 的候选测试二进制和 runner、配置既有授权的隔离环境。每次设置新的 SHIP_RESULTS_ROOT/SHIP_ARTIFACTS，保留原证据；SHIP_BINARY/SHIP_RUNNER 指向本次候选版本，不复用旧 SHA 冒充新实现。

以下为后续真实验证命令，不在本次规划中执行。模型/接口/凭据通过现有未入库配置提供。

```bash
export PHASE29_COUNT=100 PHASE29_INTERVAL_MS=125
export PHASE29_CLIENT_CONCURRENCY=4
export SHIP_TESTS=TestLiveCacheMissEmbeddingReuse,TestLiveEmbeddingModelSwitch,TestLiveVectorPartialWriteRecovery,TestLivePostProviderRedisDelay
"$SHIP_RUNNER"
```

末轮另运行完整后端编译/vet与实际 E2E；若改控制台，再运行 pnpm 的 build/typecheck。已有 scenario suite 会保留单格失败，但不能仅按 suite 退出码判全通过。其统计脚本读取特定目录/标签；新根目录需连同可复算脚本保存并检查采样分母，不能把缺失/跳过窗口计入通过率。

## 5. 推进顺序与缺失资料

顺序：W0 → W1/W2 → W3/W4 的针对性修复 → W5。W0 的分场景 SLO 决策不阻止准备失败用例、校准实验与最小一致性设计，但发布判定依赖该决策。优先完成局部可验证修复，避免在每次迭代重复整套 live suite。

需补充的资料：业务场景负载/SLO与批准人；真实独立问答集和召回目标；模型文件/版本/有效前缀/服务端排队及推理日志；失败 Provider request ID/后台日志；健康同步跨节点新鲜度要求；缓存清理期限与目标支持的存储/Provider矩阵。

上述 owner 未指定，不能替用户批准正式 SLO、阈值、生产白名单或范围变更。实际缓存删除/生产迁移及部署须按仓库边界取得明确授权；本计划不会执行这些动作。

## 6. 证据链接

- [最新真实场景报告](29-SCENARIO-VALIDATION-RESULTS-20261003.md)及[预注册诊断计划](29-SCENARIO-VALIDATION-PLAN-20261003.md)。
- [原始样本/summary](measurements/scenario-validation-20261003/summary.json)、[manifest](measurements/scenario-validation-20261003/manifest.json)及[复现步骤](measurements/scenario-validation-20261003/README.md)。
- [原修复复测](29-CACHE-FIX-RETEST-20261003.md)：正常向量复用、Qdrant竞态修复及历史1.05失败。
- 当前源码：internal/cache/semantic.go 的 persist；internal/controlstate/sqlite/repository.go 的 semanticCacheRepo；internal/storage/qdrant.go 的 qdrantPoints；internal/gateway/service.go 的 post-provider 顺序；internal/health/redis_store.go 的 EndRequest/RecordModelOutcome。
- [Nomic 官方模型卡：任务前缀及使用方式](https://huggingface.co/nomic-ai/nomic-embed-text-v1.5#task-instruction-prefixes)。模型卡描述原模型；本地服务是否已自动处理前缀须查有效配置，不假定同名接口等同于某种输入实现。
