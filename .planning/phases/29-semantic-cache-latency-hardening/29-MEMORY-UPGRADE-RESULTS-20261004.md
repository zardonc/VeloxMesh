# Phase 29 内存升级后对照测试 — 2026-10-04

本轮正式性能请求 **4,800/4,800 成功**，69 次选定真实测试调用全部 PASS；全量后端 595 个顶层测试 PASS、0 FAIL。正常 Application Residual P95 为 4.92–5.50ms。两场景的候选性能合同点估计通过，**原 1.05× 仍失败**。前后基线漂移、暖机后换页以及主模型首批请求尖峰仍存在，因此不能据此关闭性能调查、证明内存升级的因果效果，或宣称 1.05 在物理上绝对不可达。

这是对 [可用性与尾延迟调查](29-AVAILABILITY-INVESTIGATION-20261004.md) 的补充。产品源码来自 HEAD `9c323d6`；新增修改限于测试夹具、编排、观测和记录。源码及 binary 指纹、原始日志与复现方法见 [证据目录](measurements/memory-upgrade-controlled-20261004/README.md)。Phase 29 仍为 `partial`。

## 环境与控制变量

升级前只读检查得到 guest 内存 2,534MiB；本轮为 **7,376MiB（约 7.20GiB）**，CPU 两次均为 **4 vCPU**。不把此前用户描述的“2 核”当成实测 CPU，也不推断配置总量与 guest 可用总量之间差额的原因。

仅使用既有隔离 Redis/Qdrant/PostgreSQL 和本机 LM Studio：Qwen2.5 0.5B Q8_0、context=8192、parallel=4；Nomic v2 MoE Q8_0、context=512、实际 embedding 维度 768。前后模型 inventory 参数一致、最终 idle/queued=0。没有重载模型、远程付费调用或生产写入。

低命中与纯未命中场景各六个时间块，每块四窗：`off-before → on → memo → off-after` 与 `off-before → memo → on → off-after` 交替。每窗 100 请求、125ms 计划间隔、在途上限 4、同一 FAQ/生成设置、相同数据卷与 cache bounds。每窗新建非管理员测试 key/SQLite/opaque scope，防止上一窗答案让下一窗变成高命中测试。Memo 关闭时 capacity=0、TTL 为空；开启时 capacity=128、TTL=1m。三种配置先经过独立短预检。

依赖在整批保持运行，启动恢复单列；readiness 后等待 15s。每窗真实 primary 暖机一次，on/memo 等 seed 异步持久化成功，再统一静置 2s 才开始正式计时。没有并行跑压力工具、编译或其他测试。VM 使用一条 `vmstat` 持续采样，宿主机使用每秒一次的原生 CPU/内存 API；集合清点在全部测试结束后单独进行。

**仍未充分控制的因素：** off 的位置固定在每块两端；binary 每块开始上传一次；主模型只做一次暖机；宿主机未证明独占；新 scope 的写入会增加既有 Qdrant 卷的集合/点数。它们没有被悄悄改动，但固定等待不足以证明系统已进入稳定状态。实际峰值并发由响应时长决定，并非人为给 on 降低上限。所有块原样保留，无失败替换或选择性复查。

## 性能结果

以下为完整响应时长，单位 ms。off 合并每块前后两个基线；延迟按原始请求汇总，RPS 按各窗实际时长计算。

| 场景 / 模式 | 请求 / hit / HTTP 失败 | 实际 RPS / 峰值并发 | P50 / P95 / P99 | Residual P95 / P99 |
| --- | --- | --- | --- | --- |
| 低命中 off | 1200 / 0 / 0 | 8.014 / 4 | 101.473 / 131.190 / 212.432 | 4.919 / 6.284 |
| 低命中 on，Memo 关 | 600 / 24 / 0 | 8.004 / 2 | 117.066 / 154.868 / 192.201 | 5.168 / 6.767 |
| 低命中 on，Memo 开 | 600 / 24 / 0 | 8.009 / 2 | 116.140 / 149.981 / 177.547 | 4.932 / 5.640 |
| 纯 miss off | 1200 / 0 / 0 | 8.016 / 4 | 101.858 / 130.964 / 201.592 | 5.202 / 7.828 |
| 纯 miss on，Memo 关 | 600 / 0 / 0 | 8.004 / 2 | 119.868 / 158.894 / 218.735 | 5.497 / 8.318 |
| 纯 miss on，Memo 开 | 600 / 0 / 0 | 8.006 / 3 | 116.047 / 153.743 / 189.153 | 5.030 / 5.958 |

