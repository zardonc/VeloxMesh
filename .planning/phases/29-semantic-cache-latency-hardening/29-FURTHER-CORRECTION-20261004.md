# Phase 27–29 进一步排查与局部修正 — 2026-10-04

本轮修复两个已复现的产品问题：**Gemini 终态后的异常流被接受为成功**，以及**没有已激活缓存条目的 scope 仍执行 embedding/不存在集合查询**。最终后端 599 个顶层 PASS、0 FAIL、2 个显式 opt-in SKIP。真实 Gemini、SQLite/Qdrant 网关及 PostgreSQL 生命周期验证通过。原性能、业务语义和集合回收发布合同保持开放。

依据引用聊天的最新分析继续调查；源码基于 `9606669` 加本轮未提交修改。完整脚本、失败、日志、源码/二进制版本和凭据审计位于 [证据目录](measurements/further-correction-20261004/)，最终索引为 [manifest](measurements/further-correction-20261004/manifest.json)。

## Phase 27：终态后的错误必须阻止成功结算

先写下失败条件并运行真实 HTTP peer/SDK 回归。修复前，文本流接受终态后的内容、重复终态以及终态后被截断的 HTTP body，均发出成功 Done。SDK 的 scanner 错误仍只记录日志，原生 finish 状态不能独立证明 clean EOF。

修正为请求级 body 审计：透传原 HTTP transport，仅为流请求观察 EOF/读错误；迭代结束后同时核对 context、原生 terminal、clean EOF。上下文取消仍分类为 `context.Canceled`；不完整 body 返回 `provider_bad_response` 并保留读错误 cause。原生终态后再次出现 candidate 被拒绝，合法 usage-only 尾帧允许通过。没有修改全局结算合同、重试或期限。

| 验证 | 本轮结果 |
| --- | --- |
| HTTP peer + 当前 genai SDK | clean、usage trailer、重复终态、终态后内容、截断、finish 后且 EOF 前取消六个边界通过 |
| 真实 Gemini 非终态取消 | 约 0.319ms 传播，零 Usage/扣费，无 health/pending 异常 |
| 真实 Gemini 终态前中断 | 协议错误，零结算 |
| 真实 Gemini 终态后截断/重复 | 两个受控交付故障均被拒绝，零 Usage/扣费 |
| 真实 Gemini 完成后关闭 | 结算一次，随后关闭无重复扣费 |

真实故障注入保留提供商实际 frame，只改变网关的交付尾部；不推断提供商物理完成时刻。边界合同仍为：收到 native finish 尚不足以结算，网关 clean completion 前的取消归 client-cancelled。Close 自身出错、超大帧、终态之后恶意帧的全部形状、真实缓冲/Fusion 的同时取消仍未穷尽覆盖。

## Phase 28：请求参数对照已完成，间歇超时仍未关闭

新真实对照捕获原生和网关实际发送的流请求，在内存中对比参数与认证，日志只保留 JSON canonical SHA256、等同性与耗时。两个请求的 canonical body、endpoint/method、认证/Content-Type/User-Agent 相同，body SHA256 均为 `214ebcf0002e9673066d88ce3289e81b31090c5eff2f2a035620a833e7563d15`。

本次原生新连接 HTTP 200 响应头约 725.775ms、完整流约 747.182ms；网关复用连接响应头约 439.606ms，真实签名与正常工具结果续接通过。网关用例还包含一条正常非流式续接请求；“两次”仅指被比较的首轮流请求。上游没有返回 X-Request-ID，空字符串哈希不能用于节点归因。

这组结果排除了本次参数/认证差异，并表明网关路径可以正常完成；没有复现历史 12 秒无首字节超时。仅一组固定顺序对照，且新连接/复用不同，不能认定历史根因就是连接或上游节点，也不能认定间歇故障已修复。JSON canonical 等同不声称已经冻结逐字节相同 body。没有增加重试或延长期限。

## Phase 29：空/待激活 scope 的读取语义修复

新增 `SELECT EXISTS` readiness 查询，仅检查同 scope/model 的已启用、未过期关系行。SQLite、PostgreSQL 和复制 wrapper 均支持；没有扩展已有必需仓储接口，其他实现保留原查询路径。原表缺少匹配索引，已追加 SQLite 0011、PostgreSQL 0010 迁移，为 active 行建立 `(scope, model, expires_at)` 部分索引；在本轮测试库验证迁移与功能，未应用到生产。生产大表建索引的锁与空间成本仍需在实际升级流程中评估。

无已激活条目时记录 `lookup/scope_not_ready` 并直接 miss，前台不调用 embedding/Qdrant。后台首次存储仍按既有 pending→向量写入→active 合同完成。每次从仓储读取 readiness，没有永久负缓存：激活后、进程重启后的已有条目按正常路径查询。readiness 仓储失败仍记录 `repository_error`；存在 active 条目而集合丢失时，实际向量错误仍暴露。

