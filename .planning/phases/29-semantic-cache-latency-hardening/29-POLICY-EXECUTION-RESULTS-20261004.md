# Phase 29 修正执行结果

执行：2026-10-03 至 2026-10-04（America/Vancouver）。基线 `780e12e`，实现保留在当前工作区；最终文件和测试二进制哈希见 [manifest](measurements/policy-execution-20261003/manifest.json)。

**策略、响应截止和容量隔离已实现，最终全量回归通过。性能与可用性仍未满足放行条件，Phase 29 保持 partial。没有合并、部署或启用生产缓存。** 本轮结果不支持“网关已到物理下限”或“修改 P95 门槛即可通过”。

## 已执行的修正

| 项目 | 实现与依据 | 验证结果 |
| --- | --- | --- |
| 测量口径 | 六窗口汇总时累加窗口时长；完整/命中/miss 分开统计；候选使用 `min(1.25×off, off+40ms)`，原 1.05 列保留 | 4 个计算回归通过。旧结果重算 actual RPS 约 8，而非 48；旧 P95 不变，[evaluation-v2](measurements/followup-20261003/evaluation-v2.json) 独立保存 |
| 可信复用策略 | 现有 use case 新增 `disabled / exact / semantic`；省略模式默认旁路；未知值在启用配置中报错；拒绝重叠可信 profile | 三种策略真实 HTTP 测试通过。没有客户端可自行授权的白名单 |
| 精确答案缓存 | SHA-256 完整问题字节与 scope；不折叠大小写/空白；直接访问关系库，不调用 embedding/vector；TTL、限期写入、零 usage 和既有旁路保留 | 3 个相同种子命中；41 个改变的问题全部 miss，包括旧 32 问、否定/单位/产品/条件/中文期限变化；44 次主模型结算恰好一次。运行路径 embedding 调用为零 |
| scope 隔离 | reuse policy 版本进入 opaque scope；保留身份/系统/模型/设置/知识版本隔离 | exact 知识版本 v1→v2→v1，分别隔离并恢复旧命中，仅 2 次种子结算；两个真实 embedding 模型切换生命周期通过 |
| Provider 保护 | 各实际 attempt 前获取共享资源池令牌；非流式/embedding/stream 使用首响应字节、首内容/完整响应体、流式空闲和总体截止；并发满立即 429；不新增自动重试 | 六个真实用例通过：五种阶段故障、并发拒绝/恢复、取消/恢复、chat 与 embedding 共享容量、总体截止、注册表切换保留令牌 |
| 流式 usage | OpenAI-compatible 请求补发 `stream_options.include_usage=true`；非流式请求不增加该字段 | 直连 LM Studio 确认最终 usage；最终 SSE、取消、缓冲流、Fusion 全通过。普通/缓冲各结算 45 tokens；Fusion 汇总 128 tokens、`missing_rate`、不扣费，保留既有会计策略 |

精确策略验证的是“是否错误复用种子答案”，不保证主模型给出的种子事实正确，也不证明语义泛化召回。Embedding memo 仍只缓存向量。`semantic` 是明确选择的实验模式；本轮没有引入 reranker 或声称数值 token 相同即可保证逻辑等价。

