# Phase 28: Tool Calling Protocol Completion - Context

**Gathered:** 2026-09-22
**Status:** Ready for planning
**Source:** `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md`、当前代码状态与用户对全部灰区的方案委托

<domain>
## Phase Boundary

本阶段完成 `/v1/chat/completions` 工具调用协议的端到端闭环，使 OpenAI-compatible、Anthropic、Gemini 三类适配器在非流式、流式和多轮续接场景下遵循同一公共契约。

范围包括：入口校验、内部类型语义、assistant tool calls、tool results、稳定调用 ID、流式增量映射、结束原因、能力路由、组合模式限制、错误语义、契约测试和验收证据。

明确不包括：网关代执行工具、MCP 桥接、Agent 编排、新供应商/新端点、完整模型能力目录、通用重试与超时治理、语义缓存、Console 管理面功能。
</domain>

<decisions>
## Decisions

### 公共协议与请求校验

- **D-01:** 公共入口只支持现代 OpenAI `tools` / `tool_choice` 形态，首期仅支持 `type: "function"`。
- **D-02:** 旧版 `functions` / `function_call` 兼容不进入本阶段；不得通过隐式转换扩大范围。
- **D-03:** 请求解码后使用强类型内部 `ToolChoice`，覆盖 omitted、`auto`、`none`、`required`、指定函数五种语义，替换业务链路中的 `any` 判定。
- **D-04:** 不支持的工具类型、缺失 function、空白或重复函数名、畸形指定函数、指定但未声明的函数，一律在入口返回稳定 `400 invalid_request`，不得转发给上游。
- **D-05:** `parameters` 作为不透明 JSON Schema 对象传递，只做结构级校验；本阶段不实现完整 JSON Schema 求值器。
- **D-06:** assistant `tool_calls` 必须拥有非空且唯一的 ID、`function` 类型、有效名称和字符串形式的参数 JSON；assistant 内容允许为空，也允许与 tool calls 共存。
- **D-07:** `role=tool` 必须引用本次对话中尚未结算的先前 assistant tool call。并行调用结果可任意顺序返回，但每个 ID 只能结算一次，且进入下一个非 tool 轮次前必须全部结算。
- **D-08:** 网关只校验、映射和转发工具调用，不执行工具，也不解释参数业务含义。

### 调用身份与跨轮关联

- **D-09:** 上游提供有效调用 ID 时原样保留。
- **D-10:** 上游缺失 ID 时由适配器生成稳定、不透明的调用 ID；同一调用在流式片段、最终响应及后续 tool result 关联中必须一致。
- **D-11:** 生成 ID 只需请求域内抗冲突，不包含密钥、租户、提示词或工具参数等敏感信息。
- **D-12:** 每个 tool call 使用稳定的零基 `index`；允许多个调用片段交错到达，但同一 index 不得切换身份或函数名。
- **D-13:** 供应商特有的关联信息封装在适配器内部。对于缺少公共 call ID 的 Gemini 形态，通过先前 assistant 调用的名称/顺序恢复供应商关联，同时仍对公开 `tool_call_id` 做严格检查。
- **D-14:** 重复 ID、未知 ID、名称冲突、片段身份漂移或无法恢复关联均视为协议错误，不做猜测式修复。

### 供应商能力与路由

- **D-15:** OpenAI-compatible、Anthropic、Gemini 必须通过同一最低功能契约：工具定义、auto/none/required/指定函数、非流式调用、流式调用、tool result 续接。
- **D-16:** `tool_choice` 必须忠实映射。某供应商或模型无法表达某模式时，路由阶段排除或显式拒绝，禁止静默降级为 `auto`。
- **D-17:** 在现有静态 capability 基础上仅增加完成本阶段所需的工具选择模式粒度；完整的模型能力目录留到后续阶段。
- **D-18:** 自动路由排除能力不匹配候选；显式指定不兼容供应商/模型时返回稳定 `400 unsupported_tool_calling` 或 `unsupported_tool_choice`，不伪装为 `503`。
- **D-19:** 无合格候选时在调用上游前失败。仅保留已有的、尚未产生可见输出前且语义安全的 fallback；本阶段不新增通用重试机制。
- **D-20:** 单供应商和轮询模式只有在所有实际候选均满足请求能力时才可执行。Fusion 请求包含 tools/tool calls/tool results 时直接拒绝，避免合并不同调用身份。
- **D-21:** 供应商错误映射为稳定网关错误；日志和指标不得记录工具参数或工具结果原文。

### 流式语义与终态

- **D-22:** 标准化后的 tool-call 增量应即时下发，不等待完整调用组装完成。
- **D-23:** 首个片段携带稳定 index、ID、类型和函数名；后续片段主要追加 arguments。保持供应商顺序和并行交错关系。
- **D-24:** arguments 在传输中按不透明字符串片段处理；在调用完成时拼接并验证为合法 JSON。
- **D-25:** 响应包含工具调用时，规范化 `finish_reason` 为 `tool_calls`；供应商结束原因与实际事件冲突时按无效上游响应处理。
- **D-26:** 畸形增量、身份漂移、最终参数 JSON 无效、非法状态迁移，走现有流错误路径并标记 provider bad response；已下发事件不回撤。
- **D-27:** Phase 27 的流终态收敛器继续作为 cancel、done、健康度、熔断器、资源释放和 Usage 结算的唯一权威路径。
- **D-28:** 现有响应语义规则不得改写工具参数和工具结果；工具事件旁路规则需要显式测试覆盖。

