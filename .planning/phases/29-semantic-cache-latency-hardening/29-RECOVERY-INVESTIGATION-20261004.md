# Phase 29 恢复与集合生命周期排查 — 2026-10-04

**本轮确认了 Qdrant 启动时的资源压力，并发现大量稀疏预分配文件；没有取得新的正式性能 PASS。** 120 秒恢复检查失败后，未启动原计划的 24 个暖机对照窗。随后完成七个无业务请求的组件隔离条件和停止状态下的只读文件检查。产品实现、模型、VM 配置、原 P95 门槛及生产缓存启用状态均未改变。

前一轮 [内存升级报告](29-MEMORY-UPGRADE-RESULTS-20261004.md) 的 4,800 请求仍是独立历史证据。本轮资料在 [证据说明](measurements/primary-warmup-investigation-20261004/README.md)，包括失败、原始采样、源码/binary 指纹及凭据审计。

## 控制变量与实际执行

继续使用同一 guest（4 vCPU、7,376MiB），同一现有数据卷和已加载的 Qwen/Nomic 参数。初始两个模型 idle、queued=0。本轮没有模型重载、付费提供商请求、清除 swap、修改 swappiness、集合删除、数据迁移或生产部署。

首个计划只比较 cache-off 下的一次/八次 sequential primary 暖机，六组 ABBA/BAAB，每窗 100 请求、125ms 间隔、在途上限四，统一静置两秒。binary 上传和转发建立各一次，保留所有早期请求。新增夹具已编译，但 **24 窗 / 2,400 正式请求实际均未运行**。

依赖启动/readiness 与 backend 60 秒测试期限分开。正式窗前要求连续十个 vmstat 区间满足 si=so=0、块读写各≤1,024KiB/s、system CPU<15%、I/O wait<5%；启动时 MemAvailable≥1GiB，PSI 单列记录。这些是拟议诊断条件，不能当作已批准的发布 SLO。120 秒内取得 119 个有效区间，84 秒有交换活动，最长连续安静六秒，因此按预定条件失败并停止。

没有放宽门槛或选择性重跑正式窗。随后单独登记并运行：all-off 30s → Redis-only 45s → all-off 30s → PostgreSQL-only 45s → all-off 30s → Qdrant-only 120s → all-off 30s。一次只启动一个现有测试容器，不发送业务、embedding 或主模型请求，不新建语义集合。连续 VM/宿主机采样，每五秒读一次 MemAvailable、PSI、容器 cgroup 和进程状态。各条件的统计截止于最后一次状态快照，排除之后的 docker stop；第一条 vmstat 启动以来平均值也排除。

这七个条件各运行一次且顺序固定，属于定位实验，不是随机重复的集合数量因果 A/B，也不是应用性能验收。

## 实测结果

| 组件独立运行 | 最终 cgroup memory.current / swap.current | 最大 swap-in / swap-out KiB/s | 最大块读 KiB/s / I/O wait / system CPU |
| --- | --- | --- | --- |
| Redis | 80.88MiB / 0 | 1,092 / 0 | 82,092 / 1% / 23% |
| PostgreSQL | 48.80MiB / 0 | 80 / 0 | 25,128 / 1% / 23% |
| Qdrant | 6.30GiB / 24.17MiB | 9,380 / 63,948 | 191,900 / 11% / 74% |

Qdrant 条件有 124 个一秒采样区间，60 秒有交换活动；最后 30 秒仍有九秒 swap-in，最大 9,380KiB/s。三组件全部停止的各基线也有少量零星 swap-in，因此“任何全局 swap-in 都等于业务瓶颈”不成立，十秒绝对零条件的适用性仍需评估。首次失败保留，不能因为后面某些空闲区间恢复就改写成 PASS。

Qdrant 最终计账约 6.30GiB 中，**file=6.17GiB、file_mapped=5.55GiB、anon=38.79MiB、kernel≈98.22MiB**。进程 VmRSS≈5.59GiB、VmSwap≈15.42MiB；RSS 包含映射文件，不能把它全当作不可回收匿名内存。guest 同时 MemAvailable≈6.50GiB。启动期间出现了实际回收/交换和 PSI 累计增长，最终 avg10=0 则反映压力后来减弱，不能代替整个启动过程。

容器 cgroup 的 pswpin/pswpout 提供了进一步的组件归属证据；pgmajfault 本身同时包含文件缺页，不能全部当作 swap。当前 evidence 支持优先调查 Qdrant 启动，尚不能排除背景进程、顺序/页缓存状态或存储延迟影响。

## 存储布局与代码依据

停止容器后直接访问 Docker volume 被 OS 文件权限拒绝，原错误保留。改用已缓存的 PostgreSQL image 运行短暂只读检查 helper：无网络、无拉取、root filesystem/volume 均只读、128MiB/0.5CPU 限额，成功退出后自动移除。只读取文件元数据，不读取配置或向量内容。

卷的逻辑长度 **63.80GiB**，磁盘实际分配 **212.27MiB**。这不是磁盘真的被占满，也不表示需要 64GiB RAM；稀疏文件的空洞、页缓存与驻留页必须分开计量。

| 文件族 | 数量 | 逻辑长度合计 | 说明 |
| --- | --- | --- | --- |
| WAL | 1,041 | 21.69GiB | 实际分配约 83.44MiB |
| payload page_0.dat | 694 | 21.69GiB | 多数空间是稀疏预分配 |
| vector chunk_0.mmap 等 | 609 | 19.03GiB | 实际分配约 61.88MiB |