低命中 on/memo 整体比值为 **1.1805 / 1.1432**，增量 **23.678 / 18.791ms**；纯 miss 为 **1.2133 / 1.1739**，增量 **27.930 / 22.779ms**。候选要求同时满足 `on≤1.25×off`、`on≤off+40ms`，以及 residual P95≤10/P99≤15ms；两场景点估计通过。原 1.05 的四个比较均失败。

低命中还按相同计划发送位置比较 miss：matched-off P95=132.488ms，on/memo miss P95=155.298/154.008ms，比值 1.1722/1.1624。不能用全体延迟分位数相减，冒充单次缓存或 CPU 成本。

六时间块联合 bootstrap（seed=29、2000 次、探索性 95% 区间）：低命中 on 比值 1.120–1.216、memo 1.038–1.257；纯 miss on 1.091–1.234、memo 1.035–1.205。低命中 memo 上界越过 1.25。低命中第 4/5 块基线漂移为 -19.14%/-17.63%；纯 miss 第 2/3 块为 -12.50%/-18.09%。10% 是诊断提示线，不是新发布标准，也未用于剔除任何块。

低命中 24 次 hit 的 P95 从无 Memo 的 31.632ms 降至有 Memo 的 4.862ms；样本较少，另有真实 hit/billing 回归通过。纯 miss 的 Memo hit 为零，仍有 600 次前台 embedding，不能把该场景的较低点估计归因于 Memo 消除了 embedding。on cache-read P95 为 23.326/24.101ms，embedding HTTP P95 为 20.975/21.385ms，符合串行依赖仍占主要新增时间的判断；这些阶段的 P95 不能相加成整体 P95。

**对标准的结论：** 当前本地快模型路径持续不满足 1.05，按场景使用绝对增量和 residual 预算有依据。但仍缺稳定 primary/环境下的逐请求新增成本下界，不能把经验失败宣布为普遍物理限制。增加 VM 内存也不能消除前台 embedding 的串行往返。候选点估计通过不等于已经获得稳定发布证据。

## 换页、基线尖峰与集合数量

| 独立观测区间 | VM 采样秒数 / 有交换活动的秒数 | 最大 swap-in / swap-out KiB/s | 最大系统 CPU / I/O wait |
| --- | --- | --- | --- |
| 启动与预检 | 50 / 48 | 14268 / 30220 | 76% / 6% |
| 正式窗起止范围（含末尾写入 drain） | 601 / 74 | 9276 / 0 | 8% / 2% |

启动时块读最高 143,644KiB/s；正式窗最高 9,280KiB/s。正式时段宿主机 CPU P95=51.35%、峰值 72.40%，最小 available 约 3.58GiB。后端编译等其他阶段不混入这两个区间。第一条 vmstat 启动以来平均值已排除。升级前后的 workload、暖机和观测不同，不能据此作严格 RAM A/B 因果结论。

off 中 14 个 >250ms 样本全部出现在各窗前 3 个请求。最长样本 424.508ms，其中 provider complete=418.942ms、residual=5.217ms、health sync 合计=0.988ms；另一窗 420.782ms 中 provider=416.888ms、residual=3.347ms。主要尖峰落在主模型路径，不能据这些请求归因于 Redis 50ms 截止。需要 engine queue/prefill/decode 和宿主机进程级指标才能解释其原因。

