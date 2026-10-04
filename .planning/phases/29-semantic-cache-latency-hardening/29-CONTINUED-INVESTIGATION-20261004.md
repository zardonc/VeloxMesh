# Phase 27–29：硬件优先排查与局部修复 — 2026-10-04

**当前证据不支持“8 RPS 性能差距主要由 VM 硬件容量不足造成”，暂不建议扩容。** 全新隔离卷能通过原恢复条件，但纯 miss 仍不满足原 1.05× 门槛；新增前台 embedding/查询成本与主模型波动需要分别处理。本轮还复现并修复了 Gemini 取消后误结算，真实取消、中断、正常完成三项回归通过。Phase 29 保持 partial，生产缓存保持关闭。

完整证据、执行脚本、参数、失败和哈希在 [本轮目录](measurements/continued-investigation-20261004/)。引用聊天的分析作为调查输入；结果以本轮实际日志为准。此前已有工作区修改均保留，没有提交、推送、合并或部署。

## 硬件与环境隔离

| 环境 | 本轮核对结果 |
| --- | --- |
| 宿主机 | Ryzen 9 8945HS，16 个逻辑处理器；系统可见内存约 31.25GiB（32GB 规格） |
| GPU | RTX 4060 Laptop，8,188MiB 显存；测试模型已加载 |
| VM | 4 vCPU，7,376MiB 内存；未调整 CPU/RAM/swap/内核参数 |
| 依赖 | 从现有测试容器继承同一缓存 image ID、启动参数与私有环境；新建三个有唯一标签的空卷/容器，仅绑定 loopback |
| 旧资源 | 开始时没有容器运行，测试端口空闲；历史卷和集合没有挂载到本轮实例 |

新卷启动后连续十秒 si=so=0 且 CPU/I/O 满足原恢复条件。旧卷此前 120 秒未达到同一条件的失败保留，不能被本轮改写。没有清空系统 swap、全局页缓存或删除历史集合。历史 `lms log stream` PID 33964 本轮检查为 ESRCH，已经不存在。

此前的同点数、1/323 集合重复实验已经显示同硬件上集合布局会放大启动和查询成本，参见 [原实验](measurements/layout-clean-investigation-20261004/PLAN.md)。该实验关闭向量索引，不等于生产索引配置的最终容量结论；本轮新卷也不能替代相同数据量的生产容量 A/B。

## 8 RPS：没有持续资源饱和，纯 miss 门槛仍失败

两组有效窗口各为 direct→off-before→on→memo→off-after，每窗 100 请求、125ms 间隔。**1,000/1,000 请求成功**，实际 8.00–8.02 RPS，实际最大在途为 2（配置上限 4）。保留全部早期请求；每个 cache 窗口在 seed 持久化和两秒静置后正式计时。

| 组 | off-before / off-after P95 | on P95 | memo P95 | on / memo 比较比值 |
| --- | --- | --- | --- | --- |
| target-rate-1 | 119.990 / 118.507ms | 144.945ms | 143.368ms | 1.208 / 1.195 |
| target-rate-2 | 122.242 / 126.604ms | 143.606ms | 150.289ms | 1.134 / 1.187 |

比值使用两侧 **较慢** 的 off P95 作为保守锚点；四项仍超过 1.05。前后基线漂移分别约 -1.24%、+3.57%。这是两个诊断块，不是六块正式发布验收，不把候选合同点估计当作批准。

这些有效窗口的同步采样：宿主机 CPU 峰值约 45.21%，最低 available 3,646MiB；VM CPU 峰值 5%（应用路径峰值 4%），I/O wait=0、swap-out=0，零星 swap-in 最大 8KiB/s；memory PSI 累计增量为 0–1μs。GPU 利用率采样峰值 60%，空闲显存至少 6,080MiB。没有本轮 OOM、重启或持续资源耗尽证据。

cache-on/memo 的 embedding HTTP P95 为 17.66–20.45ms，cache-read P95 为 19.82–22.26ms；逐请求扣除 provider/cache 阶段后，Application Residual P95 约 4.37–5.00ms。不能把不同阶段的 P95 相加减当作单次成本，但调用路径与逐请求关联支持优先处理串行 embedding/查询开销。扩大 VM RAM 不会自动删除这次往返。

另有 12,000 个组件探测样本、四项真实组件测试全部通过：Redis health set/get 在 C1 的 P95 0.874ms；Qdrant 查询 C1 P95 1.622ms，C16 P95 9.836ms。更高并发下 SQLite/PG 写读尾延迟上升，不能据此宣称无限容量。分测试/并发结果见 [评估 JSON](measurements/continued-investigation-20261004/hardware-evaluation.json)。

## 1 RPS：主模型波动仍需调查，不能用于通过验收

补做的 150 次正式请求全部成功，原始 metadata 确认 count=30、interval=1,000ms。直连 P95 404.967ms；off-before/on/memo/off-after 为 380.530/348.579/361.330/340.112ms。off 的 provider P95 376.909ms、Application Residual P95 4.70ms，尖峰主要在等待模型首字节，不在 VM 应用处理。

off-after 在 **20:53:33–34 UTC / 13:53:33–34 PDT** 采样到宿主机 CPU 98.83%/87.79%；前后基线漂移 -10.62%。该组只能用于诊断，不能用 on/off 的点估计“通过”抵消 8 RPS 的失败。当前缺后台进程归因、引擎排队/prefill/decode 和精细电源状态；1 秒 GPU 采样也可能与 1 RPS 相位重合，不能据此判断每次推理没有 GPU 活动。

尚未做独占卸载/重载、VM resize A/B、生产规模持续写入或 hypervisor CPU-ready 观测。因此结论限于当前加载模型和测试 FAQ 的 8 RPS 稳态：没有资源容量不足证据；不能保证其他负载或所有短时竞争均无硬件因素。