### 验证策略

- **D-29:** 合并门槛以确定性本地测试为准；真实供应商调用只作为可选 smoke test，不作为唯一验收证据。
- **D-30:** 建立共享契约测试矩阵，三家适配器复用同一组语义场景，并保留供应商特有 fixture。
- **D-31:** 先运行聚焦测试，再运行完整后端测试；所有后端测试命令执行硬上限 60 秒。
- **D-32:** 本阶段不设置映射层基准数值门槛，但必须证明未新增网络 I/O、未全量缓冲流、状态只随未完成调用数量有界增长、没有逐片段高基数日志。
- **D-33:** 开发前重新核验当期 OpenAI、Anthropic、Gemini 官方工具调用与 SDK 类型文档。若当前官方行为与本上下文冲突，必须先显式修订计划，不得在实现中暗改契约。

### Agent Discretion

- 强类型结构、校验器和错误常量的具体命名。
- 生成调用 ID 的前缀、编码和请求域内实现方式。
- 共享契约 harness、fixture 和测试文件的具体组织。
- 在不改变上述边界与依赖关系的前提下，将工作拆成多少个可执行 PLAN。
</decisions>

<delivery_plan>
## Recommended Delivery Plan

| 模块 | 目标与范围 | 优先级 | 关键任务 | 主要依赖 | 预估 |
|---|---|---:|---|---|---:|
| M1 强类型公共契约与入口校验 | 将工具定义、选择策略、历史调用与结果变成可验证契约 | P0 | 定义类型；实现结构与对话状态校验；稳定 400 错误；补 handler 测试 | 现有 chat 请求模型与错误响应 | 1-1.5 人日 |
| M2 调用身份与标准化状态 | 确保 ID/index/arguments 在流式和多轮中一致 | P0 | ID 保留/生成；增量聚合状态机；重复/未知/漂移检测；终态 JSON 校验 | M1、Phase 27 finalizer | 1-1.5 人日 |
| M3 三家供应商适配器对齐 | 达成统一最低工具调用能力 | P0 | OpenAI 回归；Anthropic assistant tool_use/tool_result/tool_choice；Gemini functionCall/functionResponse/关联恢复/tool_choice；结束原因映射 | M1、M2、官方协议确认 | 2-3 人日 |
| M4 能力路由与组合模式护栏 | 只把请求发给能忠实执行的候选 | P0 | capability 模式扩展；候选过滤；显式路由错误；Fusion 拒绝；fallback 约束 | M1、M3、现有 router/catalog | 0.75-1 人日 |
| M5 流式终态集成 | 即时转发工具增量且不破坏 Phase 27 结算 | P0 | 标准事件输出；interleaving；finish_reason；错误注入；规则旁路；cancel/done/usage 回归 | M2、M3、M4、Phase 27 | 1-1.5 人日 |
| M6 契约矩阵、UAT 与文档 | 形成可重复验收证据和兼容性说明 | P0 | 共享 adapter contract；handler/service/integration 矩阵；安全断言；可选 provider smoke；接口说明 | M1-M5 | 1.5-2 人日 |

总量按单人串行估算约 **7-10 个工程日**，不含真实供应商账号、模型权限或协议变更导致的等待时间。

### 实施阶段

1. **阶段 A - 契约冻结与失败测试（1-2 天）**：完成官方协议核验、内部类型/错误语义、共享场景表和入口失败测试。
2. **阶段 B - 非流式供应商闭环（2-3 天）**：先打通三家非流式工具调用和 tool result 续接，再统一能力声明。
3. **阶段 C - 流式与路由闭环（2-3 天）**：实现稳定增量身份、并行交错、终态校验、候选过滤和 Fusion 护栏。
4. **阶段 D - 全链路验收（1-2 天）**：运行共享契约、服务与集成测试，核对安全/性能形态，整理 UAT 证据和接口说明。

### 依赖关系

```text
M1 公共契约与校验
  +--> M2 身份与状态 ----+
  |                     +--> M5 流式终态 ----+
  +--> M3 供应商对齐 ---+--> M4 能力路由 ----+--> M6 契约/UAT

Phase 27 流终态收敛器 ---------> M2 / M5
当期官方协议与 SDK 核验 --------> M1 / M3 / M4
```
</delivery_plan>

<acceptance_contract>
## Acceptance Contract

### 功能与接口验收