测试后完整只读清点 **347 个 collection、20,544 个 point、694 个 segment**；其中 **323 个 semantic collection、16,542 个 point**，95 个最多 1 point、289 个最多 100 point、4 个为空。没有删集合、迁移数据或改存储参数。Qdrant 官方说明每个 collection 有独立资源成本，很多小集合会增加开销；这使集合生命周期成为有证据支持的排查方向，尚不能证明它是换页或基线漂移的唯一根因。[Qdrant 多租户说明](https://qdrant.tech/documentation/manage-data/multitenancy/)

三容器最终均 stopped、OOM=false、restarts=0；SSH 转发与采样进程退出。结束后 guest available=6,773MiB、swap used=157MiB；空闲检查没有新的交换活动。**低 free 配合大量 page cache，不等于仍有同量不可回收内存；已占用 swap 也不等于正在换页。** 当前配置能跑完本轮 8RPS 测试，但尚不足以宣布环境稳定或给出生产容量保证，不宜仅凭这些数字再次扩容。

## 功能验证与未关闭项

69 次选定真实测试调用全部 PASS、无 SKIP：包含模型可用性、配置短预检、48 个性能窗、Memo hit/billing、exact 安全及版本隔离、六项 Provider 保护、四项真实流式/取消/缓冲/Fusion，以及三项 Phase 29 acceptance。全量后端另有 595 顶层 / 含 subtests 954 条 PASS，38 个有测试的 package PASS、0 FAIL；两项既有 Plan4 opt-in SKIP，四个无测试 package 不记为通过的测试。

正式 on/memo 的 2,400 条请求均有 lookup 结果，2,352 个 miss 全部 store 成功，其余 48 个为 hit；Memo 命中计数与预期一致。正式窗口无自然 `health_state_unavailable`、provider 400 或缓存读错误。**24 个 on/memo 窗口的 seed 仍各有一次初始 collection NotFound**；等写入后才计时把初始化与稳态分开，没有修复或隐藏这个冷态问题。0/4,800 不关闭历史间歇故障。

没有重新调 embedding 或以这套固定 FAQ 代替业务语义评测。既有 `.84` 不安全负例 3/20、独立业务语料/召回目标未获确认的问题保持开放；exact 41 个变题拒绝和版本隔离通过，只支持 exact 路径。

最初两个编排分别因测试参数 `experimental-semantic` 非法、Memo capacity=0 时 TTL 非空而被配置校验拒绝，已在组边界停止并恢复容器。原证据保留在根目录及 `validated/`，**最终结果只使用完整独立运行的 `final/`**，不是从失败块选择重试结果。分析工具的默认 uv cache 权限及 Windows GBK 解码错误已通过工作区 cache、显式 UTF-8 修正；它们不计为应用故障或通过的测试。Go vet、构建、复杂度/diff 检查与凭据审计通过。

## 下一步可执行测试

| 优先级 / 依据 | 操作步骤与控制变量 | 预期结果 / 仍需资料 |
| --- | --- | --- |
| P1：固定等待后仍换页 | 保持当前 CPU/RAM、模型、50ms health 截止；用至少连续 10 秒 si/so=0、低且稳定的块 I/O/PSI 作为拟议就绪条件，设独立 120s 上限。记录未能稳定而跳过的性能窗；不把跳过写成 PASS。单独记录恢复耗时 | 区分 readiness 与真正稳态；若始终不能稳定，先查 page cache/匿名内存/cgroup/宿主机压力。需加载期间 MemAvailable、Qdrant/Redis RSS 与进程级宿主机指标；本轮缺这些同步资料 |
| P1：347 个集合，多数很小 | 在独立实例/新测试卷按相同总点数、维度和配置比较少集合与多集合；每条件完成后恢复到相同起始数据量，再交替测 restart/readiness/稳态 P95。保留现有卷。先设计 shared-collection + 强制 opaque-scope filter 的原型及跨 key/model/version/设置拒绝测试，再决定架构修改 | 判断独立集合数量是否放大恢复 I/O、RSS 和换页。需核实每集合向量配置、WAL、索引和持久化体量；不能仅按总点数估 RAM。共享方案必须维持 fail-closed 身份隔离 |
| P1：首批 primary 尖峰、基线漂移 | 在独立测量中统一 binary 上传时点；固定多次 primary 暖机并确认 queue=0，所有模式相同。将 off 的位置也做平衡排列，完整保留早期请求，并按贯穿链路的 ID/hash 对齐 TTFB、queue、prefill/decode；不在看到结果后删前 N 个样本 | 证明尖峰来自传输、slot/前缀缓存、推理调度还是外部 CPU 竞争。需要 LM Studio engine 的排队/推理时序与历史 400 的内层 fetch cause/errno；本轮仅定位至 provider_complete |
| P0：业务语义安全尚无代表性批准 | 继续使用已实现的 exact/受控语义适用 profile。在独立标注集冻结模型/前缀/阈值，覆盖数字、否定、主体、版本、权限及同答案改写；先验规定 unsafe-hit=0 和召回目标，先测业务方认可的白名单，之后再评估实体保护或重排 | exact 变题应全部 miss，实验语义的危险负例应零命中；任何一例不安全命中即拒绝对应适用域。需业务认可的答案等价标签、危险负例与 recall 目标；固定 FAQ 压测不能补足这些资料 |
| P2：性能门槛与物理限制判断 | 稳态/主模型控制成立后按预登记六块重测；同时报告 original、候选 AND、miss、residual、失败和区间。若要评估架构可达性，在独立条件中只改变 embedding 的部署距离或执行方式，保持 primary 不变并比较逐请求新增成本 | 给出特定拓扑/模型的可行 SLO，而非普遍“物理不可能”。首次 collection 缺失还需独立 seed→store→repeat 流程与真实错误矩阵，不以无限提高 timeout 或宽泛吞错达标 |

后续顺序：先补恢复/集合和主模型时序证据，业务安全评测独立进行；在这些控制成立后再把候选预算用于稳定性能验收。单纯修改阈值不会消除本轮已经观测到的漂移与冷态问题。