## Phase 27：Gemini 非终态退出误结算已复现并修复

受控转发只交付真实 Gemini 的首个非终态 SSE frame；内容只保留 SHA256。随后 gate 阻止后续终态交付，直到客户端断开。该实验控制的是**网关观察到的交付完成顺序**，不声称知道外部提供商物理计算的完成时间。

修复前：非终态交付→客户端关闭→上游 context 约 0.268ms 后取消，仍出现一条 settled Usage、15 tokens/15 credits。另一个真实字节流受控 UnexpectedEOF 也未返回所需协议错误。源代码确认当前 `genai v1.60.0` 在 scanner 出错时仅记录日志，迭代器结束没有 yield error；适配器把这种结束发为成功 Done。

局部修改仅在 `internal/providers/gemini/adapter.go`：向 forwardStream 传递 context；context 取消保留为 `context.Canceled`；迭代结束后核对 context 和原生 terminal 状态，缺失终态返回 `provider_bad_response`。没有修改结算合同、重试策略或超时。

| 修复后真实用例 | 结果 |
| --- | --- |
| CancelBeforeComplete | PASS；传播约 0.182ms，零 Usage/扣费，无健康惩罚或 pending 泄漏 |
| InterruptedBody | PASS；真实非终态流受控中断返回协议错误，零结算 |
| CompleteBeforeCancel | PASS；正常完成结算一次，随后关闭不会重复扣费 |

日志见 [修复后目录](measurements/continued-investigation-20261004/gemini-terminal-green/)。修复前失败单独保留，不累计为通过。修复覆盖取消和终态前中断，没有据此宣称所有 SDK/网络错误或终态后的恶意帧均已覆盖。

## Phase 28：响应头等待超时仍存在，责任边界更明确

原生 SDK 工具流成功：HTTP 200、首响应约 2,404ms、总计约 2,453ms，含真实签名。紧接着网关工具流失败：DNS/TCP/TLS 完成，请求在约 51.34ms 已写出，此后没有 first-byte，约 11,999ms 后由既有客户端 12 秒期限取消。没有进入签名续接。

这次失败发生在请求已写出后的上游响应等待段，不支持归因于 VM 算力不足，也不足以单独认定网关请求转换或某个上游节点是唯一根因。原生和网关参数由各自现有协议转换生成，没有冻结到相同 wire body 的严格因果对照。保留请求耗时和状态，未提高超时或自动重试；三组计划在第一组真实失败后停止，剩余组未运行。独立的取消实验另行执行。

真实客户端扩展签名保留、多工具和恶意尾帧覆盖仍待验收。

## Phase 29：其余当前判断

| 项目 | 本轮证据与后续边界 |
| --- | --- |
| 种子答案事实错误 | 三条固定 FAQ 的直连和网关均在取消后的访问期限上失败：模型答成“取消后 30 天”，预期是当前账期结束。再次证明错误在种子生成路径；exact 命中一致性不能证明种子正确。保留业务来源/审核/版本前置条件 |
| 首次 scope 初始化 | 新卷下首个 lookup 仍报 Qdrant Collection NotFound；随后异步创建并正常测试。属于初始化语义问题，不是旧文件或硬件不足造成 |
| 集合生命周期 | 所有临时测试库结束后新卷仍有 11 个集合，其中 9 个 semantic；源码点级清理不删除 collection。未等待业务 TTL 全部到期，不能把该清点说成新的 TTL 故障。仍需明确持久 scope 所有权和回收协议 |
| 自然 Redis 停顿、LM Studio 历史 400 | 新卷短窗没有再现这些历史故障；组件通过只能证明本轮窗口可用，不能关闭间歇故障 |
| 语义策略及发布合同 | 本轮没有重标业务集、降低阈值或启用生产白名单。历史 unsafe-hit、召回、真正冷加载、不同维度生命周期及生产容量合同仍未关闭 |

优先顺序：保留业务语义安全与种子正确性发布前置条件；技术排查继续聚焦集合初始化/生命周期、模型首字节和后台竞争，再做稳定的正式性能复验。当前不建议先扩容、放宽 health deadline 或用重试隐藏上游等待失败。

## 验证、失败保留与清理

`go build ./...`、`go vet -tags phase29preflight ./...`、runner vet、Linux tagged 编译、脚本语法和 diff 检查通过。全量后端内外均为 60 秒硬上限。首次全量运行暴露既有 PubSub 测试竞态：消息收到后测试提前返回，异步 publisher 尚未拿到确认，client 被关闭后 goroutine 在已结束的测试上调用 Errorf。把该测试改为同步等待发布确认，保留原 panic 后，最终独立全量结果 **595 个顶层 PASS、0 FAIL、2 个显式 opt-in SKIP**；SKIP 不计通过。

另外保留两个夹具问题：首次“low-rate”参数被旧 helper 的环境覆盖，实际是 100 请求/125ms，500 个样本不计入预登记对照；补做 1 RPS 使用明确参数，未覆盖旧记录。首次 Gemini timeline wrapper 二次读取 stdin，在任何上游调用前失败；提取已有用例 helper 后另目录完整运行。均不伪装为产品或上游故障。

新建测试容器全部 stopped、OOM=false、restarts=0；三个原测试容器也保持 stopped。本轮 observer、GPU 采样、SSH 转发和测试进程已退出；远端本轮专用二进制经记录 hash 后删除，端口仅剩 SSH/DNS。历史卷和本轮停止的诊断卷保留，用户模型服务保留。没有广泛 prune、迁移或生产清理。凭据审计和最终源/证据哈希见 [manifest](measurements/continued-investigation-20261004/manifest.json)。
