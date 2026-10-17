# Phase 30: Request Budget and Safe Failover - Context

**Gathered:** 2026-10-17
**Status:** Planned — 8 plans passed independent review; implementation not started
**Source:** 当前源码与留存验收核对；用户选择请求总预算与安全故障转移，并明确采用保守策略。

<domain>
## Phase Boundary

补齐 TIMEOUT-F01：复用现有单次 Provider 分阶段保护与有限跨 Provider fallback，增加覆盖整个请求的总预算，补齐连接阶段限制、SDK 隐式重试审计与安全切换资格。目标是慢上游和多次尝试不能叠加耗尽资源，也不因不确定失败自动重复执行或计费。

本轮只更新报告与规划，不执行产品开发、部署或在线测试。Phase 29 代码已随 PR #31 合入，但生产门槛继续 partial；生产缓存保持关闭、白名单为空。
</domain>

<decisions>
## Implementation Decisions

### 用户本轮已确定

- **D-01:** 下一阶段优先请求总预算与安全故障转移；Console 和完整模型/账号能力目录后续推进。
- **D-02:** 首版采用保守策略：只对明确安全的失败切换；请求已发出且执行状态不明的超时直接结束。不能仅因未收到正文就推断未执行。开始输出后不重放。

### 继承既有合同与本阶段范围

- **D-03:** Phase 27 保留唯一终态和 Usage 结算所有权：第一终态获胜，取消/错误不扣费，completed 先获胜才按既有合同结算一次；已知 Usage 保留诊断，不遗留 attempt/admission 许可。Phase 28 工具协议和 Fusion 工具限制保留。工具请求及带工具结果历史的请求不得因不确定执行而重放。
- **D-04:** 新增请求预算覆盖入口到非流式响应完成或流转发结束，包括缓存、路由、admission 等待、全部 Provider 尝试，以及同步结算、Usage 日志和成本聚合的有界终结。更早的客户端 deadline 优先，子阶段只能收紧剩余预算；耗尽后不启动新尝试。缓存独立截止可以旁路，整请求截止必须结束请求。不得通过丢弃已知 Usage、无界后台调用或新增持久队列绕过截止；持久化失败必须可见。
- **D-05:** 新增预算/连接配置保持可选，未配置时保留现有超时默认值；保守重放规则是安全行为收紧，需在文档和兼容验证中明确。配置需在边界校验，不硬编码生产值，不升级依赖。
- **D-06:** 首版复用既有有限跨 Provider fallback，约束最大尝试次数与剩余时间；不新增同 Provider 重试、退避队列或 hedging。检查锁定 SDK/transport 默认重试，避免网关限额之外的隐式放大；实际调用次数必须可验证。
- **D-07:** 复用现有低基数原因/计时钩子记录预算耗尽、attempt 数与切换/拒绝原因，不记录正文、工具参数、凭据或用请求 ID 作 metrics 标签。先提供阶段验收证据，不扩展完整请求诊断平台。
- **D-08:** 先写失败场景和有业务信号的回归，再改代码。优先真实本地 HTTP/SSE 夹具；开发中只跑聚焦检查，完整 E2E 在阶段末执行。后端测试有硬60秒外层限制和 `go test -timeout 60s`；每个命名用例必须有匹配 PASS，不能把无测试/跳过算通过。

### Agent Discretion

- 复用现有保护层、错误类别与 config loader，选择最小配置名、预算载体和发送进度表达；无需引入通用策略框架。
- DNS/TCP/TLS 能用标准库已有 context/transport 限制解决时直接复用；SDK 扩展点以锁定版本源码为准，执行时再核对官方文档。
- 精确失败分类由源码和受控传输实验证明，不能把“所有5xx”“所有超时”或“没有收到token”统称安全切换。
- 计时测试使用可控同步、调用计数和合理调度容差，避免以严格墙钟等值制造不稳定测试。
</decisions>

<canonical_refs>
## Canonical References

以下均为完整仓库相对路径；报告路径经目录联接指向外部 Notes-sur-l-IA 仓库，不能假定它随 VeloxMesh 分支提交。