1. **请求校验**：所有合法 `tools` / `tool_choice` 形态通过；所有 D-04 所列非法形态在入口稳定返回 400，且未产生上游调用。
2. **非流式响应**：三家适配器均输出 OpenAI-compatible `tool_calls`，保留/生成稳定 ID，arguments 为合法 JSON 字符串，结束原因为 `tool_calls`。
3. **多轮续接**：assistant tool calls 与后续多个 tool results 可正确往返；未知、重复、缺失和未结算 ID 均被拒绝。
4. **流式响应**：支持单调用、多调用和交错增量；相同 index 的 ID/名称稳定，参数拼接结果与非流式等价，且首片段无需等待完整参数。
5. **能力路由**：自动路由不会选择不兼容候选；显式不兼容请求得到稳定 400；Fusion 工具请求在上游调用前被拒绝。
6. **失败语义**：供应商协议错误、畸形流、非法结束原因和取消路径均进入既有终态收敛器，不重复完成、不泄漏 lease、不错误结算 Usage。
7. **安全性**：日志、指标、错误消息中不出现工具参数、工具结果、提示词或密钥；错误标签保持低基数。
8. **兼容性**：无 tools 的现有聊天请求、纯文本流式请求和 Phase 27 终态行为全部回归通过。
9. **性能形态**：工具流保持增量转发；无新增外部 I/O；内存状态与未完成调用数量线性且有界；无逐片段高基数日志。
10. **证据**：聚焦测试与 `go test -timeout 60s ./...` 通过；每个共享契约场景都有三家适配器结果；可选真实供应商 smoke 结果单独记录，不替代确定性测试。

### 开发前必须确认

- 当期 OpenAI、Anthropic、Gemini 官方请求/响应/流式事件和 `tool_choice` 支持矩阵。
- 仓库锁定 SDK 版本的实际类型与序列化形态，是否需要升级；升级必须单独评估兼容风险。
- 目标模型是否全部支持 required 和指定函数；不支持模型如何在静态能力中标记。
- 现有 router 的显式供应商/模型选择错误契约及可复用错误码。
- Fusion、轮询和 fallback 的准确入口，确保拒绝发生在任何上游副作用之前。
- Phase 27 完整验证状态，尤其是流错误、取消、Usage 与 lease/breaker 的终态不变量。
- 是否具备三家真实供应商测试账号与低成本模型；没有也不阻塞本阶段合并。
</acceptance_contract>

<canonical_refs>
## Canonical References

### 规划与阶段边界

- `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md`
- `.planning/phases/27-stream-terminal-and-usage-settlement/27-CONTEXT.md`
- `.planning/PROJECT.md`
- `.planning/ROADMAP.md`

### 公共请求与内部语义

- `internal/http/handlers/chat.go`
- `internal/http/handlers/chat_stream.go`
- `internal/http/handlers/chat_test.go`
- `internal/llm/types.go`

### 供应商与能力路由

- `internal/llm/openai/adapter.go`
- `internal/llm/anthropic/adapter.go`
- `internal/llm/gemini/adapter.go`
- `internal/llm/capabilities.go`
- `internal/llm/catalog.go`
- `internal/llm/router.go`
- `internal/llm/adaptertest/`

### 网关流式链路与验证

- `internal/gateway/service_stream.go`
- `internal/gateway/service_stream_terminal.go`
- `internal/gateway/fusion.go`
- `internal/gateway/service_stream_test.go`
- `tests/integration/chat_completions_test.go`
</canonical_refs>

<code_context>
## Existing Code Context

- `internal/llm/types.go` 已存在 `RoleTool`、`Tool`、`ToolCall`、`ToolCallChunk` 和流事件字段，可作为收敛类型而不是重建协议层。
- HTTP handler 已透传 tools、tool_choice、assistant tool_calls 和 tool_call_id，但当前校验不足以表达跨轮状态规则。
- OpenAI-compatible 适配器已具备较完整透传路径，适合作为共享契约基线。
- Anthropic 已有 tool definition、tool_result 和 tool_use 映射基础，需要补 assistant 历史调用与 tool choice 对齐。
- Gemini 已有 declaration 和 function call 输出基础，需要补 tool result、稳定关联、tool choice 与结束原因语义。
- Phase 27 已提供统一流终态收敛器，本阶段应接入而不是建立第二套完成/结算路径。
- 当前 capability 以适配器级布尔值为主，可做最小粒度扩展，但不应在本阶段演化成完整动态目录。
</code_context>

<specifics>
## Specific Constraints

- 低延迟优先：流式工具参数不得为统一格式化而整段缓存。
- 协议正确性优先于“尽量成功”：无法忠实映射时必须早失败。
- 生成的调用 ID 是请求/对话协议关联键，不承诺跨独立请求的持久稳定性。
- 首期仅支持 function tools，避免把未来工具类型误包装成函数调用。
</specifics>

<deferred>
## Deferred Ideas

- 旧版 `functions` / `function_call` 兼容层。
- MCP client/server 桥接、网关代执行工具、Agent loop 与权限治理。
- 完整模型能力目录、动态发现、能力版本与 Console 管理界面。
- 新供应商、新端点、通用重试/超时预算、缓存和成本策略。
</deferred>

---

*Phase: 28-tool-calling-protocol-completion*
*Context gathered: 2026-09-22*
