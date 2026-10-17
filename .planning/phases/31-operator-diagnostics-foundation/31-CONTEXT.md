# Phase 31: Operator Diagnostics Foundation - Context

**Gathered:** 2026-10-17
**Status:** Ready for planning; execution blocked on Phase 30 implementation/verification
**Source:** 用户本轮两项选择、2026-10-17 调研报告与当前源码静态核对。

<domain>
## Phase Boundary

为后续 Console 提供可信诊断：真实有界探测、准确网关 TTFT/TTLT、请求 ID 关联和脱敏元数据查询。复用现有 Go/Chi Admin API、Provider adapters、健康状态与观测钩子；不开发界面。

本轮只写规划。Phase 30 的8份计划尚未实施，执行顺序必须是30验收后再31；不能把未来的请求预算、SDK控制或测试runner当作当前已存在/通过。Phase 29 生产门槛独立开放，生产缓存继续关闭且白名单为空。
</domain>

<decisions>
## Implementation Decisions

### 用户本轮已确定

- **D-01:** Phase 31 做 Console 前置诊断：真实主动探测、准确 TTFT/TTLT、按请求 ID 查询脱敏诊断；Console UI 随后推进，完整模型/账号能力目录另行规划。
- **D-02:** 后台只做有界非生成探测，结果只表示实际验证的连通/鉴权能力；生成能力由管理员显式手动测试。不得把非生成成功当成模型生成可用，后台不得偷偷回退到计费生成请求。

### 继承合同与本阶段最小实现边界

- **D-03:** 分开记录配置有效性、连通/鉴权、手动生成与真实流量健康，附探测类型/时间/有限原因。轻探测成功不得清空真实请求失败或恢复生成健康；不支持轻探测的 Adapter 明确返回 unsupported/unknown。手动生成成功如何参与恢复须显式定义并测试，不能静默改变路由/熔断合同。
- **D-04:** 统一网关计时从入口起算，用单调时钟记录成功 write+flush 的首个/最后一个客户端可见语义输出；text/tool 分型。响应头、role-only、空事件、Usage、DONE 和隐藏候选/Judge事件不算首token。成功结束才有完整 TTLT；中断/无语义输出/非流式的 token 时长为 nullable，保留 last-output 诊断和独立 E2E。这里只证明服务器已成功写/flush，不宣称测得客户端实际收到或解码时刻。HTTP交付观测与现有 Provider终态分开，不新增结算owner或因观测重复finalize；旧指标不得无说明地混入口径不同的新样本。
- **D-05:** 元数据采集可选且默认关闭；首版仅有界节点本地内存，受 TTL、条数、总字节、单字段和attempt条数限制，满载时非阻塞丢弃/驱逐并有低基数原因。只存白名单时间/有限状态/内部ID/安全关联字段；无正文、工具参数、凭据、原始header/URL查询或任意上游错误文本。无需新数据库、Redis持久化、日志搜索平台或迁移；明确重启丢失、节点范围与过期限制。
- **D-06:** 一个服务端生成内部ID贯穿请求、尝试、trace和诊断，并以明确响应头供查询；外部ID不可信，校验长度/格式，以安全关联表示保留（首选摘要，查询同样规范化），重复外部ID只能返回有界多匹配，不能覆盖/唯一化错误记录。现有结算UUID和公开协议ID保持各自合同，不能改用外部ID作为扣费键。metrics不使用任何请求ID或不受限外部值作标签。
- **D-07:** 复用现有全局管理员鉴权；本阶段不扩JWT/RBAC。新增查询、探测及现有TestConnection触点必须有明确权限、schema校验、速率/并发限制。探测只访问注册且已验证的Provider目标，不接收任意URL；凭据不得经重定向泄漏，也不能一刀切禁止既有合法自托管端点。手动生成需明确Provider/model和有限输出/时间/调用数，潜在上游费用与Usage单独归类；不扣业务租户、不走其缓存/排队/故障转移，不混入业务健康和用户延迟指标。后台工作必须使用独立有界容量，保留数据面资源并及时停止。
- **D-08:** 复用 Phase 30 已验证的请求预算/SDK发送限制和 Phase 27/28 的唯一终态/Usage/工具合同，禁止诊断重新分类已获胜的终态或影响扣费。执行前重新核对30实际源码与SUMMARY/VERIFICATION，缺依赖时停在明确前置条件；只规划31不自动执行30。生产缓存不启用，不新增在线凭据或依赖升级。
- **D-09:** 失败场景与命名测试先于产品实现；真实本地 HTTP/SSE 故障夹具优先。开发只跑聚焦检查，完整 E2E 在阶段末，两次独立新运行输出脱敏可复跑证据；Go测试内层 `-timeout 60s` 且外层硬60秒，需准确命名PASS，skip/无匹配不得算通过。修改后按Go项目做gofmt/vet/build，无前端pnpm任务，不重跑历史大规模性能批次冒充本阶段验收。

