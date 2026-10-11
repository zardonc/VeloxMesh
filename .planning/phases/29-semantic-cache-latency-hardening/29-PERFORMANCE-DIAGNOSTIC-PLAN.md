---
phase: 29
date: 2026-10-02
status: proposed_test_plan
implementation_started: false
performance_signoff: false
merge_allowed: false
---

# 请求链路计时可行性与性能测试计划

本计划评估 Phase 27–29 真实应用的业务正确性、网关开销和外部依赖限制。本轮只形成计划，不安装注入工具、不修改运行时、不运行模型请求，不改变既有验收门槛。

## 1. 现状与参考报告的修正

| 项目 | 已核对的事实 | 对测试设计的影响 |
| --- | --- | --- |
| 观测基础 | `go.mod` 已包含 OTel SDK；`internal/observability/tracing.go` 已有 gateway span；缓存操作 observer 已接入最近负载测试 | 复用现有依赖，补关联与正确边界，不新建通用 AOP 框架 |
| 实际顺序 | 请求规则 → cache eligibility/lookup → miss 选路 → admission → scheduler（如启用）→ adapter → 响应规则 → 同步结算 → 异步写入入队 → HTTP 返回 | 参考报告把 admission 放到 embedding 前面，与当前代码不符 |
| admission | `LimitAdmissionController.Admit` 刷新余额、查询限流规则，必要时执行 Redis 限流；release 为 no-op，返回 QueueWaitMs=0 | admission 耗时不是并发许可排队；不能用请求到 admission 完成的差值代表队列等待 |
| cache 读并发 | semaphore 满时立即 fail-open，并不排队等待许可 | 区分 `concurrency_full`、lookup timeout 和真实队列等待 |
| scheduler | 最近 Phase 29 用例设置 `SCHEDULER_ENABLED=false`；启用时 `SynchronousRunner.RunChat` 当前把等待完整结果的总时间赋给 QueueWaitMs | 调度队列必须以入队成功到真正开始 execute 的时间测量，不直接信任现有 QueueWaitMs |
| 已有 span | gateway span 从 service 内开始，未覆盖入口认证与 JSON 解析；`RecordOutcome` 会结束 span，缓存 hit 在响应规则前调用它 | HTTP 根 span 应独立覆盖整个 handler 生命周期；现有属性不作为完整 E2E/TTFT 的计时真值 |
| embedding 耗时 | adapter 调用包括传输、连接池等待、Provider 排队、推理与解析 | 命名为 embedding round-trip；只有 Provider 端证据能进一步分出纯推理 |
| 最近 timeout | GPT 两轮 lookup timeout 分别为 1/32、0/32；本地稳定 12.5/s 两轮均为 0/80 | 不能使用参考报告的“当前 10% 超时率”描述所有现有负载，也不先扩大 100ms 预算 |
| 本地拓扑 | 远程测试主机通过 LAN/SSH 转发访问本地模型；chat 和 embedding 共用模型服务 | 本地测试排除了公网聊天 Provider，但没有排除全部网络或共享资源竞争 |
| 数据规模 | 最近本地每轮 80 请求；32 请求属于较早 GPT 轮次 | 统一记录实际样本规模；100 请求只是最低探索规模，不能保证 P95/P99 稳定 |

相关代码：`internal/gateway/service.go`、`internal/admission/controller.go`、`internal/scheduler/executor.go`、`internal/cache/measurement.go`、`internal/app/live_ship_load_test.go`。
历史证据：[GPT 复测](29-GPT-PERFORMANCE-RETEST-20261002.md)、[本地并发复测](29-LOCAL-QWEN-CONCURRENCY-RETEST-20261002.md)。

## 2. 注入方式评估