实际 SQLite 验证缺失、pending、过期、其他 scope/model、随后激活和关闭仓储错误。真实 PostgreSQL 验证缺失→pending→active→expired、其他 model 和错误。新独立 scope 的真实网关首次 miss、异步落库、随后 hit 成功，`vector_error=0`，命中答案哈希一致、只有首次主模型请求结算。原 exact 安全、版本隔离及网关语义功能用例通过。既有 embedding 503 和队列 burst/关闭回归通过；新 scope 的故障发生在后台 `store/embedding_error`，前台直接 miss，主响应保持 HTTP 200。已激活 scope 的 read 故障仍由原隔离回归覆盖。

readiness 不能证明每个 active scope 的集合始终存在，也没有实现完整持久所有权或自动回收。补充回归后，最终本轮新卷仍有十个集合，其中九个 semantic，保留为诊断证据（见 `cleanup-final.log`）。需要定义跨进程 owner、停用/回滚、在途写入保护与重启恢复，再选择专属集合回收或强制 scope 过滤的共享集合方案；不能仅凭本地测试库关闭就删除持久集合。

## 缓存路径成本：区分模式，保留漂移

有效 miss 窗口均为 100 请求、125ms 间隔，实际约 8 RPS、最大在途 2；全部早期正式请求保留。各 cache 窗口的 seed 先持久化，再静置两秒。首次矩阵中的 exact 标签被 helper 环境重新覆盖为 semantic，100 个样本保留但排除；只另目录补跑 off/exact/off，没有覆盖原记录。

| 对照 | P95 与阶段 | 结论边界 |
| --- | --- | --- |
| 修正 exact miss | off 119.109/123.660ms；exact 128.083ms；cache-read 0.325ms，100 次正式请求零 embedding/向量查询 | 保守较慢 off 锚点比值 1.0358，漂移 +3.82%；只是单块点估计，未批准正式性能门槛 |
| semantic miss | off 145.517/124.608ms；semantic 150.791ms；cache-read 20.943ms、embedding HTTP 18.814ms、向量查询 1.982ms、readiness 0.360ms | 基线漂移 -14.37%；较慢锚点比值 1.0362 不能算通过。readiness 未删除已激活 scope 的串行 embedding 成本 |
| semantic hit | 100/100 hit，结算只发生于 seed；P95 27.947ms | 既有 helper 实际为 C4 burst，约 162 RPS；不同问题/吞吐，仅作功能及并发成本证据，不与 8 RPS miss 作门槛比较 |

不能相加/相减独立阶段 P95 得出单请求成本。逐请求关联与零阶段调用支持：exact 避免 embedding/向量往返；semantic 的新增成本主要仍在 embedding。此前[硬件优先排查](29-CONTINUED-INVESTIGATION-20261004.md)在当前已加载模型、8 RPS 稳态下没有资源容量不足证据，暂不支持以扩容解决差距；该结论不保证其他负载或短时竞争均无硬件因素。本轮没有新同步 host/GPU 采样、扩容、冷加载或生产规模持续写入实验。没有为了取得“通过”而继续重复扫参数。

## 验证、版本与保留的失败

最终全量 `go test -count=1 -timeout 60s -json ./...` 在本轮独立真实依赖下通过，外层同样 60 秒；599 顶层 PASS、0 FAIL。两个 opt-in SKIP 为 `TestPlan4PostgresSmoke`、`TestPlan4PostgresSansPrimaryRealProviderSmoke`，不计通过。`go vet -tags phase29preflight ./...`、Linux tagged 编译、Go 格式/函数边界及 diff 检查通过。

先行失败保留：初版回归夹具缺少 adapter 类型转换、NOT NULL vector 及 UTC 时间规范；修正夹具后的红色回归才作为产品失败证据。随后一次 active-scope 测试的本地时间比较失败属于夹具，修正 UTC 后通过。首个 exact 标签错误和初次评估输出字段错误也均记录或更正。首次 indexed-readiness 启动时编译尚未结束，时间戳证明用了旧 v3 二进制；该日志排除，新 v4 在 `indexed-readiness-confirmed` 重验成功，没有使用旧结果宣布索引验证通过。

`live-v1-binary.sha256` 对应 Gemini 与本地网关功能检查；v2 对应 PostgreSQL 与首个路径矩阵；v3 对应修正 exact 和索引追加前的回归。v1–v3 的产品 Go 实现一致，差别为新增/修正测试夹具及记录字段。最终 v4 追加上述 SQL 索引迁移，在 `indexed-readiness-confirmed` 和 `backend-indexed-final` 验证；之前路径测量仍属于索引追加前的小测试库，不被改写成最终容量结果。源码与最终 binary 哈希见 manifest。基于本轮修改没有创建提交、推送或部署。

本轮测试容器、SSH 转发和进程全部退出，远端专用测试 binary 记录哈希后移除，端口只剩 SSH/DNS。历史资源和停止的诊断卷保留，用户模型服务保留。凭据未写入源码或 retained logs。

## 仍需关闭的合同

Gemini 间歇响应头超时、稳定多区组 semantic miss 性能、自然 Redis 停顿/历史 LM Studio 400、持久集合回收和真正冷加载/不同维度生产容量仍开放。FAQ 种子事实错误和危险语义负例没有通过本轮性能结果被消除；业务来源、审核、版本及安全留出集仍是发布前置条件。生产缓存关闭，Phase 29 保持 partial。