### Agent Discretion

- 选最少配置与接口，复用 DI、context、现有错误映射和标准库；具体端点/字段名及有限数值在详细计划和受控测试中确定，不硬编码部署参数。
- 探测能力以锁定SDK源码及执行时官方文档为准；无可证明安全的非生成方法就返回unsupported，不伪造成功，不把所有403/404/5xx/超时一概视为相同健康结论。
- 定义可测试的语义输出分类（工具首次有效片段需与text分型）和write/flush处理，不承诺网络接收不可见的时刻。
- 多节点查询只诚实说明当前节点范围，不新增全局聚合/复制或跨节点ID自动路由；安全关联字段是否可检索必须有有界索引和碰撞/重复用例。
</decisions>

<canonical_refs>
## Canonical References

- `.planning/PROJECT.md`、`.planning/ROADMAP.md`、`.planning/REQUIREMENTS.md` — 网关边界、阶段顺序与需求ID。
- `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md` 第9/10/11节 — Console之前先提供可信数据；该文件经junction在外部Notes目录，本轮不改写。
- `.planning/phases/30-request-budget-and-safe-failover/30-CONTEXT.md` 和 `30-01-PLAN.md` 至 `30-08-PLAN.md`（同目录）— 未来执行依赖，尚非通过证据。
- `.planning/phases/27-stream-terminal-settlement-consistency/27-CONTEXT.md`、`.planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md` — 终态/结算和工具边界。
- `.planning/phases/31-operator-diagnostics-foundation/31-PATTERNS.md` — 本阶段源码角色/调用路径与可复用片段。
- `internal/health/prober.go`、`internal/health/store.go`、`internal/health/redis_store.go` — 探测/真实流量目前共用状态的缺口。
- `internal/controlstate/admin_service.go`、`internal/http/handlers/admin_providers.go`、`internal/http/router.go` — 复用管理员测试入口与鉴权。
- `internal/http/handlers/chat.go`、`internal/http/handlers/chat_stream.go`、`internal/gateway/service_stream.go`、`internal/gateway/stream_terminal.go`、`internal/observability/` — 入口与客户端输出/Provider终态分界。
- `go.mod`、`go.sum` — 锁定SDK；执行时核对，不能以本规划宣称默认重试/探测端点已验证。
</canonical_refs>

<code_context>
## Existing Code Insights

- 已有 Prober/HealthStore/Redis状态和Admin TestConnection框架，但Adapter健康检查含配置判断/占位成功，不能当真实生成可用。
- `RecordProbe` 会影响与真实流量共用的成功/失败计数，需要先拆分语义，不能只替换HTTP请求后仍错误清空真实失败。
- 服务层TTFT是路由/admission之后的首事件；buffered/Fusion的Provider终态可先于客户端写出，因此计时交付观测属于HTTP层，结算仍归原owner。
- Usage/trace/Audit目前不能直接充当有界请求查询库；通用本地缓存缺容量/过期清理保障，不得把TTL等同内存有界。
- 现有AdminAuth是全局管理员边界，当前Admin路由没有可直接宣称已完成的rate limiting；新增触点必须落实限制。
</code_context>

<specifics>
## Delivery Outline

先交付管理员发起真实非生成探测的最薄HTTP闭环，再完善健康语义/后台隔离与显式手动生成；随后统一请求关联、客户端输出计时和有界查询，最后验证联动、容量/隐私及业务路径开销。详细计划按依赖拆成小纵向切片，每片少于15个修改文件，不为了命中阈值省略真实改动文件。

阶段退出看可重放证据：轻探测零生成调用、手动测试有限真实wire次数、失败恢复不被占位成功污染、TTFT/TTLT与实际语义write/flush一致、重复ID不混记录、权限/限速/字段脱敏有效、内存/后台资源有界、诊断开关和故障不阻塞业务请求。预计5–8工程日，另计评审和Phase30依赖等待，非承诺日期。
</specifics>

<deferred>
## Deferred Ideas

- Console界面、JWT/RBAC扩展、完整模型/账号能力目录、Provider/协议扩展、MCP与Agent运行时。
- 跨节点诊断聚合、持久日志检索、正文/工具参数留存、后台计费生成探测、自动复杂健康策略。
- Phase29原性能目标、业务语义质量、集合回收、容量和生产启用；不以31诊断验收替代这些门槛。
</deferred>