共检查 13,306 个文件。文件系统中 347 个 collection 名称与本轮启动后 REST 清单完全相同，其中 semantic 为 323。没有增删集合。此前 20,544 点/694 segment 来自独立的完整元数据清点；本轮没有重新调用每集合 API 验证点数，不能将其描述为最新逐点清点。

代码中 `vectorCollection(scope, model)` 把 scope/model 的 hash 转成单独 collection。`discard()` 调用的是向量点 Delete，再移除可丢弃条目；没有删除 collection。这意味着条目过期不等于集合生命周期结束。新 key/version/system/settings 的隔离 scope 会增加集合；测试还为每窗使用临时 SQLite 和新 key，结束后没有一个持续运行的原 repository 为这些历史条目执行清理。

**待证实的机制：** 多集合/segment 的预分配与加载行为放大启动时的映射、页缓存触碰和回收。当前文件布局与计账相符，但还缺按文件的驻留/预取归属，以及同点数、不同集合数的重复对照；不能声称已证明哪一个文件族或哪个设置是唯一根因。

## 主模型首批尖峰与 P95 判断

重新校验前一轮十二个正式时间块，off 中 14 个 >250ms 样本全部在 off-before 的第 1–3 请求（8/4/2 个）；profile-smoke 不混入统计。慢首请求已复用连接，写出约 0.03ms，主要等待 provider 非流式首字节，多数尖峰当秒无 VM swap-in。因此，Qdrant 启动压力和 primary 首批尖峰是两个仍需分别验证的问题，不能直接认定前者解释后者。

此前一次直接探测确认 LM Studio CLI 可提供 timeToFirstToken、totalTime、tokensPerSecond；HTTP response 的 stats 却为空。新增 observer 仅保存输出 hash 和 timing，完整跨窗 hash/时间匹配尚未执行。一次直接 probe 不记为 P95 性能测试。

**本轮不改变性能结论：** 原 1.05× 仍是前一轮失败记录；候选 AND（≤1.25× 且≤off+40ms）与 residual 预算只获得先前点估计支持，没有因本轮恢复失败而自动通过。更无法据此宣告 1.05× 普遍物理不可达。也不建议仅看到 memory.current≈6.3GiB 就继续扩容或提高超时。

## 下一步方案

| 优先级与依据 | 具体步骤 | 预期结果 / 必需资料 |
| --- | --- | --- |
| P1：Qdrant 无请求也触发启动压力 | 先记录 image/version、每集合 segment/vector/payload/WAL 配置；在独立测试卷生成相同向量、维度、总点数，比较少集合与接近当前数量的多集合，固定 CPU/RAM。两种布局交替至少三次，启动与稳态分别计时，保留全部失败，记录 file/anon、cgroup pswpin/out、PSI 和 I/O；保留现有卷 | 若多集合持续放大相同数据量的启动成本，才支持集合粒度调整。需固定 Qdrant 版本、配置、页缓存起始策略及小容量数据生成清单；不能用当前旧卷与空卷比较，或把所有配置一起改 |
| P1：条目清理未关闭 collection 生命周期 | 先设计有界 collection 生命周期与过期/停用 scope 的 dry-run 清单，区分当前有效 scope、历史测试 scope、其他业务 collection；shared collection 方案按 embedding 维度/版本分区，必须在查询前强制 opaque scope filter，并在候选返回后再次验证 scope/model/版本 | TTL/禁用/换版本/重启后数量应有界；跨 key/model/version/system/settings 候选必须拒绝。需要持久 repository 对 scope 所有权和并发写入的真实资料。未经具体审阅不删除现有集合、不迁移数据 |
| P1：primary 首批尖峰尚未解释 | 恢复成立后执行已准备的一次/八次暖机 ABBA/BAAB；固定上传/转发时点和 generation 设置，保留首 3/10 请求；用输出 hash+完成时间匹配 LM TTFT/total/TPS，统计不匹配率、位置效应与 bootstrap 区间 | 区分暖机效果与测试顺序/host 调度；sequential 八次暖机不保证覆盖四个并行 slot。仍需要 engine queue/prefill/decode 或服务内时间线才能进一步拆分 TTFT |
| P0：业务语义安全与发布合同保持开放 | 独立冻结业务标注集和白名单，覆盖数值、否定、主体、权限和版本。先写出答案等价标签与 unsafe-hit=0、recall 目标，再测试；性能实验不代替质量评测 | 任一不安全命中拒绝对应适用域；需业务认可的留出集与召回目标。原 `.84` 3/20 不安全命中结论未被本轮改变 |

恢复门槛应先定义清楚“背景的少量回读”与“当前持续内存停顿”的区别，再登记新协议。不得在同一失败运行中提高截止或删除前 N 请求求 PASS。若在独立的最小 cache-off 环境做 primary 对照，也必须明确其不能代表带 Qdrant 的 cache-on 性能。

## 验证、清理与限制

新增 opt-in 夹具/runner/storage helper 通过 Go vet、Linux tagged 编译/Windows helper 构建及复杂度检查；Python 分析成功。既有全量后端结果不重复运行或累计。七个 idle 条件均完成，属于诊断观察，不记作七个 backend 测试 PASS。正式性能窗为零。

三个现有测试容器最终均 stopped、OOM=false、restarts=0；本轮托管 observer、SSH 转发和 idle 采样已退出，临时只读 helper 已移除，现有卷及模型保留。另一个在前置调查中由工具启动的只读 `lms log stream`（PID 33964）仍存在：Ctrl-C 和定向终止均未成功，Windows 返回 Access denied。它不是推理服务，但清理尚未完成；不把该项写成全部进程已退出。错误与范围记录于 `cleanup-limitations.json`。

Phase 29 保持 partial。产品源码没有因这次定位被改写，正式缓存仍关闭，原始失败和未完成项目继续可见。
