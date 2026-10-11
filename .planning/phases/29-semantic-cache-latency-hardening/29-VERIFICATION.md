---
phase: 29-semantic-cache-latency-hardening
verified: 2026-10-01T20:48:00-07:00
status: partial
functional_status: passed
acceptance_scope: local-model-functional
source_revision: 90dfd695156d7f78d8f3cb7ee63b319502ee3a15
latest_verified: 2026-10-08
latest_source_revision: 214422cb4ca0d40e455ce3dd7468323dbd092005
latest_source_uncommitted: false
latest_acceptance_scope: candidate-budget-code-integration
integration_status: accepted_candidate_budget
production_release_approved: false
---

# Phase 29 验收报告

## 当前合入结论 — 2026-10-08

用户已明确采用候选预算推进当前分支全部提交合入，并要求保留规划历史、生产
缓存关闭及空白名单。应用正确性和本轮候选性能证据支持提交 PR；`status: partial`
保留生产范围。以下历史验收的门槛和失败记录不改写为通过。

| 检查 | 当前结果及边界 |
| --- | --- |
| 源码身份 | `214422c` 与 [最终清单](../../debug/phase29-comprehensive-20261008/source-manifest-03.json) 的411份源码全部一致；本次仅更新文档 |
| 全后端／race | 613顶层PASS、2显式SKIP、0 FAIL；相关race 83 PASS；跳过的两项为PLAN4 opt-in smoke |
| 真实组件 | Redis 7 PASS；缓存／网关24项唯一PASS，包含默认期限、迟到写入、恢复、计费与隔离；原两项测试失败和修正证据保留 |
| 当前性能 | 3区组、36窗口、3,600成功请求；约8 RPS、并发上限4、memo关闭；9场景点值及配对bootstrap区间通过候选预算 |
| 原1.05 | 仍失败：语义纯miss比值1.097/1.191，低命中1.153/1.150；未证实为硬件物理极限 |

采用的合入预算：语义miss/低命中P95≤每个前后off端点的1.25倍且增量≤40ms；
exact miss增量≤10ms；命中P95≤60ms且≤每个off的0.5倍；逐请求应用剩余耗时
P95≤10ms、P99≤15ms。语义miss/低命中P95=119.954/117.114ms，semantic/exact
hit P95=16.772/1.694ms。应用剩余耗时为handler减provider_complete及cache_read，
不等同于CPU时间。

最新应用修复移除有序Redis写入器的多余同键网络串行等待，保留Lua旧版本拒绝、
错误及默认50ms期限。后续同provider发布RED/GREEN=199.426→1.312ms；默认
期限扩展实测首调用51.134ms、后续2.414ms，计数和恢复正确。本轮扩大验证没有
发现新的应用实现缺陷，不代表应用绝无缺陷。

完整证据见 [扩大验证报告](../../debug/phase29-comprehensive-20261008/REPORT.md)
及 [完成审计](../../debug/phase29-comprehensive-20261008/completion-audit.json)。
旧Formal07为修复前程序的144窗口/43,200请求，不能作为当前程序重测。
本轮只有3区组，off漂移−11.31%至+5.18%，native优先级实际为Normal；修正
记录已保留。主模型runtime版本描述继承旧记录，未独立重新枚举DLL。

2026-10-03已记录两款本地embedding的完整生命周期；不等于所有在线模型、
独立业务语义质量或GPU替代方案合格。生产仍待：原1.05目标、语义质量/种子事实、
空集合回收、历史primary EOF、长期/更高负载/冷启动容量及生产启用决策。
测试后17容器停止、17卷及历史证据保留；未修改生产配置、部署或启用缓存。

## 历史验收记录

**2026-10-04 最新增量：** [进一步排查与局部修正](29-FURTHER-CORRECTION-20261004.md) 已修复空/待激活 scope 的不存在集合查询及 Gemini 终态后错误接受。599 个后端顶层 PASS、0 FAIL、2 个明确 SKIP，真实 SQLite/Qdrant、PostgreSQL 与 Gemini 边界通过。exact miss 的零 embedding 与成本得到单块验证；semantic 仍有约 21ms 前台读取成本且基线漂移。持久集合回收、业务答案安全及稳定性能合同保持开放，状态保持 partial。该增量不覆盖或替代文末各历史验收范围。

