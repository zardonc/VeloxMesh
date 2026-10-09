# Phase 29 扩大验证结果 — 2026-10-08

**扩大验证完成：全后端 613 项通过、2 项明确跳过；相关 race 83 项通过；
真实 Redis 7 项、真实缓存／网关 24 项通过；36 个性能窗口、3,600 次测量请求
全部成功。9 类场景满足本轮候选条件。原始 1.05 倍要求仍未达到。**

本轮未发现新的应用实现缺陷。上一轮的 Redis 同键发布等待修复通过了更广的
并发、默认超时、状态恢复与真实网关验证。本结论仅覆盖下述路径和负载，
不表示应用绝无缺陷，也不是 Phase 29 全部生产发布条件已经完成。

## 继承证据及本轮范围

- 已读取引用聊天 `hi`，继承 Formal07 的旧程序正式证据和上一轮 RED/GREEN。
  未重复旧的 144 窗口／43,200 请求；没有将旧结果归到本轮程序。
- 上一轮确认并修复：有序 Redis 客户端在网络响应期间仍持有同键发布令牌，
  导致后续请求排队。真实 Redis 故障对照的后续等待从199.426降至1.312 ms。
- 本轮开始时核对410份当前源码哈希；新增扩展测试后冻结411份源码。
  后续两个测试修正分别保留新的源码和二进制清单，原清单和失败记录保留。
- 产品逻辑仍只有上一轮的 `internal/health/redis_store.go` 修改。本轮新增3项
  真Redis回归，修正2处已有测试；未修改生产中间件、模型、超时或缓存开关。
- 入场检查确认隔离容器已停止、无任务后端、无已加载模型。继续使用现有
  CPU4嵌入、Qdrant `no_populate`、本地Qwen，不调用付费上游。

## 应用正确性验证

| 层次 | 本轮证据与结果 |
| --- | --- |
| 全后端 | `backend-01/full-backend.jsonl`：613个顶层PASS、0 FAIL、2 SKIP；真实依赖已接通 |
| Race | `affected-race.log`：health/hotstate/cache/gateway共83个顶层PASS、0 FAIL、0 SKIP |
| 静态／构建 | 应用tagged vet、每次测试修改后的vet、Linux测试构建、派生runner vet/build及计划自检通过 |
| Redis | 7项全部PASS：默认发布期限、模型迟到发布、探针并发发布、健康读取40/60/100ms矩阵、多路由故障分类、健康peer隔离、C8/200次计数更新 |
| 缓存／网关 | 24项唯一测试最终全部PASS，原失败保留；详见 `functional-summary.json` |

所有后端测试具有Go内部60秒限制与外层60秒进程限制（Windows终止进程树，
远端使用 `timeout --signal=KILL 60s`）。容器启动与多项测试控制器独立计时。

新增默认期限测试不设置SyncTimeout，直接使用生产默认50ms：延迟首个Redis响应200ms，首调用
在 **51.134ms** 返回且记录 `context deadline exceeded`，后续同provider发布
仅 **2.414ms**。恢复后远端pending=0、successes=2，本地计数一致。
模型迟到写入保留successes=2/failures=1；并发探针保留最后成功状态和同样计数。

24项真实缓存／网关检查覆盖：健康依赖错误和恢复、主模型返回后的Redis延迟、
未命中向量复用、嵌入／向量搜索网络期限、memo容量/TTL/账户与模型隔离/故障恢复/
计费、缓存重放/重启/过期清理、部分向量写入恢复、pending不可见、精确匹配策略/
安全回归/版本隔离、禁用与省略策略、scope readiness、Qdrant并发建集合及维度
拒绝、provider并发与取消恢复、鉴权、流式结算。PASS不等于独立语义召回合格。

2项跳过为 `TestPlan4PostgresSmoke` 和
`TestPlan4PostgresSansPrimaryRealProviderSmoke`，其显式PLAN4密钥／开发身份或
SANS真实provider变量未提供给本轮后端运行；未记为通过。其余数据库用例已执行。

## 扩展中发现的测试问题

1. `TestLiveCacheMissEmbeddingReuse` 对空scope仍要求读取阶段嵌入。
   `functional-01`原始记录实际为`lookup/scope_not_ready`，写入阶段仅一次嵌入；
   没有重复嵌入。测试现在先建一个同scope候选，再测未命中的读取向量复用，
   并将两次真实主模型请求的结算期望设为2。`reuse-green-01`单项PASS。
2. `TestLiveEmbeddingMemoFailureRecovery` 写死主模型端口11234；独立嵌入配置
   使用11235，因此`functional-02`返回`provider_invalid_model`。现在复用既有
   `liveDeadlineProxy(t, "embedding")`，按实际嵌入地址建代理。
   `memo-green-01`单项PASS，恢复后完成剩余17项，未重跑已通过用例。

两项都只修正测试前提／路由，不放松错误、计费、向量复用或缓存安全断言。
原测试源码、原失败日志、当时测试程序及修正后的证据分别保留。

## 性能设计与结果

预先声明3区组×12窗口×100请求，固定125ms到达间隔，并发上限4、memo关闭。
纯未命中、固定4%命中和同问题场景各有匹配的前后off基线。每个窗口验证实际
缓存配置、成功数、命中位置、响应哈希、计费及阶段记录，不替换失败/慢窗口。

