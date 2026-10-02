---
phase: 29-semantic-cache-latency-hardening
verified: 2026-10-01T20:48:00-07:00
status: partial
functional_status: passed
acceptance_scope: local-model-functional
source_revision: 90dfd695156d7f78d8f3cb7ee63b319502ee3a15
production_release_approved: false
---

# Phase 29 验收报告

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