| 方式 | 能获得什么 | 局限与选择 |
| --- | --- | --- |
| HTTP middleware + 接口 decorator + 现有 OTel | 入口、auth、Provider、embedding、选路/admission、向量/存储调用的 span；在应用组装处注入，计时和输出集中在观测模块 | 推荐起点。Go 无需复制 Spring 容器；私有逻辑块、队列事件和流式事件仍需少量语义边界钩子 |
| 标准库 `httptrace` + HTTP transport/body wrapper | 连接池等待、DNS、TCP、TLS、请求写完、响应头首字节、body EOF/关闭、连接复用 | 与 SDK/业务逻辑分离；RoundTrip 返回只表示响应头就绪，不能据此结束完整响应 span；gRPC 向量调用需对应拦截器 |
| OTel `otelc` 编译期织入 | 不修改业务源文件，通过 `-toolexec`/函数匹配规则注入计时 | 接近 AOP，官方已有该方案；需单独验证本项目 Go 版本、OTel 版本、CGO/交叉构建、现有手动 span、目标私有方法和规则稳定性，不能直接保证所有场景覆盖 |
| OTel OBI/eBPF 外部采集 | HTTP/gRPC/数据库边界和网络观测，通常不改业务源文件 | 需要符合要求的 Linux 内核与权限；不能自动解释 hit/miss、结算一次性、入队/出队等业务语义，作为补充 |
| `pprof` / runtime trace | CPU、GC、锁、goroutine 阻塞、调度和内存热点 | 归因辅助，不替代按请求计时；按需短窗口采集，并校准自身开销 |

推荐将计时实现、日志格式和导出放在 observability/边界 wrapper 中，业务代码只传 context 并标记少量事件。第一版不引入新的 AOP 框架；若零业务源码修改是优先约束，先做独立 `otelc` 小范围验证，通过后再决定是否采用。两种路径不同时自动与手动记录同一层 span。

已有 `ProviderAdapter`、`StreamAdapter`、`EmbedAdapter`、admission Controller 和存储接口可复用；OpenAI adapter 的私有 HTTP client 与 Gemini 的 SDK client 目前在构造函数内创建，采用 transport 注入需调整组装入口，不能承诺零代码改动。缓存 observer 没有 context/request ID，不能靠并发事件的时间邻近猜测归属。

## 3. 计时模型与关联数据

使用嵌套 span 加事件，而不是要求所有场景都有同一组线性 t0–t10。缺失分支记为 N/A；计时使用同一进程的单调时钟。跨主机不直接相减墙钟时间。

```text
client.scheduled → client.send → client.first_valid_event → client.body_complete
gateway.http
  auth → decode_validate → request_rules → cache.eligibility
  cache.lookup [仅 eligible 且开启]
    embedding [含 HTTP 子事件] → vector.search → candidate_read/validate
  hit: response_rules → encode/write
  miss/bypass:
    route → admission → scheduler.queue [如启用] → provider.attempt
      encode → connect/write/response_first_byte → body_read → decode/normalize
    response_rules → settlement → cache.enqueue → encode/write
cache.write [独立生命周期，关联 originating request ID]
  enqueue_accepted → worker_start → embedding → repository_write → vector_insert
```

| 指标 | 边界/定义 |
| --- | --- |
| client E2E | 请求发送到完整 body 读取完成；原 harness 还包含客户端 JSON 验证，新增字段将其单独列出并保留旧字段口径 |
| client dispatch lag | 实际发起时间减计划发起时间；另报从计划发起到完成的总时间，避免压测器排队被隐藏 |
| gateway HTTP | middleware 进入到 handler 返回，包括认证、返回写入；不声称最后一字节已到客户端 |
| cache lookup | Lookup 开始到 hit/miss/fault 判定；超时返回后底层仍运行的工作单独记录，不能混入主请求关键路径 |
| admission | Admit 开始到返回，区分 credit/rule/Redis；不命名为 queue wait |
| scheduler queue | 成功入队到实际 execute 开始；与 scoring/intake、模型执行分别计时 |
| upstream TTFB | HTTP attempt 开始到响应头首字节；另报请求写完到首字节；不等于首 token 或纯推理 |
| provider full | 每次实际 adapter/HTTP attempt 的完整生命周期，包括读取与协议解析；SDK 重试/redirect 分别标号 |
| stream TTFT | 分别记录首个非空文本片段和首个有效工具片段；单独记录 metadata、首 SSE 帧、终止与尾部 Usage |
| settlement | record 构造、持久化与 cost aggregation 的边界；保留原取消/终止结算语义 |
| write queue | accepted 到 worker_start；enqueue accepted/drop 与每个写入阶段分别记录，不加到前台延迟 |