| 场景 | 请求数／命中 | 完整响应P50/P95/P99 ms | 应用剩余P95/P99 ms |
| --- | ---: | --- | --- |
| pure/off | 600／0 | 80.456 / 108.727 / 130.361 | 3.724 / 4.970 |
| 语义纯未命中 | 300／0 | 96.172 / 119.954 / 150.943 | 3.961 / 4.647 |
| 精确未命中 | 300／0 | 79.504 / 100.372 / 130.160 | 3.475 / 4.078 |
| low-hit/off | 600／0 | 80.937 / 101.881 / 132.604 | 3.604 / 4.168 |
| 语义4%命中 | 300／12 | 96.489 / 117.114 / 147.017 | 4.259 / 4.813 |
| same-question/off | 600／0 | 70.781 / 81.134 / 98.102 | 3.865 / 4.409 |
| 语义同问题命中 | 300／300 | 13.666 / 16.772 / 18.196 | 0.751 / 1.092 |
| 精确命中 | 300／300 | 1.165 / 1.694 / 1.993 | 0.748 / 1.057 |
| system不匹配旁路 | 300／0 | 75.169 / 81.976 / 194.519 | 3.841 / 4.461 |

实际各场景8.018–8.079 RPS，峰值并发1–4。应用剩余耗时是逐请求
`http_handler - provider_complete - cache_read`，不等于纯CPU时间。

本轮候选条件：语义未命中/低命中P95同时满足≤两个off各自1.25倍、增量≤40ms；
精确未命中增量≤10ms；命中P95≤60ms且≤两个off各自0.5倍；应用剩余
P95≤10ms、P99≤15ms。**9类场景汇总及复用原公式的95%配对区组bootstrap
区间均通过；逐区组点值也无候选超限。** 原始逐请求审计0失败client/operation/transport。

语义纯未命中相对前/后off为 **1.097／1.191**，增量 **10.628／19.211ms**；
低命中为 **1.153／1.150**，增量 **15.555／15.233ms**。两者均未通过原1.05。
纯未命中embedding HTTP P95=14.628ms、cache read=17.042ms；低命中分别
14.491/16.937ms。单独相加各阶段P95不能作为总P95的因果分解。

最慢请求391.405ms中provider_complete=387.879ms、网关handler=391.120ms，
健康同步仅0.498/0.415ms；另一366.178ms请求的provider_complete=362.509ms。
这些长尾在上游HTTP等待阶段，不支持把它们归到本轮已修复的发布队列。
尚未进一步区分上游运行时调度、服务排队和转发网络的贡献。

## 证据边界与资源

- 这不是12区组Formal07重放。3区组的置信区间和短期稳定性证据有限，
  没有遍历全部6种顺序；off区组漂移范围 **−11.31%至+5.18%**。
  同时报告前后端点，未选择较慢端点宣告1.05合格。
- 本轮native实际为Normal：启动代码显式选择，运行中Windows API读回32。
  原source-manifest中的backend描述误继承“AboveNormal”，但同清单的
  effective_load_settings和当前启动证据写明Normal。保留原清单，另存
  `runtime-metadata-correction.json`，并修正后续生成器；没有改运行设置或原始数据。
  原启动器的“三窗口”提示同样来自复用旧函数；真实计划和执行为36窗口。
- Nomic/Qwen权重本轮均重新核对SHA256；当前模型身份、context8192、parallel4
  和native命令保留。主模型runtime 2.54.0为此前记录，本轮未单独重新枚举其
  已加载DLL，不把这一继承描述作为精确重放的证据。
- 558个主机样本：CPU均值26.89%、峰值57.73%，可用内存最低8.83GiB。
  VM最低idle27%、最大I/O wait3%、1个swap-in非零样本、0个swap-out样本。
  这些为整个控制器期间记录，含启动；跨主机时钟未校准，不能逐请求归因。
  没有证据确认硬件物理限制，因此未以硬件极限为由改软件或降低门槛。

## 最终状态与遗留事项

确认：上一轮应用修复通过更广验证；本轮2项测试问题已修正；当前配置在这次
扩大验证中满足候选预算。未新增生产配置调整，未提交、合并、部署或启用缓存。

仍未关闭：原1.05、独立语义质量/种子事实正确性、GPU方案资格、历史primary EOF、
长期/更高负载容量、集合容器级回收和生产发布决策。具体数据和向量清理测试
通过，但不意味着空集合也会回收；本轮性能集合数从176增至185，全部保留。

清理：模型列表为空，native端口释放，控制器和远端测试进程退出；17个隔离
容器停止、17个卷及历史证据保留。没有为了绿色结果清空数据或删除失败记录。

## 复核入口

- `PLAN.md`、`source-manifest.json`、`source-manifest-02.json`、`source-manifest-03.json`
- `functional-summary.json` 和其链接的原始PASS/FAIL日志
- Redis原始证据：`../phase29-followup-20261008/comprehensive-health-01/`
- `performance-01/test-plan.json`、`analysis.json`、`analysis-summary.json`
- `performance-01/raw-audit.json`、`resource-audit.json`、`runtime-metadata-correction.json`
- `remote-final.jsonl`、`completion-audit.json`
- 复测入口 `.tmp/phase29-comprehensive-20261008/run.py`，必须提供新标签；
  当时执行的脚本副本保留在performance目录，原目录不可覆盖。