- `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md` — 2026-10-17 更新，第4/11节为完成核对与下一阶段任务。
- `.planning/PROJECT.md`、`.planning/REQUIREMENTS.md`、`.planning/ROADMAP.md` — 网关边界、TIMEOUT-F01 与 Phase 30。
- `.planning/phases/27-stream-terminal-settlement-consistency/27-CONTEXT.md` — 终态、取消、结算与许可所有权。
- `.planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md` — 工具协议、能力过滤与 Fusion 边界。
- `.planning/phases/29-semantic-cache-latency-hardening/29-CONTEXT.md`、`.planning/phases/29-semantic-cache-latency-hardening/29-VERIFICATION.md` — 既有阶段保护与生产限制。
- `.planning/REVIEW-20261008.md`、`.planning/REVIEW-EVIDENCE-20261008.json` — 留存验收和候选合入预算；不是本轮重测。
- `internal/providers/protection.go`、`internal/providers/protection_attempt.go`、`internal/providers/protection_stream.go`、`internal/providers/protected_adapter.go` — 单次保护和共享并发资源。
- `internal/gateway/service.go`、`internal/gateway/service_stream.go`、`internal/gateway/fusion.go`、`internal/gateway/stream_terminal.go` — 尝试循环与终态。
- `internal/gateway/service_settlement.go` — 现有同步 `context.Background()` 结算/日志/聚合边界；纳入有界终结并保留 Phase 27 所有权。
- `internal/errors/errors.go`、`internal/config/provider_protection.go`、`internal/controlstate/runtime.go` — 错误分类、配置与运行时注入。
- `internal/http/handlers/chat.go`、`internal/http/handlers/chat_stream.go`、`internal/observability/stages.go` — 请求边界、实际输出与取样钩子。
- `go.mod`、`go.sum` — SDK 锁定版本；本阶段不升级依赖。

本阶段没有新增外部规范锁定；SDK 行为必须从锁定源码与官方文档确认，不能引用未经核验的默认重试次数。
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `providers/protection_attempt.go:20` 每次 begin 重置 policy.Total，并有 first-byte/first-content 保护；`protection_stream.go` 已有 idle/取消/许可释放。请求总预算应在它们之外建立，再向内传播。
- `gateway/service.go:99` 已有最大次数和跨 Provider fallback；流式只在获取事件 channel 前处理启动失败，事件消费之后不重试。
- `errors/errors.go:125` 旧错误类别可触发 fallback；新阶段超时类别默认不可重试。需要明确发送进度，而非简单把新超时全加入可重试名单。
- `controlstate/runtime.go` 已接入保护，资源池跨 registry 重建保留；保留此并发边界。

### Established Patterns

- 使用 context 传递截止，复用标准库 transport、既有 DI 和不可变请求/结果；不新增同步控制面或逐片段外部 I/O。
- `observability.Stage` 和缓存操作观察可提供受控样本；普通生产 metrics 尚非完整细分 Stage 系统。
- 本地真实传输测试优先，已存在 `internal/app/live_provider_protection_test.go` 与 `live_provider_lifecycle_protection_test.go` 可参考覆盖意图，不把未执行测试描述为当前通过。

### Integration Points

入口 context → cache/routing/admission → 普通/stream/buffered/Fusion → protected Adapter → Phase 27 finalizer。跨 Provider fallback 和 SDK 内部尝试均必须接受同一剩余预算约束。
</code_context>

<specifics>
## Delivery Outline

T1 请求总预算纵向闭环 → T2 连接/SDK边界与 T3 保守故障转移 → T4 阶段末完整验收。详细计划按可验证的纵向切片拆分，先有最薄 HTTP 请求闭环，再扩展流式与多 Provider，最后产出脱敏重放证据。预计4–7工程日，另计评审等待。

验收至少覆盖：配置缺省/非法、客户端更早deadline、cache旁路与总预算区别、排队超时、耗尽前后fallback、连接/首内容/idle、未知执行状态拒绝重放、工具请求、SDK调用计数、取消竞态、一次终态/结算与许可回收。正常路径应验证不增加同步外部 I/O 和无配置时的兼容性。
</specifics>

<deferred>
## Deferred Ideas

- Phase 29 原1.05、独立语义安全/种子事实、集合回收、长期/高负载/冷启动容量和生产启用批准。
- 真实主动健康探测：当前 OpenAI 仅检查配置，Anthropic/Gemini 仍为占位成功；TestConnection 不可视为真实连通。它属于后续控制面诊断，不混入本阶段。
- Console、完整 TTFT/TTLT 指标及请求查询；模型/账号能力覆写、结构化输出能力、Adapter Kit、新协议端点、多模态和 MCP。
- 同 Provider 自动重试、Retry-After/退避调度、hedging、Agent 编排与工具代执行。
</deferred>

*Phase: 30-request-budget-and-safe-failover*
*Context gathered: 2026-10-17*