每条记录包括 run/block/condition/scenario、request ID、trace/span/parent ID、provider/model、operation、attempt、相对起止/耗时、cache outcome、terminal/status/error category、连接复用、Usage/输出长度统计。客户端读取 `X-Request-ID` 等现有头；请求 ID 不作为 Prometheus 标签。

异步写入用 originating request ID 和 span link/明确父关联；它使用自己的已有生命周期预算，不能因前台结束自动取消，也不能为了记录而延长写入。结算使用 Background context 的地方需要保留 trace 关联而不改变取消策略。

边界 wrapper 必须保持原返回值/错误、HTTP flushing、取消、Done/EOF 与 Usage 一次性语义；Provider wrapper 不可丢失 StreamAdapter/EmbedAdapter 能力，也不可增加原 adapter 不支持的能力。流式 span 不能在 Stream 返回 channel 时就结束，不能为观测增加阻塞缓冲或串行消费瓶颈。

诊断期间完整保留客户端尝试与受控范围的 stage 样本。输出使用有界缓冲/批量导出，单独统计丢弃与导出错误；不因观测故障使聊天失败，也不能把丢失证据记为通过。不记录 prompt、答案、工具参数、签名、凭据或原始 URL 查询；保留安全的错误类别和 HTTP 状态。

## 4. 场景与指标矩阵

所有行均报告计划/实际 RPS、实际在途峰值、发起积压、成功/失败/取消数量、客户端及各阶段 P50/P95；样本充足时报告 P99。

| 场景 | 单独关注的指标与正确性 |
| --- | --- |
| 非流式 cache off / 不满足 eligibility 的 bypass | 网关与上游完整响应；cache off/bypass 无缓存依赖 I/O；两种路径不混为一组 |
| 非流式全 miss / 固定低命中 | lookup 增量、embedding/vector、主模型、结算、写队列；低命中完整响应 P95 on/off ≤1.05 仍是既定门槛 |
| 非流式已预热 hit | lookup 与响应规则/写回；不调用主模型，零上游 Usage；高命中不能替代低命中门槛 |
| 普通 SSE / buffered SSE | 上游/客户端首个有效文本、完整流、flush、终止、Usage 与取消；缓冲模式单独统计 |
| 非流式工具 / 工具 SSE 续接 | 首工具片段、完整 arguments、每轮/整轮业务时间与准确性；客户端工具执行时间独立，不算网关耗时 |
| scheduler off / on | 同负载分组；开启后记录真正队列等待，避免现有 QueueWaitMs 口径误导 |
| fallback / Fusion | 每 attempt/member/judge 耗时、失败原因及总链路；并行分支按关键路径分析，不把 span 全部相加 |
| 多 Provider 分流 / embedding 压力与故障 | 按 Provider 分层，单账户负载与总网关负载；embedding 读/写分别统计，fail-open、drop、超时和错误不删除 |

业务回归继续检查 Phase 27 的终止/取消/结算、Phase 28 的工具协议、Phase 29 的正负例、租户/版本/模型隔离和绕过规则。Gemini 客户端签名保留要求按 README；Gemini、SANS 资源缺失期间相应用例记为 deferred，不声称已验证。

## 5. 执行步骤与控制变量

### A. 先验证计时本身

1. 记录失败方式：request ID 串线、并发 observer 丢样本、span 重复/提前结束、SSE buffering、超时 span 未结束、异步父关联丢失、optional interface 丢失、导出积压、PII 泄漏。后续实现前先形成可失败的检查。
2. 最小真实垂直链：非流式 miss → 真实 embedding/Qdrant → 主模型 → 结算 → 异步写入 → 可命中续接，确保每一阶段可关联；再验证 SSE/工具/取消。
3. 使用相同业务 build 与真实组件，比较观测关闭/开启/全量诊断模式。记录采集 CPU、内存、日志/collector 压力及完整响应增量。若采集扰动接近 5% 门槛，不用全量诊断数据直接签署性能验收；降低开销并以常规观测水平复验。
4. 如评估 `otelc`，只对上述最小链构建独立诊断二进制，保存 toolchain/规则/二进制 hash，验证 Go/OTel/CGO/交叉构建兼容与重复 span；失败时采用现有接口边界方案，不装运行时 monkey patch。