流式 usage 修复前的三个失败保留在 `warm-batch/protected-local-stream/`。适配器此前未请求 usage，最终行出现 `missing_usage`；加入标准请求字段后在 `protected-stream-final/` 通过。[LM Studio 官方 API 更新说明](https://lmstudio.ai/docs/developer/api-changelog)说明该字段会返回流式 token 用量。

## 最终检查与故障边界

- 全量后端：**38 个有测试包通过，595 个顶层测试、954 条含子测试 PASS 记录，0 失败**。2 项无关 Plan 4 opt-in 按原约定 SKIP，未计为 Phase 29 通过。完整日志：[full-backend.jsonl](measurements/policy-execution-20261003/final-backend/backend-regression/LOCAL-qwen2.5-0.5b-instruct/full-backend.jsonl)。
- `go vet -tags phase29preflight ./...`、`go build ./...`、tagged Linux 编译通过；改动 Go 文件格式化、新增函数长度检查及 `git diff --check` 通过。该改动不涉及前端。
- 已用未跟踪环境文件中的实际凭据值扫描结果目录、55 个改动源码/文档及历史重算文件，均未发现匹配；方法和结果见[审计记录](measurements/policy-execution-20261003/credential-audit.log)。该检查仅覆盖已加载的凭据值。
- 测试先定义故障条件再实施。首次配置/测量回归暴露缺失实现；真实 Deadline 测试最初因旧结算断言不接受零 token 而失败，修正后通过。全量回归暴露旧 header 测试既未显式选择 semantic，又依赖已被删除的重复 embedding；现改为阻塞实际 repository write，继续验证前台不等待异步落库。
- 阶段故障注入转发真实模型字节，不伪造业务答案。约 1s 测试截止产生 1.03s 左右终止；无首响应、响应体停滞/部分 JSON、只有 SSE 心跳、流中停滞及总体截止均分类正确，零结算。取消、共享资源和注册表替换后恢复，无重复扣费。
- 每个后端测试设置 Go 与外部 **60 秒**硬截止。Qdrant 启动就绪另设 120 秒；多区组期间依赖保持运行，最终恢复三个原先停止的测试容器。见 [最终回收日志](measurements/policy-execution-20261003/final-backend/lifecycle/runner.log)。未删除历史集合或 volumes。

首次沙盒运行无法访问 SSH 主机密钥/VM 网络；在已授权隔离范围执行后测试跑通。Go cache 改用工作区临时目录。环境失败日志保留，没有替换为 PASS。

## 新性能结果：候选合同未通过

真实主模型 `qwen2.5-0.5b-instruct`，embedding `text-embedding-embedder_collection`。每场景六个顺序平衡区组，direct/off/on/memo 各 600 次；计划间隔 125ms、客户端上限 C4。两个条件分别固定 4% 命中和 100% miss。计算与逐阶段数据见 [evaluation.json](measurements/policy-execution-20261003/warm-batch/evaluation.json)。

| 场景 | direct P95 | off P95 | on P95 | memo P95 | on/memo residual P95 | 结论 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 4% hit | 140.882 | 249.497 | 297.043 | 379.297 | 15.535 / 26.980 | 含 2 个 off 400，诊断数据；原门槛及候选完整响应/本体预算均不通过 |
| 纯 miss | 235.256 | 305.689 | 199.349 | 207.638 | 13.320 / 13.202 | 选定窗口零 HTTP 失败，但本体 P95≤10/P99≤15 不通过；端到端点估计不足以放行 |

单位均为 ms。实际吞吐约 7.85–8.01RPS，峰值在途 4。low-hit 的 on/memo 各 24 次命中；pure-miss 命中及 memo-hit 均为零，避免用旁路或未启用 memo 冒充优化。

low-hit 完整比值 **1.191 / 1.520**、增量 **47.546 / 129.800ms**；候选绝对增量 40ms 仍失败。pure-miss 的比值 **0.652 / 0.679** 只是本轮时间区组的点估计：off 上游 P95=281.685ms，而 on=154.913ms，主模型耗时明显漂移，不能解释为 cache 让纯 miss 更快。

六区组探索性 bootstrap 区间很宽：low-hit on 比值约 **0.867–1.640**、memo **0.876–2.863**；pure-miss on **0.272–1.338**、memo **0.248–1.593**。本轮不能确定固定串行成本的下限，亦不能确定正式比例 SLO。候选合同仍用于诊断，原失败证据保留；不以改文档掩盖失败。

最初每个 runner 重启 Qdrant，历史集合加载曾超过旧 30s 就绪窗口；随后保持依赖热态。即使如此，low-hit 首区组和 pure-miss 第四区组仍出现 503。失败区组仅复查一次，记录在 [block-selection.json](measurements/policy-execution-20261003/warm-batch/block-selection.json)：low-hit 复查出现 2 个 off **400 / provider_invalid_request**，明确纳入诊断分布和失败判定；pure-miss 复查通过，原先 2 个 memo 503 仍保留。成功复查不消除总体可用性风险。

两次 400 的网关完成时间为 `2026-10-04T06:47:42Z / 06:47:43Z`，request ID 为 `diag-71 / diag-85`。LM Studio 在对应本地时段 `2026-10-03 23:47:41 / 23:47:43` 记录 `Channel Error`，原因链为 `Engine protocol predict request failed: fetch failed`，见[安全摘录](measurements/policy-execution-20261003/lmstudio-engine-errors.log)。这支持优先排查模型服务内部引擎连接；日志尚未共享端到端 request ID，关联依据是时间与错误次数，不足以确认底层根因。不能将这两次 400 归为 Redis 503，也不能认定测试请求参数非法。

重复 hit 本轮 P95：普通 **61.781ms**、memo **9.405ms**、同题 off **280.433ms**。memo 满足候选 hit 预算；普通 hit 超过 60ms。此前 6.444ms 属于旧运行，不替换本轮数值。

## 容量实测

每项 100 次真实请求，客户端持续填满并发槽，无保护限流，以测上游容量曲线；该模式的 actual RPS 不等于稳态计划到达率。

| 在途上限 | bypass RPS / P95 ms | miss RPS / P95 ms |
| --- | ---: | ---: |
| 1 | 8.46 / 143.029 | 7.39 / 155.542 |
| 2 | 11.43 / 208.541 | 9.57 / 275.419 |
| 4 | 14.00 / 349.319 | 13.86 / 350.315 |
| 8 | 13.78 / 702.802 | 13.06 / 771.240 |

800 次容量请求零失败。C8 相比 C4 不增加吞吐，P95 约翻倍，支持把四个槽作为该部署的进一步候选；不证明所有模型/硬件的最优并发都是四。保护测试另外验证满额立即 429、零额外模型调用和恢复。多进程集群总限制尚需单独配置决策。

## 仍需排查与可执行验证

| 优先级与依据 | 下一步具体操作 | 预期结果/所需资料 |
| --- | --- | --- |
| P0：Redis 健康读取故障被呈现为 provider 不健康 | 对齐失败 request ID 的 `routing`、`health_provider_sync`、Redis 命令和 VM 调度时序；注入 40/60/100ms Redis 延迟，分别测试真实 provider 健康和不健康；先定义快照不可用策略，再比较有界旧快照/明确依赖失败 | 解释当前 50ms snapshot 截止为何触发 `no_healthy_provider`；保留依赖错误，不能把真正不健康 provider 放行。需快照最大可接受陈旧时间、集群一致性/故障策略和 VM/host 调度数据 |
| P0：上游 400 与模型内部 Channel Error 同时出现 | 在隔离模型实例保留失败响应的安全错误字段、转发 request ID；对齐引擎进程/连接、Windows 调度与模型服务日志；按原请求重放 C1/C4，先复现再判断是否需要特定上游错误映射 | 区分真实参数错误与引擎连接失败，避免修改请求或自动重试掩盖服务故障。需 LM Studio/引擎版本、底层 fetch 的 cause/errno 与完整进程生命周期；当前时间关联不等于根因证明 |
| P1：本体 P95 与上游区组均明显抖动 | 使用专用测试实例或新隔离数据集；记录集合数/规模、VM steal/CPU、Redis latency、模型权重/量化/线程/slot；冻结条件后复跑六区组，保留所有失败，不无限重复直到 PASS | 取得有稳定对照的 residual 和完整响应 P95/P99；解释 off 比 on 更慢的区组效应。需后端请求 ID 与队列/推理时序，不能用客户端 P95 相减当推理成本 |
| P1：语义泛化收益仍未批准 | 当前政策 FAQ 保持 exact/disabled；由业务冻结新同答案标注集、校准集与留出集；定义误复用容忍度和最低召回后只执行冻结候选 | 旧 32 问仅回归，41 问零 exact 误命中不代表语义召回达标。业务数据与目标未提供，暂不加入 reranker/特征规则或恢复政策泛化 |
| P1：真实模型冷启动缺失 | 在明确隔离 embedding 实例上执行卸载→首请求→加载→热态，检查缓存 read deadline 旁路、后台写入/permit 回收和后续恢复 | 需隔离实例地址、启动/卸载方式和模型服务参数。本轮未卸载共享 LM Studio；首次请求或并发预热不称为真正冷态 |
| P2：生产预算与容量值 | 将本轮 C4 拐点、各阶段截止及三类请求预算交业务/部署 owner 冻结；单独配置共享 chat/embedding resource group 和多实例总容量 | 测试使用的 1s/5s/30s、cap1/cap4 均不是生产默认值。还需生产完整响应、拒绝率、停止期限和 publisher/version-switch ownership |

## 复现和证据边界

代码选项见 [复用策略](../../../docs/cache-reuse-policy.md) 与 [Provider 保护](../../../docs/provider-protection.md)。入口、构建及环境注入步骤见 [README](measurements/policy-execution-20261003/README.md)。[summary.json](measurements/policy-execution-20261003/summary.json)列出每次真实测试状态、失败 HTTP 码和最终回归；[manifest](measurements/policy-execution-20261003/manifest.json)记录源码、binary、原始日志的 SHA-256。

本轮性能和首次安全测试使用 `app-linux-final.test`；最终 usage 修复与 Provider/SSE/Fusion 验证使用 `app-linux-verified.test`，全量 Go 回归从最终工作区编译。两次构建间产品变化仅为流式请求 usage 选项，不更改非流式性能路径；binary 差异在 manifest 中显式记录。最初失败、复查和最终回归保存在不同目录。

生产缓存配置和空白名单未修改。没有执行远程付费模型测试、额度购买、集群上线、历史集合删除或共享模型卸载。语义质量、冷态、稳定性能及可用性仍为开放项。