**恢复与集合排查（2026-10-04）：** [最新诊断](29-RECOVERY-INVESTIGATION-20261004.md)。120s 恢复门槛失败，正式暖机对照未运行，不计为性能 PASS。七个组件隔离条件完成；Qdrant 无请求启动即触发交换/I/O，停止卷显示 63.80GiB 稀疏逻辑长度与 212.27MiB 实际分配，collection 名称保持 347。点级过期清理未关闭集合生命周期，仍需同点数/不同集合数对照和 primary engine 时序；原 P95 与业务语义安全结论保持开放。

**内存升级后复测（2026-10-04）：** [完整对照结果](29-MEMORY-UPGRADE-RESULTS-20261004.md)。VM 实测 4 vCPU/7376MiB；六块低命中及六块纯 miss 共 4800 正式请求、69 选定真实测试调用全部成功，全量后端 595 顶层 PASS、0 FAIL。Residual P95=4.92–5.50ms，候选 AND 点估计通过，原 1.05 仍失败；四组基线漂移、正式时段 swap-in 和大量小集合仍需排查。347 集合已只读清点，冷态 NotFound 与业务安全未关闭，`partial` 保留。

**最新继续排查（2026-10-04）：** [可用性与尾延迟调查](29-AVAILABILITY-INVESTIGATION-20261004.md)。Redis 健康状态读取失败现在明确返回 `health_state_unavailable`，维持 fail-closed；真实五路、恢复、健康 peer 与协议回归通过。全量后端 595 顶层测试通过，0 失败。新单窗口 residual P95=4.39/4.64ms、原比值=1.078，候选仅点估计通过；启动换页与慢 Redis 命令不足以归因历史故障，160 次直连未复现上游 400。性能、自然依赖停顿、业务语义与冷态证据仍开放；下文保持历史范围，`status: partial` 不变。

**最新执行（2026-10-04）：** [策略与保护修正结果](29-POLICY-EXECUTION-RESULTS-20261004.md)。最终全量后端、真实六项 Provider 保护、SSE/取消/缓冲/Fusion 通过；完整同题 exact 命中、41 个变题旁路、版本隔离通过。性能候选合同、本轮 Redis 健康快照可用性、业务语义质量与真正冷态仍开放，`status: partial` 保留。本报告下文为历史验收，不替代最新证据。

**后续验证（2026-10-03）：** [分场景真实验证结果](29-SCENARIO-VALIDATION-RESULTS-20261003.md)补齐两款embedding模型生命周期、实际C4/C8并发及故障恢复。正常网关处理通过本轮诊断预算；原1.05性能门槛、扩展语义召回、部分写入残留及自然尖峰仍有未解决项，未合并main。下文保留2026-10-01初验的历史范围与结论。

**本轮本地模型功能验收通过，未发现系统功能性失败。** `status: partial` 表示原性能、在线双模型及生产发布证据尚未全部完成，不表示本次功能测试失败。

## 本次验收范围

用户明确允许在线 embedding 额度不足时以本地模型完成验证，非系统功能性异常只记录。该调整应用于本次验收；在线双模型与压测不执行，也不写成通过。另已明确授权向既有隔离机 `192.168.234.129` 传入测试凭据、临时启动既有测试容器及 SSH 转发，结束后恢复状态。

本轮未修改产品实现、生产配置、生产白名单或账户额度。新增内容为验收记录、脱敏日志和可复用的测试传输辅助程序。没有创建提交或部署。

## 实测环境与来源