### B. 单 Provider 基线与完整缓存对照

1. D：直连真实主模型。使用与 gateway adapter 等价的请求/协议解析，并从同一负载主机沿同一 Provider 传输路径访问；称为“直连上游往返”，不称“纯推理”。不同直连拓扑仅作为独立网络实验。
2. G0：真实 gateway，cache off。
3. G1：真实 gateway，cache on，全 miss；记录真实 lookup 与后台写入。
4. G2：真实 gateway，cache on，固定低命中。先验证种子条目实际持久化，再按预定约 3%–5% 命中负载发起；记录实际命中率。G0 使用完全相同的问题/顺序。
5. H：预热 hit 单独实验，不加入 G0/G2 门槛对照。

不执行固定延迟 embedding stub：与本会话真实组件要求冲突，且更改 embedding 服务会改变向量、命中和写入行为。先用逐请求的真实 embedding span 分解贡献，再做资源隔离实验。

每个对照固定模型、prompt 集合/顺序、generation 参数、输出长度分布观测、Provider 配额、路由/重试、规则、调度开关、缓存参数与初始状态、transport/复用策略、环境和 build。D/G0/G1/G2 在多轮区组中交叉/随机平衡顺序，不同时运行以免互相抢同一模型资源。

起始诊断负载沿用 1/s 与此前稳定的 12.5/s；20/s 作为容量探测，而非默认验收负载。每个负载以实际达到速率且无持续发起积压为可比前提；压测器在途上限需覆盖测得的时延，并同时尊重每 Provider 额度。未达到目标时保留数据、标明瓶颈，不能降低实际负载后宣称目标通过。

现有用例只有单个 primary 配置、client cap 和45秒发起窗口；上述多场景、direct 与分段采样是后续测试入口的必要扩展，尚未实现。每次 test 保留内外层60秒硬限制，发起窗口缩短以给 warmup、尾部请求和清理留时间；不足的样本通过多个有界调用累积，不放宽超时。

### C. 按证据进入资源与网络实验

| 假设 | 真实实验 | 归因条件 |
| --- | --- | --- |
| chat 与 embedding 竞争 | 相同主模型/负载、相同 embedding 模型，比较共用服务与独立服务/资源；首先隔离进程，GPU共享与硬件隔离分别记录 | 同模型、同拓扑对照才用于判断竞争；用远程 GPT 替换 Qwen 同时改变多项变量，不能单独归因 |
| 后台写入竞争 | 相同前台全 miss，比较起始队列已排空与真实写入积压窗口，记录 worker embedding/存储和在途 | 必须控制制造积压时的主模型额外负载；无法控制时只报告相关性。不能设 WriteWorkers=0：现有配置禁止该值 |
| LAN/SSH/公网传输 | 同模型、同模型服务器、同负载，比较直连与转发路径，并记录连接复用/建连/TTFB | 无同拓扑对照或 Provider 端排队计时，不将等待首字节全部算成网络 |
| 网关CPU/锁/数据库 | 异常窗口短时 CPU、mutex/block、runtime trace，关联 SQLite/Redis/Qdrant spans | 分开采集 profile，校准扰动；不能用 CPU profile 证明所有 I/O 等待来源 |
| embedding容量/截止 | 真实单条输入与生产形态一致，单独读负载与读+真实写负载，记录并发、timeout/full/drop | 保留100ms候选预算直到证据支持调整；不通过加大超时隐藏可选缓存对主链的影响 |

独立 embedding 服务、Provider 端计时或替代网络路径若尚不可用，记录 unavailable 与后续准备事项，不假定已具备。额外压力请求不混入前台接受门槛样本。

### D. 多 Provider 网关容量与其他流程

先分别确认每个真实 Provider 的稳定负载范围；仅使用当前有资源且支持场景的 Provider。固定流量权重和每 Provider 在途上限，逐级增加总入口RPS，检查网关资源/数据库与路由准确性；按 Provider 分层结果，另报总吞吐。

网关容量实验先 cache off，再跑相同分布的 cache on 场景；embedding 仍可能是共享瓶颈。cache-on 对照通过独立配置/模型作用域控制选路，不能用 RouteOverride，因为该字段会绕过缓存。多 Provider 混合 P95 不替代单 Provider 同负载低命中门槛。

之后按矩阵覆盖 SSE、工具续接、buffered、scheduler on、fallback/Fusion；分别确认每个场景的业务 oracle，再做相应压力窗口，不将全部模型/请求类别合并为一个延迟指标。

## 6. 样本、分析与归属判定

最初每条件累计≥100请求用于探索；正式比较建议每条件≥500请求、至少3个平衡区组。接近5%边界或Provider波动大时继续积累到每条件≥1000并增加区组；P99低样本标为探索统计。这些是测试规模候选，不代表统计效力保证；远程低RPS通过更多60秒以内窗口累积。

保留每区组结果与同质总体统计，冷启动单独报告；按区组估计置信区间以避免把串行相关请求当独立样本。相同题目可配对分析差值，但不是同一次上游执行，也不能消除服务端随机性；P95门槛仍比较各条件分位数，不用“配对差值P95”偷换。

不相减不同请求/不同轮次的阶段P95来做耗时分解。逐请求计算同步关键路径/嵌套span的覆盖区间，避免double count；报告未覆盖时间，不命名为“纯CPU”。分别观察正常请求与尾部请求，不能只看各阶段平均值。

成功时延、全部尝试、超时/取消/错误分别报告。超时样本保留客户端截止时间并标记censored，不删除后宣称通过；全量样本和stream终止/结算证据可复算。

| 判定 | 所需证据与状态 |
| --- | --- |
| 网关/组件问题 | gateway同步阶段/队列或后台竞争随负载明显增长且可重复；更换同负载稳定上游后仍有同样开销，或真实控制实验复现 |
| 上游问题 | 同拓扑直连同模型也出现异常，gateway独占阶段稳定，最好有Provider队列/推理证据；需排除HTTP连接池等待、网关重试等本地原因 |
| 传输问题 | 同模型不同路径对照及建连/复用/网络证据支持；TTFB长本身不足以判断网络 |
| 无法归属 | 缺少关联、样本、上游时序或控制变量；标为inconclusive，不强行二选一 |

业务准确性、网关容量、上游容量与Phase29完整响应门槛分别签署。已证明仅上游受限时可记“已验证范围通过/上游待跟进”，不把未验证项计为pass；当前P95≤1.05、生产参数与原合并条件不自动放宽。

## 7. 交付与后续实现边界

每次执行输出环境与二进制manifest、原始client/stage JSONL或OTel span、load/queue/系统摘要、按场景/provider的分位数与区组置信区间、业务oracle结果、失败/超时列表、profile（如采集）、可重复命令及清理记录。统计脚本保留原始日志，汇总文件可重算。

后续实现首先集中在现有 observability、HTTP middleware/transport wrapper、组装入口及live harness；再补队列/终止等少量必要钩子。先列失败方式并写有价值的真实行为检查，不添加重复mock矩阵；实现后执行适用build/vet和60秒以内验证。生产配置/性能参数不因本计划改变。

## 官方参考

- [Go HTTP client trace hooks](https://pkg.go.dev/net/http/httptrace)：连接/首响应头字节，hook可能并发且重试多次触发；回调必须安全且不阻塞网络路径。
- [Go diagnostics](https://go.dev/doc/diagnostics)：profiling、runtime tracing和采集扰动。
- [OTel Go code instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/)：复用SDK/context创建分段span。
- [OTel Go zero-code方案对比](https://opentelemetry.io/docs/zero-code/go/choosing-auto-instrumentation/)：OBI的Linux/权限要求与otelc的编译期织入。
- [OTel Go compile-time instrumentation](https://opentelemetry.io/docs/zero-code/go/compile-time/)：基于规则和`-toolexec`的无源文件修改注入；具体项目兼容性仍需验证。