代码 HEAD：`90dfd695156d7f78d8f3cb7ee63b319502ee3a15`。工作区在开始时干净。Linux 验收 binary 从本次工作区用 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags phase29preflight ./internal/app` 构建，SHA-256 为 `0793cdea12f777246b42519bfddc2aff6cd301667ab0701ef686fd049745b4d4`。

本地模型：从已运行服务的 inventory 确认并手动选择 `text-embedding-embedder_collection`；真实 adapter 探测得到 768 维。另一款 inventory 模型没有被选择或计作验收。本机模型服务通过 loopback SSH reverse forwarding 被隔离机访问；主模型仍为已有配置的 `oc/big-pickle`。没有在线 embedding 调用或持续负载实验。

真实功能用例运行于隔离机：`App.New()`、认证 HTTP chat handler、正常 gateway、独立磁盘 SQLite 数据库及实际 Qdrant adapter。测试创建非管理员 API key，仅将其实际 ID 加入测试 profile。跨版本测试复用相同 key/model/database。全量后端测试在 Windows 执行，通过 loopback SSH 转发访问隔离 Redis、PostgreSQL 和 Qdrant。

隔离测试参数显式传入：read=100 ms、read concurrency=4、write timeout=2 s、workers=2、queue=32、shutdown grace=1 s。这些是既有实验配置，不是本轮批准的生产默认值；模型、维度及凭据未硬编码进产品实现。

## 本次测试结果

| 检查 | 实际结果 | 日志 |
| --- | --- | --- |
| 全量 `go test -count=1 -timeout 60s -json ./...` | 592 个顶层测试通过，0 失败，2 跳过；38 个有测试的 package 通过，另 4 个无测试 | `full-backend.jsonl` |
| cache/gateway/config/Gemini/OpenAI/metrics 离线检查 | 124 个顶层测试通过，0 失败，0 跳过；为全量结果的子集，不重复累计 | `offline.jsonl` |
| cache/gateway focused race | 22 个顶层测试通过，无 race 报告 | `race.jsonl` |
| HTTP 缓存回归 | 2 个测试通过：响应不等待慢写入、开发 key 绕过 | `cache-http.jsonl` |
| 本地模型网关完整流程 | pass，6.64 s：首次 miss、异步落库、改写 1/1 hit、相同答案哈希、负例 2/2 miss、版本切换 miss | `TestPhase29LocalGatewayAcceptance.log` |
| 本地模型 embedding 故障 | pass，2.76 s：探测后代理注入 503，HTTP 200、hit=false、lookup/store embedding_error；完整响应 2682.715 ms | `TestPhase29LocalEmbeddingFault.log` |
| 本地真实队列及关闭 | pass，0.66 s：128 candidates，34 accepted、94 dropped-newest；enqueue 0.040 ms；close 588.635 ms；34 stored，关闭后拒绝 | `TestPhase29LocalQueueBurst.log` |
| `go vet -tags phase29preflight`（涉及的 8 个 package）及 tagged Linux 编译 | exit 0 | 命令执行结果；binary hash 如上 |

所有后端测试均设置 60 秒 Go timeout；全量、focused 和 remote runner 另设 60 秒外层限制。完整集成包本轮用时 15.852 s。全部本地模型验收是真实执行，未通过 skipped 测试或旧日志代替。

默认关闭、可信白名单、工具/流式绕过、配置边界、错误向量/答案拒绝、TTL、租户/provider/model/version 隔离、零 Usage、读截止、并发饱和、异步快照及限期关闭由现有离线/HTTP测试覆盖。真实模型语义正确性覆盖一个正例和两个负例，不能推广为全部业务问题均安全。真实 503 测试覆盖 embedding 故障；其他依赖故障和强制 shutdown cancellation 主要由离线故障注入覆盖。

## 仅记录的异常与延期项

| 项目 | 分类 | 处理及限制 |
| --- | --- | --- |
| 在线 embedding 日额度/429 | 既有外部限制 | 本轮不调用在线 embedding、不购买额度；原双模型完整生命周期延期。以本地模型替代本次功能验收，不等于验证了两款在线模型 |
| 低命中延迟及持续容量 | 既有性能问题 | 不重跑压测。此前修正实现的 P95 比值 1.143 / 1.511 大于 1.05，仍是未达标记录；主模型超时和不同实际吞吐使缓存增量归因不充分。不能标为通过或用于批准最终生产参数 |
| `TestPlan4PostgresSmoke` | 非本期 opt-in 跳过 | 未提供专用 `PLAN4_*` 输入，按既有测试约定跳过；已运行的 PostgreSQL、Qdrant、Redis 和 Phase 29 测试均通过 |
| `TestPlan4PostgresSansPrimaryRealProviderSmoke` | 非本期 opt-in 跳过 | 全量套件显式清空 SANS 输入以避免无关在线调用；单独 Phase 29 真实功能用例仍使用授权的主模型配置 |
| 初始 uv cache/paramiko 与 Go cache权限 | 本机工具环境 | uv 默认 cache 无写权限，备用 cache 中无 paramiko；改用项目现有 Go SSH dependency，并将新辅助程序的 build cache 放在临时目录。未改产品依赖或安装新包 |
| 初次 SSH known_hosts 访问及自动审批拒绝 | 访问/授权流程 | 沙箱账户不能读取已有 known_hosts；首次提权被拒绝，原因是未明确授权向隔离目的地传输凭据和启动依赖。暂停该操作，获得用户明确授权后才重试成功；SSH 使用已有 known_hosts 校验，未跳过 host-key 验证 |
| 生产发布 owner、原子切换、部署停止期限及参数批准 | 本次范围外 | 本轮不执行生产发布。当前隔离功能结果不代表生产原子切换程序或持续容量已验证 |

这些项不阻塞本次本地模型功能验收，但不能被描述为原发布门槛全部关闭。原 `CACHE-F01` 全部发布证据和 Phase 29 自动完成状态未被强行修改。

## 复现与证据

所有本轮日志位于 `measurements/acceptance-20261001/`。原始历史测量保留于 `29-LOCAL-MODEL-MEASUREMENT.md` 及其 JSONL，不覆盖或混入本轮测试统计。已用现有环境中的 key/password/secret/token 值逐个扫描本轮 JSONL/log，未发现凭据值；不会在扫描输出中回显这些值。

离线检查可直接复现：

```bash
go test -count=1 -timeout 60s ./internal/cache ./internal/gateway ./internal/config ./internal/providers/gemini ./internal/providers/openai ./internal/observability
go test -count=1 -timeout 60s -race ./internal/cache ./internal/gateway -run 'TestSemanticCache|Test.*Cache'
go test -count=1 -timeout 60s ./tests/integration -run '^TestSemanticCache_'
go vet -tags phase29preflight ./internal/cache ./internal/gateway ./internal/config ./internal/providers/gemini ./internal/providers/openai ./internal/observability ./internal/app ./tests/integration
```

全量与真实流程需要上述已授权隔离环境和现有凭据；`measurements/acceptance-20261001/runner/main.go` 从 `.env`/`.env.local` 读取配置，并在内存中处理、通过 SSH stdin 传入测试进程。日志保存前替换凭据，不将配置文件复制到 git。辅助程序只对既有测试容器执行 inspect/start/stop，保留 volumes；凭据不作为 shell 参数。

可选真实测试入口是 `TestPhase29LocalGatewayAcceptance`、`TestPhase29LocalEmbeddingFault` 和 `TestPhase29LocalQueueBurst`，分别用 `-test.run '^<name>$' -test.timeout 60s -test.v` 执行。runner 的模型、endpoint 和六项 bounds 来自运行时环境，缺少输入时不以产品默认值兜底。重新运行必须先构建文首 Linux binary，并保存到 runner 预期的 `app-linux.test` 位置。

## 清理与结论

本轮启动前三个既有测试容器均停止；结束后实际输出确认 `veloxmesh-test-redis false`、`veloxmesh-test-qdrant false`、`veloxmesh-test-postgres false`。转发 listener 和 SSH client 已关闭，本机已有 embedding 服务继续保留。未删除测试 volumes。本机生成的传输可执行文件与 Linux binary 在记录 hash 后清理；可从保留的源码重新构建。隔离机 `/tmp/veloxmesh-phase29-acceptance-20261001.test` 中无凭据的测试 binary 保留，未保留测试配置或凭据文件。

**验收结论：按照用户调整后的本地模型功能范围，通过。** 本轮不存在需要修复的新增功能性问题；在线双模型、持续压测、既有 P95 未达标及生产发布前置事项仅保留记录。尚未获得生产缓存启用许可。

## 2026-10-04 硬件优先续查

后续调查见 [硬件优先排查与局部修复](29-CONTINUED-INVESTIGATION-20261004.md)，证据位于 `measurements/continued-investigation-20261004/`。新建隔离空卷、保留历史资源后，两组有效 8 RPS 窗口共 1,000/1,000 请求成功，未见持续 CPU、GPU、内存或 I/O 饱和；纯 miss 的 on/memo 比值仍为 1.134–1.208，超过原 1.05 门槛。该证据支持先排查串行 embedding/查询成本，不支持先扩容；1 RPS 补测存在宿主机 CPU 尖峰和基线漂移，仅作诊断。

本轮还复现并修复 Gemini 非终态流退出后的误结算，取消、终态前中断、正常完成三项真实回归均通过。首次全量暴露既有 Redis PubSub 测试的异步发布竞态，修复测试夹具后，最终全量后端 595 个顶层 PASS、0 FAIL、2 个显式 opt-in SKIP，内外均有 60 秒上限。失败日志和夹具参数错误保留，未覆盖历史结果。

网关 Gemini 响应头等待超时、FAQ 种子事实错误、首次 scope 的 Collection NotFound、集合生命周期与原语义/容量发布合同仍未关闭。Phase 29 保持 partial，生产缓存保持关闭；本轮测试容器、转发和观察进程均已停止，历史卷与本轮诊断卷保留。
