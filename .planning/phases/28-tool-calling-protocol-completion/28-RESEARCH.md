# Phase 28: Tool Calling Protocol Completion - Research

**Researched:** 2026-09-22
**Domain:** OpenAI-compatible tool-calling protocol normalization across OpenAI-compatible, Anthropic, and Gemini adapters
**Confidence:** MEDIUM

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

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

### the agent's Discretion

- 强类型结构、校验器和错误常量的具体命名。
- 生成调用 ID 的前缀、编码和请求域内实现方式。
- 共享契约 harness、fixture 和测试文件的具体组织。
- 在不改变上述边界与依赖关系的前提下，将工作拆成多少个可执行 PLAN。

### Deferred Ideas (OUT OF SCOPE)

- 旧版 `functions` / `function_call` 兼容层。
- MCP client/server 桥接、网关代执行工具、Agent loop 与权限治理。
- 完整模型能力目录、动态发现、能力版本与 Console 管理界面。
- 新供应商、新端点、通用重试/超时预算、缓存和成本策略。

### Additional Phase Constraints

- 低延迟优先：流式工具参数不得为统一格式化而整段缓存。
- 协议正确性优先于“尽量成功”：无法忠实映射时必须早失败。
- 生成的调用 ID 是请求/对话协议关联键，不承诺跨独立请求的持久稳定性。
- 首期仅支持 function tools，避免把未来工具类型误包装成函数调用。
</user_constraints>

The constraints above are copied verbatim from the authoritative phase context. [VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:18-73,186-191,195-200]

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TOOL-F01 | Complete internal and provider mappings for `tools`, `tool_choice`, tool-call fragments, and `tool_call_id`. | The gap inventory, provider matrix, state-machine boundaries, routing contract, and validation matrix below map this requirement to implementation tasks. [VERIFIED: .planning/REQUIREMENTS.md:25-30; exact requirement quoted in this row] |
</phase_requirements>

## Summary

VeloxMesh already carries tool definitions, assistant tool calls, tool-call chunks, and tool-result IDs through its public structs, but the contract is not enforced end to end. The decisive defects are typed-decoding and conversation validation at the handler, provider-specific request/response gaps in Anthropic and Gemini, absence of stream identity/final-JSON validation, adapter-wide boolean-only capabilities, and routing paths that can select incompatible candidates or Fusion before rejecting tool protocol traffic. [VERIFIED: internal/llm/types.go:30-99,116-166; internal/http/handlers/chat.go:25-99,152-164; internal/providers/capabilities.go:38-113; internal/routing/router.go:57-79,127-247,270-288]

The pinned SDKs are adequate and should remain pinned for this phase. Anthropic SDK `v1.50.1` exposes `OfAuto`, `OfAny`, `OfTool`, and `OfNone`, plus schema `ExtraFields`; Gemini SDK `v1.60.0` exposes `FunctionCall.ID`, `FunctionResponse.ID`, `AUTO`, `ANY`, `NONE`, `AllowedFunctionNames`, and `ToolConfig`. [VERIFIED: go.mod:7-22; exact versions quoted: `github.com/anthropics/anthropic-sdk-go v1.50.1`, `google.golang.org/genai v1.60.0`; C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6632-6652,6692-6770; C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:314-335,1260-1280,1342-1371,2592-2616]

Current official documentation does not conflict with D-01 through D-33. OpenAI documents modern function tools, tool-choice control, assistant tool calls, correlated tool messages, streamed deltas, and `finish_reason: "tool_calls"`. Anthropic documents `tool_use`/`tool_result`, `auto`/`any`/`tool`/`none`, and incremental `partial_json`. Gemini documents function declarations/calls/responses and `AUTO`/`ANY`/`NONE` with allowed function names. [CITED: https://developers.openai.com/api/docs/guides/function-calling] [CITED: https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create] [CITED: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use] [CITED: https://platform.claude.com/docs/en/build-with-claude/streaming] [CITED: https://ai.google.dev/gemini-api/docs/function-calling] [CITED: https://ai.google.dev/api/generate-content]

**Primary recommendation:** implement one shared typed request validator and one shared tool-call normalization state machine, then make each adapter and routing path consume those contracts; keep Phase 27’s finalizer as the only terminal owner and bypass response-rule buffering before the first tool-call fragment can be delayed. [VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:24-65; internal/gateway/service_stream.go:127-228,254-325]

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Public `tools`/`tool_choice` decoding and conversation validation | API / Backend | — | The HTTP boundary owns structural validation and must fail before routing or provider I/O. [VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:22-29] |
| Provider request/response translation | API / Backend | External provider boundary | Adapters own Anthropic/Gemini-specific content blocks, identifiers, stop reasons, and tool-choice wire shapes. [VERIFIED: internal/providers/anthropic/adapter.go:82-185,298-413; internal/providers/gemini/adapter.go:84-169,172-241,301-379] |
| Candidate capability filtering | API / Backend | Configuration / Catalog | The router must exclude or reject incompatible candidates before upstream calls. [VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:42-48; internal/routing/router.go:57-79,127-247,270-288] |
| Streaming identity and argument assembly | API / Backend | Provider adapter | Normalization must preserve provider event order while validating stable index/ID/name and final JSON. [VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:52-57] |
| Terminal health, breaker, release, and settlement effects | API / Backend | Observability / Storage | Phase 27’s one-shot finalizer already owns these effects and must receive the normalized protocol error or completion exactly once. [VERIFIED: internal/gateway/service_stream_terminal.go:39-114; internal/gateway/stream_terminal_test.go:18-27,81-180] |
| Tool execution, MCP, and agent loops | Out of scope | — | The gateway validates, maps, and forwards only. [VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:29,197-200] |

## Project Constraints (from AGENTS.md)

### Code and Safety

- Do not use `git restore`, `git stash`, `git checkout`, or `git worktree` unless explicitly requested; expose failures instead of swallowing them; obtain approval for high-risk deletions, bulk edits, or DDL. [VERIFIED: AGENTS.md:3-9]
- Use ES modules and destructured imports where JavaScript/TypeScript is touched. Prefer immutable values and dependency injection; avoid mutating existing objects or constructing implementations inside business logic. [VERIFIED: AGENTS.md:15-19]
- Keep functions at 50 lines or fewer, files at 500 lines or fewer, nesting at three levels or fewer, positional parameters at three or fewer, and cyclomatic complexity at ten or fewer; extract magic numbers. [VERIFIED: AGENTS.md:21-32]
- Never hardcode secrets or configuration credentials. Validate external input at system boundaries and preserve injection, XSS, CSRF, and endpoint rate-limiting controls. [VERIFIED: AGENTS.md:36-41]
- Tests must cover core input/output behavior, regression boundaries/error paths, and real integrations with minimal mocking; do not add coverage-only, redundant, implementation-detail, deprecated, over-mocked, or trivial tests. [VERIFIED: AGENTS.md:63-78]

### Workflow and Tooling

- Every backend test command must enforce a hard 60-second timeout. [VERIFIED: AGENTS.md:9-9]
- Use `pnpm run build` and `pnpm run typecheck` for frontend changes, `uv` for Python, `rg` for search, and Git Bash by default; terminate local services/test processes after use and run relevant linters/typecheckers after changes. [VERIFIED: AGENTS.md:45-56]
- Use commit messages in `<type>: <description>` form; PRs require history review, a concise summary, and a test plan. [VERIFIED: AGENTS.md:58-61]
- Follow `Receive -> Gather Context -> Plan -> Execute -> Test/Reflect -> Deliver`. [VERIFIED: AGENTS.md:82-86]
- Limit advanced MCP usage to two calls per turn; use Context7 first for library/SDK documentation and the prescribed fetch/search tools for web research. [VERIFIED: AGENTS.md:90-101]
- Keep document structure readable with whitespace around headings and short lists; use ASCII diagrams for UI/UX changes. [VERIFIED: AGENTS.md:105-111]
- Use CodeGraph before grep/file reads when `.codegraph/` exists; this research did so for the request path, provider adapters, routing, and terminal-finalizer call paths. [VERIFIED: AGENTS.md:114-125; current-session CodeGraph queries]

## Current Code Gap Inventory

| Area | Current implementation evidence | Gap to close |
|------|---------------------------------|--------------|
| Shared request types | `ChatCompletionRequest.ToolChoice any` and `LLMRequest.ToolChoice any`. Exact current declarations: `ToolChoice any \`json:"tool_choice,omitempty"\`` and `ToolChoice any`. [VERIFIED: internal/llm/types.go:78-99; values quoted verbatim] | Replace business-path `any` with a decoded union that distinguishes omitted, auto, none, required, and named function. |
| Handler decoding | The proxy request also uses `ToolChoice any`; content JSON unmarshal errors are ignored; validation checks only message presence, role, and nonempty tool result ID. [VERIFIED: internal/http/handlers/chat.go:25-82,152-164] | Add one boundary validator for tools, choice, assistant calls, JSON arguments, and full conversation settlement order; return stable `400 invalid_request` before service invocation. |
| Internal tool structs | Exact existing role values are `"system"`, `"user"`, `"assistant"`, `"tool"`; exact tool type is `"function"`. Existing tool call/chunk fields already carry ID, index, name, and argument strings. [VERIFIED: internal/llm/types.go:3-10,38-76; values quoted verbatim] | Extend rather than replace these structs; add normalized choice and protocol-state helpers beside them. |
| OpenAI-compatible adapter | It forwards messages, tools, and raw choice directly, and forwards streamed chunks; nonstream validates only nonempty choices, while stream accepts clean EOF after any event as Done. [VERIFIED: internal/providers/openai/adapter.go:76-130,175-223,276-347] | Preserve pass-through transport, but run both complete responses and stream deltas through the shared validator/normalizer; reject ID/name/index drift, bad argument JSON, and finish-reason conflict as `provider_bad_response`. |
| Anthropic request mapping | Assistant history is converted to one text block only; tool results are mapped; schemas are reduced to `properties` and `required`; `ToolChoice` is not set. [VERIFIED: internal/providers/anthropic/adapter.go:91-185] | Encode assistant text plus every `tool_use`, preserve the opaque schema using SDK extra fields, and map all five public choice states to omitted/auto/none/any/tool. |
| Anthropic stream mapping | The adapter emits tool identity at `content_block_start` and argument fragments at `input_json_delta`, but does no `content_block_stop` final-JSON validation or identity state tracking. [VERIFIED: internal/providers/anthropic/adapter.go:298-413] | Use the shared assembler keyed by provider content-block index; finalize calls at block stop and validate the message-level stop reason against observed calls. |
| Gemini request mapping | Assistant tool calls and tool results are not encoded; tool declarations are present; `ToolConfig` is absent. [VERIFIED: internal/providers/gemini/adapter.go:84-169] | Encode model `FunctionCall` parts, user `FunctionResponse` parts, recover provider association by validated name/order when needed, and map choices to `AUTO`/`NONE`/`ANY` plus allowed names. |
| Gemini response mapping | Complete and stream paths generate IDs as `call_%d`, ignore SDK-provided IDs, increment index for every observed function-call part, and map provider `STOP` to public `stop` even when calls exist. [VERIFIED: internal/providers/gemini/adapter.go:172-241,301-379; values quoted verbatim] | Preserve valid upstream IDs, generate opaque IDs only when absent, keep one stable index per logical call, and force public `tool_calls` only when observed calls and provider finish semantics agree. |
| Capability model | Exact provider types are `"openai-compatible"`, `"anthropic"`, `"gemini"`; `CapabilitySet` has one `ToolCalling bool`, and `SatisfiesRequirements` cannot represent choice modes. [VERIFIED: internal/providers/capabilities.go:3-18,38-113; values quoted verbatim] | Add minimal choice-mode support to capability requirements and clone behavior; avoid a general feature catalog. |
| Catalog granularity | One adapter capability set is cloned onto every model, and eligibility checks only operation. [VERIFIED: internal/providers/catalog.go:54-74,98-124] | Allow the catalog entry to carry the minimal effective per-model tool-choice support used by routing, with adapter defaults and narrow exact-model overrides rather than dynamic discovery. |
| Router | Explicit override checks only model/operation; normal selection filters only model/operation; tool requirement is inferred only from `len(req.Tools) > 0`; round-robin ignores capabilities; capacity-auto’s second pass explicitly ignores capabilities; Fusion can be selected. [VERIFIED: internal/routing/router.go:57-79,127-247,270-288] | Derive one shared `ToolProtocolRequirements` from definitions, choice, assistant calls, and tool results; use it on every route mode; reject Fusion and incompatible explicit overrides before admission/provider calls. |
| Response-rule path | A response-rule-enabled stream is fully collected; only after observing a tool call does it skip rules and replay buffered events. [VERIFIED: internal/gateway/service_stream.go:254-325] | Detect tool-protocol requests before collection and take the immediate pass-through path, so D-22 is true even when response rules are enabled. |
| Semantic cache isolation | Cache lookup/store keys marshal only `req.Messages`; they do not include tools or choice, and cached choices can be returned before routing. [VERIFIED: internal/gateway/service.go:135-195,341-349] | Do not redesign caching; make tool-protocol requests ineligible for existing semantic-cache lookup/store so cached text cannot bypass choice/capability semantics. This is a protocol isolation guard, not deferred cache strategy work. [LOCKED] |
| Phase 27 terminal integration | `newStreamTerminal` wires provider health, breaker, metrics, trace, settlement, admission release, and client result; `complete` submits to the one-shot finalizer. [VERIFIED: internal/gateway/service_stream_terminal.go:39-58,66-114] | Feed normalized provider protocol failures into the existing stream error path; do not add a second terminal or settlement owner. |
| Existing tests | Handler coverage checks pass-through only; the shared adapter harness covers generic success/error behavior; router has no tool-capability matrix; the response-rule test proves content is unmodified but permits delayed replay. [VERIFIED: internal/http/handlers/chat_test.go:75-115; internal/providers/adaptertest/harness.go:13-166; internal/gateway/service_stream_test.go:217-242; current-session `rg` found no router tool cases] | Add Wave 0 shared scenarios and negative protocol fixtures before implementation; update the response-rule test to assert first tool fragment is not buffered. |

## Standard Stack

### Core

| Library / Runtime | Pinned Version | Purpose | Recommendation |
|---|---:|---|---|
| Go | `1.26.1` | Gateway implementation and tests | Keep the repository-pinned toolchain. `[VERIFIED: go.mod:3-3]` The source-of-truth declaration is `go 1.26.1`. |
| Go standard library | Toolchain-bundled | `encoding/json`, `net/http`, synchronization, errors | Use `json.Valid`/`json.RawMessage` and existing HTTP/SSE infrastructure; do not add a JSON-schema evaluator or streaming parser dependency. `[VERIFIED: internal/providers/openai/adapter.go:1-15]` |
| Anthropic Go SDK | `v1.50.1` | Anthropic request/response/stream wire types | Keep the pinned SDK. Its tool input schema and all required tool-choice variants already exist. `[VERIFIED: go.mod:8-8]` The exact module declaration is `github.com/anthropics/anthropic-sdk-go v1.50.1`. |
| Google Gen AI Go SDK | `v1.60.0` | Gemini request/response/stream wire types | Keep the pinned SDK. It already exposes function declarations, function calls/responses, tool config, allowed names, and function-calling modes. `[VERIFIED: go.mod:22-22]` The exact module declaration is `google.golang.org/genai v1.60.0`. |
| Google UUID | `v1.6.0` | Collision-resistant opaque generated call IDs | Reuse through an injected ID generator; do not derive IDs from arguments, tool names, or request content. `[VERIFIED: go.mod:10-10]` The exact module declaration is `github.com/google/uuid v1.6.0`. |

### Supporting

| Existing Component | Purpose | When to Use |
|---|---|---|
| `internal/providers/adaptertest` | Cross-adapter behavior harness | Extend with one shared tool-protocol scenario matrix rather than duplicating protocol expectations in three provider packages. `[VERIFIED: internal/providers/adaptertest/harness.go:13-47]` |
| `terminalFinalizer` | Single terminal classification and side-effect authority | Route normalized upstream tool-protocol errors through the existing Phase 27 finalizer; do not add an adapter- or handler-owned terminal path. `[VERIFIED: internal/gateway/service_stream_terminal.go:39-58]` |
| Existing SSE writer | OpenAI-compatible event emission | Continue emitting normalized `StreamEvent` values and let the handler serialize them. `[VERIFIED: internal/http/handlers/chat_stream.go:59-111]` |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|---|---|---|
| Structural-only schema validation | A full JSON Schema engine | Rejected by D-05: the gateway must keep schemas opaque and validate only the envelope. |
| Shared protocol normalizer | Provider-local ad hoc validation | Rejected because it creates three subtly different state machines and prevents a single contract matrix. |
| Existing SDK versions | SDK upgrade during this phase | No required protocol field is missing from the pinned versions; upgrading adds unrelated migration risk. |
| Existing finalizer | A tool-specific terminal coordinator | Rejected by D-27 and Phase 27’s single-winner lifecycle invariant. |

**Installation:** none. This phase requires no new external package. `[VERIFIED: go.mod:1-52]`

**Version verification:** the versions above were read from `go.mod` and their installed module-cache source was inspected in this session. The Anthropic cache contains the exact tool-choice union values `"auto"`, `"any"`, `"tool"`, and `"none"`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6692-6770]` The Gemini cache contains the exact modes `"MODE_UNSPECIFIED"`, `"AUTO"`, `"ANY"`, `"NONE"`, and `"VALIDATED"`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:314-335]`

## Package Legitimacy Audit

No package installation is recommended, so the external-package legitimacy gate is not triggered. All named modules are already pinned in the repository’s `go.mod` and were inspected from the local Go module cache. `[VERIFIED: go.mod:1-52]`

**Packages removed due to SLOP verdict:** none.

**Packages flagged as suspicious:** none.

## Provider Mapping Matrix

The public contract is the modern OpenAI-compatible `tools`/`tool_choice` shape restricted to function tools. OpenAI’s current Chat Completions reference defines function tools, tool selection, assistant tool calls with IDs, and tool-result messages that reference `tool_call_id`. `[CITED: https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create]`

### Request Choice Mapping

| Normalized Public Choice | OpenAI-Compatible | Anthropic | Gemini |
|---|---|---|---|
| omitted | Omit `tool_choice` | Omit `tool_choice` | Omit `ToolConfig` |
| `auto` | String `"auto"` | `{"type":"auto"}` | `FunctionCallingConfig.Mode=AUTO` |
| `none` | String `"none"` | `{"type":"none"}` | `FunctionCallingConfig.Mode=NONE` |
| `required` | String `"required"` | `{"type":"any"}` | `FunctionCallingConfig.Mode=ANY` with no allowed-name restriction |
| specified function | `{"type":"function","function":{"name":"<name>"}}` | `{"type":"tool","name":"<name>"}` | `Mode=ANY` plus one `AllowedFunctionNames` entry |

The Anthropic mappings are defined by the official tool-use documentation and match the pinned SDK union. `[CITED: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use]` `[VERIFIED: C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6692-6861]` The Gemini mappings use the documented `AUTO`, `ANY`, and `NONE` modes and the allowed-function-name restriction. `[CITED: https://ai.google.dev/gemini-api/docs/function-calling]` The public contract must not expose Gemini’s `VALIDATED` mode because D-03 fixes the supported public choice set.

### Full Protocol Mapping

| Contract Element | OpenAI-Compatible Adapter | Anthropic Adapter | Gemini Adapter |
|---|---|---|---|
| Tool definition | Forward `type=function` and function object after structural validation. | Build `ToolParam` and preserve the entire schema through `ToolInputSchemaParam.ExtraFields` rather than projecting only `properties`/`required`. The SDK marshaler explicitly merges `ExtraFields`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6632-6652]` | Build `FunctionDeclaration` and preserve the schema through `ParametersJsonSchema`. `[VERIFIED: internal/providers/gemini/adapter.go:140-150]` |
| Assistant call history | Forward validated assistant `tool_calls` unchanged. | Encode each call as a `tool_use` content block, preserving text blocks when content is mixed. `[CITED: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use]` | Encode each call as a model `FunctionCall` part with ID, name, and parsed argument object. `[CITED: https://ai.google.dev/api/generate-content]` |
| Tool result continuation | Forward role `tool` plus matching `tool_call_id`. | Encode a user `tool_result` block with `tool_use_id`; tool-result blocks immediately follow their associated assistant tool-use turn. `[CITED: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use]` | Encode a user `FunctionResponse` with matching ID and name. The pinned type states that the ID should match the corresponding function call. `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:1342-1371]` |
| Non-stream call response | Validate every returned ID/name/argument payload and normalize to `llm.ToolCall`. | Convert each `tool_use` block, preserving upstream ID. | Convert each `FunctionCall`, preserving upstream ID when present and generating an opaque ID only when absent. The pinned type contains optional `ID`, `Args map[string]any`, and `Name`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:1260-1280]` |
| Streaming call response | Consume provider delta index/ID/name/argument fragments; emit first identity fragment immediately and shadow-assemble arguments for final validation. | Map `content_block_start` to identity and `input_json_delta.partial_json` to arguments; finalize on `content_block_stop`. `[CITED: https://platform.claude.com/docs/en/build-with-claude/streaming]` | Emit each received complete `FunctionCall` immediately as one normalized chunk. The pinned SDK marks partial function-call arguments as unsupported by the Gemini API, so do not fabricate incremental partials. `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:1260-1280]` |
| Terminal reason | Accept `tool_calls` only when normalized calls are present and complete; conflicting terminal semantics are a provider bad response. | Map `stop_reason=tool_use` to public `tool_calls` only after all tool blocks validate. `[CITED: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use]` | Normalize a response containing function calls to public `tool_calls` even if the provider finish enum is otherwise generic; reject contradictory terminal state. |

### Current Provider Gaps

- **OpenAI-compatible:** request forwarding exists, but response validation is limited to checking that `choices` is nonempty; streaming forwards deltas without identity-drift, duplicate-ID, final-JSON, or finish-reason checks. `[VERIFIED: internal/providers/openai/adapter.go:175-189]` `[VERIFIED: internal/providers/openai/adapter.go:276-347]`
- **Anthropic:** assistant tool-call history is dropped because assistant messages are emitted only as text blocks; tool choice is not mapped; schemas are flattened; streaming never validates completed argument JSON. `[VERIFIED: internal/providers/anthropic/adapter.go:82-185]` `[VERIFIED: internal/providers/anthropic/adapter.go:298-413]`
- **Gemini:** assistant tool-call history and function responses are not represented, `ToolConfig` is not set, upstream IDs are ignored, IDs are synthesized from local ordinal position, and function calls terminate as `stop`. `[VERIFIED: internal/providers/gemini/adapter.go:84-169]` `[VERIFIED: internal/providers/gemini/adapter.go:172-241]` `[VERIFIED: internal/providers/gemini/adapter.go:301-379]`

## Architecture Patterns

### System Architecture Diagram

```text
OpenAI-compatible HTTP request
        |
        v
Handler structural validator
  - tools envelope
  - strong tool_choice
  - message/tool-call sequence
        |
        v
Normalized llm request + derived ToolRequirements
        |
        +--> semantic cache bypass for tool-protocol traffic [LOCKED]
        |
        v
Router preflight
  - provider/model operation support
  - tool-calling support
  - choice-mode support
  - topology compatibility
        |
        +--> stable 400 contract error before upstream I/O
        |
        v
Provider request mapper
  OpenAI | Anthropic | Gemini
        |
        v
Provider response / stream normalizer
  - preserve or generate call identity
  - per-index state machine
  - immediate fragment emission
  - shadow argument assembly
        |
        +--> provider_bad_response on malformed upstream protocol
        |
        v
Gateway StreamEvent pipeline
  - explicit response-rule bypass
  - Usage passthrough
        |
        v
Phase 27 terminalFinalizer (single authority)
  health -> breaker -> metrics -> trace -> settlement -> release -> client terminal
        |
        v
OpenAI-compatible JSON / SSE response
```

### Recommended Project Structure

```text
internal/
├── llm/
│   ├── types.go                    # existing public-neutral request/response types
│   ├── tool_protocol.go            # strong choice, derived requirements, history validation
│   └── tool_protocol_test.go
├── providers/
│   ├── toolstream/
│   │   ├── state.go                # shared upstream stream-call state machine
│   │   └── state_test.go
│   ├── adaptertest/
│   │   └── tool_contract.go        # shared request/response/stream scenarios
│   ├── openai/
│   ├── anthropic/
│   └── gemini/
├── routing/
│   └── router_tool_test.go
├── gateway/
│   ├── service_tool_test.go
│   └── service_stream_tool_test.go
└── http/handlers/
    └── chat_tool_test.go
tests/integration/
└── chat_tools_test.go
```

These are recommended implementation locations, not current paths. They preserve the existing ownership boundaries: public validation in `llm`/handler code, provider wire mapping in adapters, selection in routing, and lifecycle ownership in gateway finalization.

### Pattern 1: Strong Normalized Choice at the Boundary

**What:** Decode `tool_choice` once into a closed internal representation. Keep JSON compatibility in the HTTP proxy type if useful, but never pass an untyped `any` beyond boundary normalization.

**When to use:** Every request, before routing or provider mapping.

```go
// Recommended internal shape. Values are locked by D-03.
type ToolChoiceMode string

const (
    ToolChoiceAuto     ToolChoiceMode = "auto"
    ToolChoiceNone     ToolChoiceMode = "none"
    ToolChoiceRequired ToolChoiceMode = "required"
    ToolChoiceFunction ToolChoiceMode = "function"
)

type ToolChoice struct {
    Mode ToolChoiceMode
    Name string // nonempty only for Mode == ToolChoiceFunction
}
```

The exact public semantic states are `omitted/auto/none/required/specified function`. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:23-23]`

### Pattern 2: One Derived Protocol Requirement

**What:** Centralize the predicate that determines whether a request uses the tool protocol. It must cover more than `len(req.Tools)>0`: declarations, explicit choice, assistant call history, and tool-result continuation all require compatible routing and cache behavior.

**When to use:** Handler validation, routing, topology rejection, cache bypass, response-rule bypass, and tests.

```go
type ToolProtocolRequirements struct {
    UsesProtocol    bool
    ChoiceMode      ToolChoiceMode
    NeedsDefinitions bool
    HasCallHistory  bool
    HasToolResults  bool
}
```

The current router derives only `requiresTools := len(req.Tools) > 0`, so history-only and choice-only requests are not capability-gated. `[VERIFIED: internal/routing/router.go:127-139]`

### Pattern 3: Immediate Emit, Shadow Assemble

**What:** For each stable zero-based call index, emit the normalized fragment as soon as it arrives while separately accumulating argument bytes and immutable identity for validation. The assembler is a verifier, not a buffer.

**When to use:** OpenAI-compatible and Anthropic incremental streams; Gemini uses the same final validation but may provide a complete function call in one response part.

```go
type CallStreamState struct {
    Index     int
    ID        string
    Name      string
    SeenType  bool
    Arguments []byte
    Complete  bool
}

// Apply returns the immediately-emittable normalized fragment or a protocol error.
// It must reject identity drift, duplicate completion, and fragments after completion.
func (s CallStreamState) Apply(fragment ProviderCallFragment) (CallStreamState, llm.ToolCallChunk, error)
```

Use value-returning transitions or copy-on-write maps to comply with the repository’s immutability directive. The public stream chunk’s exact optional fields are `Index *int`, `ID *string`, `Type *ToolType`, and `Function *FunctionCallChunk`. `[VERIFIED: internal/llm/types.go:58-76]`

### Pattern 4: Capability Preflight Before I/O

**What:** Resolve effective provider/model capabilities and topology compatibility before calling any adapter. Keep the capability extension narrow: tool protocol support plus supported choice modes.

**When to use:** Explicit override, ordinary single routing, round-robin selection, and Fusion rejection.

Recommended capability shape:

```go
type ToolChoiceSupport struct {
    Auto     bool
    None     bool
    Required bool
    Function bool
}

type CapabilitySet struct {
    // Existing fields remain.
    ToolCalling bool
    ToolChoice  ToolChoiceSupport
}
```

Because `CapabilitySet` is copied and cloned in the catalog, update `Clone` and all constructors/tests when adding nested capability state. The current exact fields include `ChatCompletions bool`, `Streaming bool`, and `ToolCalling bool`. `[VERIFIED: internal/providers/capabilities.go:38-48]`

### Anti-Patterns to Avoid

- **Passing `tool_choice any` through the stack:** defers malformed shape discovery until a provider rejects it and makes mode capability checks impossible.
- **Validating tools only when declarations are present:** misses continuation turns and explicit choices.
- **Generating IDs from sequence alone:** collides across requests and loses upstream correlation; preserve upstream IDs and inject a collision-resistant generator for missing IDs.
- **Buffering tool fragments for semantic rules:** violates immediate emission and changes TTFT; bypass rules before buffering.
- **Treating provider finish enums as sufficient:** terminal semantics must agree with normalized call state.
- **Adding provider-specific terminal side effects:** breaks Phase 27’s exactly-once lifecycle.
- **Silently coercing unsupported choices to auto:** violates request intent and D-16/D-18.

## Routing, Capability, and Error Contracts

### Routing Contract

1. Derive `ToolProtocolRequirements` before cache lookup or routing. This is required because the current semantic-cache key hashes only messages and omits tools and choice. `[VERIFIED: internal/gateway/service.go:135-185]`
2. Reject Fusion whenever `UsesProtocol` is true. The current Fusion branch executes before any tool-specific guard and selects multiple upstreams. `[VERIFIED: internal/gateway/service_stream.go:65-94]` `[VERIFIED: internal/routing/router.go:222-247]`
3. For explicit overrides, check effective provider/model tool and choice-mode capability before adapter I/O. The current override path checks model and operation only. `[VERIFIED: internal/routing/router.go:270-288]`
4. For ordinary single and round-robin routing, every actual candidate considered executable for this request must satisfy the derived requirements. The current round-robin combo path has no capability preflight. `[VERIFIED: internal/routing/router.go:142-167]`
5. Remove the capacity-auto second pass that ignores capability requirements for tool-protocol requests. The current fallback pass explicitly retries selection without the capability requirement. `[VERIFIED: internal/routing/router.go:169-220]`
6. Resolve model-level support through a small effective-capability function: adapter default plus exact model overrides. Do not build a persistent or dynamic provider catalog in this phase; model catalog expansion is deferred by CONTEXT. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:195-200]`

### Stable Error Contract

| Failure Class | HTTP / Stream Contract | Health / Retry Meaning |
|---|---|---|
| Malformed client tool declaration, choice, call history, or tool result | HTTP 400 `invalid_request` before routing | Client-owned; no provider health effect and no retry. |
| Provider/model lacks tool protocol | HTTP 400 `unsupported_tool_calling` before upstream I/O | Capability mismatch; no provider health effect. |
| Provider/model cannot honor the requested choice mode | HTTP 400 `unsupported_tool_choice` before upstream I/O | Capability mismatch; no provider health effect. |
| Fusion requested with tool-protocol traffic | HTTP 400 `unsupported_tool_calling` before upstream I/O | Topology cannot preserve one-call identity and continuation semantics. |
| Upstream rejects an otherwise valid mapped request | Existing provider error translation | Preserve provider-invalid-request/auth/rate-limit categories as applicable. |
| Upstream emits malformed calls, identity drift, invalid final argument JSON, or contradictory finish state | `provider_bad_response`; stream emits one error terminal candidate | Provider-owned; existing health and retry classification applies, subject to pre-visible fallback only. |

The exact current provider category values include `"provider_invalid_request"`, `"provider_bad_response"`, and `"provider_internal"`. `[VERIFIED: internal/errors/errors.go:50-63]` The current error policy treats `ProviderBadResponse` as health-affecting and retryable. `[VERIFIED: internal/errors/errors.go:81-116]` D-19 and D-26 constrain that retry to the existing safe pre-visible fallback path; after visible stream output, return the normalized stream error without switching providers.

### Model Capability Data

Use a conservative allow model: a provider/model is eligible only when its effective capability record explicitly supports the requested mode. Unknown model support is not silently assumed. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:38-41]` The exact locked behavior is `auto routing excludes incompatible provider/model combinations; explicit override to an incompatible target fails before upstream I/O with a stable 400-class code`. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:38-38]`

Do not populate this matrix from model-name guesses. The planner must add exact model overrides only from repository configuration plus current provider evidence or a live smoke result. Routing is fail-closed: an unlisted or otherwise unverified model/choice-mode combination is ineligible and cannot be treated as qualified. `[LOCKED]`

## Streaming and Phase 27 Finalizer Integration

### Required Event Flow

1. The adapter receives an upstream fragment and maps provider identity to a stable zero-based normalized index.
2. The first fragment for an index includes ID, `type=function`, and function name; later fragments carry only argument bytes unless a provider repeats identical identity metadata.
3. The adapter emits the normalized fragment immediately and updates a shadow immutable state for that index.
4. On provider block/call completion, validate the assembled argument string with `json.Valid` and mark the call complete.
5. At stream terminal, require all opened calls to be complete and reconcile the provider finish reason with normalized call state.
6. On success, emit exactly one `Done` candidate with public finish reason `tool_calls` when calls occurred.
7. On protocol failure, emit exactly one error candidate using `provider_bad_response`; never emit a subsequent `Done` from the adapter.
8. The gateway submits that candidate to `terminalFinalizer`, which remains the sole owner of health, breaker, metrics, trace, settlement, admission release, and client terminal emission.

The exact Phase 27 lifecycle order is `"classification"`, `"provider_health"`, `"circuit_breaker"`, `"metrics"`, `"trace_close"`, `"settlement_decision"`, `"admission_release"`, and `"client_terminal_emission"`. `[VERIFIED: internal/gateway/stream_terminal_test.go:18-27]` Existing tests already prove first-winner, concurrent-winner, and repeated-callback exactly-once behavior. `[VERIFIED: internal/gateway/stream_terminal_test.go:81-180]`
### Response Rules and Semantic Cache

The current response-rule path buffers every stream event before replay and only notices tool calls after buffering. `[VERIFIED: internal/gateway/service_stream.go:254-325]` Phase 28 must branch on `ToolProtocolRequirements.UsesProtocol` before response-rule buffering so tool fragments are never delayed or rewritten. This directly implements D-22 and D-28.

The current semantic-cache lookup hashes messages without tools or choice, and cache storage has no tool-specific guard. `[VERIFIED: internal/gateway/service.go:135-185]` `[VERIFIED: internal/gateway/service.go:341-349]` A narrow read/write bypass for any tool-protocol request is locked as a correctness guard; redesigning semantic-cache keys or adding tool-aware caching remains outside Phase 28. `[LOCKED]`

### Usage and Terminal Semantics

Tool-call normalization must not create a second usage-settlement path. Continue forwarding provider usage into existing stream events, and let terminal classification/settlement use the Phase 27 mechanism. The exact stream event fields include `Usage *Usage`, `FinishReason string`, `Done bool`, and `Error *ProviderError`. `[VERIFIED: internal/llm/types.go:157-166]`

## Security and Logging Constraints

- Validate all tool-protocol input at the HTTP boundary before routing. Required checks are envelope shape, function-only type, valid nonblank names, duplicate declarations, selected-name declaration, call-ID uniqueness, argument JSON-string validity, and legal message sequencing. These checks are locked by D-04 through D-07. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:25-28]`
- Treat tool schemas, argument fragments, and result content as opaque potentially sensitive data. Do not log or attach them to metric labels. D-21 explicitly prohibits raw argument/result exposure. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:44-44]`
- Generated IDs must be collision-resistant, opaque, and independent of request content. Inject the generator so deterministic tests can supply fixed IDs without weakening production generation. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:31-32]`
- Keep metric dimensions bounded: provider type, choice mode, protocol outcome, and stable error code are acceptable; tool name, call ID, argument hash, result size bucket with uncontrolled cardinality, and schema content are not. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:44-44]`
- Do not execute tools, follow URLs embedded in schemas/results, load plugins, or dispatch MCP/agent work. The gateway validates, maps, and forwards only. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:29-29]`
- Bound accumulation by existing request/response limits and fail closed on impossible or illegal state transitions; do not create an unbounded side buffer per fragment. D-32 requires proof of no new unbounded buffering. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:60-60]`

Structured log fields may include `provider_type`, `model`, `streaming`, `tool_choice_mode`, `tool_call_count`, and stable `error_code`. Tenant-defined tool names are sensitive/high-cardinality and must not appear in logs, metrics, traces, or error messages; retain them only in public protocol fields and provider wire mappings. `[LOCKED]`

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| JSON Schema semantics | A validator/evaluator for schema keywords | Opaque pass-through plus structural envelope checks | D-05 forbids semantic interpretation; provider runtimes own schema semantics. |
| Provider wire unions | Stringly typed Anthropic/Gemini request maps | Pinned SDK union/config types | The required variants already exist in the pinned SDKs. `[VERIFIED: C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6692-6861]` `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:2592-2616]` |
| Call ID entropy | Counter-, name-, or argument-derived IDs | Injected generator backed by existing `uuid` dependency | Avoids cross-request collisions and content leakage. `[VERIFIED: go.mod:10-10]` |
| Stream termination | A tool-specific finalizer | Existing `terminalFinalizer` | Phase 27 already centralizes exactly-once terminal side effects. `[VERIFIED: internal/gateway/service_stream_terminal.go:39-114]` |
| Cross-provider tests | Three unrelated fixture suites | Shared `adaptertest` tool-contract matrix plus provider-specific mapping assertions | D-30 requires one matrix and only explicit provider deltas. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:58-58]` |
| Tool orchestration | Execution loop, registry, MCP bridge, or agent dispatcher | Forward-only protocol mapping | Explicitly outside the phase boundary. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:13-15]` |
| Dynamic capability discovery | Runtime catalog service or admin UI | Small static effective-capability override table | Provider/model catalog work is deferred. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:195-200]` |

**Key insight:** the hard part is preserving identity and legal turn/stream transitions across incompatible provider shapes, not executing a function. One normalized contract and one verifier state machine eliminate most cross-provider drift.

## Common Pitfalls

### Pitfall 1: Only Gating Requests That Declare Tools

**What goes wrong:** an assistant-call continuation or tool-result turn routes to a provider/model that cannot honor tool calling.

**Why it happens:** the current router uses only `len(req.Tools) > 0`. `[VERIFIED: internal/routing/router.go:127-139]`

**How to avoid:** derive one protocol requirement from declarations, explicit choice, call history, and result messages.

**Warning signs:** history-only requests bypass capability tests; explicit `none` or named choice is ignored by selection.

### Pitfall 2: Anthropic Schema Projection

**What goes wrong:** valid JSON Schema keywords outside `properties` and `required` disappear before upstream submission.

**Why it happens:** the current mapper reconstructs only those two fields. `[VERIFIED: internal/providers/anthropic/adapter.go:137-159]`

**How to avoid:** preserve the schema through the SDK’s `ExtraFields`-aware type.

**Warning signs:** enums, nested arrays, `additionalProperties`, or descriptions are absent in captured provider requests.

### Pitfall 3: Gemini Ordinal IDs

**What goes wrong:** generated IDs are not request-global opaque identifiers and upstream IDs are discarded.

**Why it happens:** the current complete and stream paths synthesize IDs from call order. `[VERIFIED: internal/providers/gemini/adapter.go:196-205]` `[VERIFIED: internal/providers/gemini/adapter.go:322-345]`

**How to avoid:** preserve `FunctionCall.ID`; only generate through an injected collision-resistant generator when absent.

**Warning signs:** repeated `call_1` values across requests or tool results that cannot be associated after reordering.

### Pitfall 4: Stream Validation That Delays Delivery

**What goes wrong:** tool-call TTFT regresses and interleaved calls appear only after completion.

**Why it happens:** validation is implemented as whole-stream buffering instead of shadow assembly.

**How to avoid:** emit each valid fragment immediately, separately accumulating only verification state.

**Warning signs:** the first client event occurs after provider `message_stop` or response-rule code sees tool calls before the client does.

### Pitfall 5: Trusting Finish Reason Without Call State

**What goes wrong:** `tool_calls` is emitted with incomplete/invalid calls, or calls are emitted with public `stop`.

**Why it happens:** provider finish enums are mapped independently from normalized call state.

**How to avoid:** reconcile terminal reason only after all call states complete and final argument strings validate.

**Warning signs:** invalid JSON arguments reach the client, or Gemini function calls terminate as `stop`.

### Pitfall 6: Fallback After Visible Output

**What goes wrong:** fragments from two providers share one public stream and call identities collide.

**Why it happens:** `provider_bad_response` remains retryable without checking whether client-visible output already occurred.

**How to avoid:** use existing pre-visible fallback only; after first emitted event, finalize the current stream error.

**Warning signs:** a second provider request begins after an SSE chunk has been written.

### Pitfall 7: Accidentally Reopening Phase 27

**What goes wrong:** tool errors duplicate health, breaker, metrics, settlement, or release effects.

**Why it happens:** adapter-specific cleanup is added alongside `terminalFinalizer`.

**How to avoid:** adapters produce normalized events only; finalizer callbacks own all terminal effects.

**Warning signs:** new direct calls to health/breaker/settlement from provider adapters or stream handler.

### Pitfall 8: Treating Current Full-Suite Failures as Product Regressions

**What goes wrong:** Phase 28 planning grows unrelated infrastructure work.

**Why it happens:** the local full suite depends on PostgreSQL/pgvector, Redis Stack/RediSearch, Qdrant, and the scheduler Python environment. The current local run failed on those unavailable services, while focused tool-adjacent packages passed. `[VERIFIED: current-session go test output]`

**How to avoid:** provision the established integration environment for the phase gate, while keeping per-task commands focused and under 60 seconds.

**Warning signs:** failures mention connection refusal/timeouts to configured dependency hosts or missing scheduler worker smoke signals rather than tool-protocol assertions.


## Code Examples

Verified SDK patterns for the implementation plans follow. The normalized `ToolChoice` type is a Phase 28 recommendation; provider constructors and fields are from the pinned SDK source.

### Anthropic Choice Mapping

```go
// Source: pinned anthropic-sdk-go v1.50.1 message.go:6670-6861.
func toAnthropicChoice(choice ToolChoice) anthropic.ToolChoiceUnionParam {
    switch choice.Mode {
    case ToolChoiceAuto:
        return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}
    case ToolChoiceNone:
        none := anthropic.NewToolChoiceNoneParam()
        return anthropic.ToolChoiceUnionParam{OfNone: &none}
    case ToolChoiceRequired:
        return anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}
    case ToolChoiceFunction:
        return anthropic.ToolChoiceParamOfTool(choice.Name)
    default: // omitted
        return anthropic.ToolChoiceUnionParam{}
    }
}
```

The exact SDK union fields are `OfAuto`, `OfAny`, `OfTool`, and `OfNone`; the exact serialized discriminators are `"auto"`, `"any"`, `"tool"`, and `"none"`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6670-6861]`

### Anthropic Opaque Schema Preservation

```go
// Source: pinned anthropic-sdk-go v1.50.1 message.go:6632-6652.
func toAnthropicSchema(parameters any) (anthropic.ToolInputSchemaParam, error) {
    encoded, err := json.Marshal(parameters)
    if err != nil {
        return anthropic.ToolInputSchemaParam{}, err
    }

    var schema anthropic.ToolInputSchemaParam
    if err := json.Unmarshal(encoded, &schema); err != nil {
        return anthropic.ToolInputSchemaParam{}, err
    }
    return schema, nil
}
```

The SDK type’s exact extensibility field is `ExtraFields map[string]any`, and its marshaler calls `param.MarshalWithExtras`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/message.go:6632-6652]` This pattern must have a regression fixture containing keywords beyond `properties` and `required` before it is trusted.

### Gemini Choice Mapping

```go
// Source: pinned google.golang.org/genai v1.60.0 types.go:314-335,2592-2616.
func toGeminiToolConfig(choice ToolChoice) *genai.ToolConfig {
    config := &genai.FunctionCallingConfig{}
    switch choice.Mode {
    case ToolChoiceAuto:
        config.Mode = genai.FunctionCallingConfigModeAuto
    case ToolChoiceNone:
        config.Mode = genai.FunctionCallingConfigModeNone
    case ToolChoiceRequired:
        config.Mode = genai.FunctionCallingConfigModeAny
    case ToolChoiceFunction:
        config.Mode = genai.FunctionCallingConfigModeAny
        config.AllowedFunctionNames = []string{choice.Name}
    default: // omitted
        return nil
    }
    return &genai.ToolConfig{FunctionCallingConfig: config}
}
```

The exact SDK enum values are `"AUTO"`, `"ANY"`, and `"NONE"`; the exact restriction field is `AllowedFunctionNames []string`. `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:314-335,2592-2616]`

### Stream Final Validation

```go
// Recommended shared verifier boundary; json.Valid is from the Go standard library.
func finalizeCall(state CallStreamState) error {
    if state.ID == "" || state.Name == "" || state.Complete {
        return errInvalidProviderToolCall
    }
    if !json.Valid(state.Arguments) {
        return errInvalidProviderToolArguments
    }
    return nil
}
```

The required terminal checks are nonempty stable identity, legal transition, and valid assembled JSON; malformed final JSON is a provider bad response. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:33-38,52-58]` Error symbol names remain Agent Discretion.

## State of the Art

| Older / Current Local Approach | Required Current Approach | Evidence / Impact |
|---|---|---|
| OpenAI-compatible raw `tool_choice any` passthrough | Normalize the closed five-state public contract before routing | Current official Chat Completions supports modern tools/tool choice, while D-01/D-03 intentionally restrict the gateway subset. `[CITED: https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create]` |
| Anthropic assistant messages represented only as text | Mixed content blocks containing text and `tool_use`; user `tool_result` continuation | Official Anthropic tool-use contract. `[CITED: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use]` |
| Anthropic stream forwarded without block finalization | Accumulate `partial_json` by content-block index and parse at block stop | Official streaming contract. `[CITED: https://platform.claude.com/docs/en/build-with-claude/streaming]` |
| Gemini generated local ordinal IDs and omitted tool config | Preserve function-call IDs, emit function responses, and map `AUTO/ANY/NONE` plus allowed names | Official function-calling/API docs and pinned SDK types. `[CITED: https://ai.google.dev/gemini-api/docs/function-calling]` `[VERIFIED: C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/types.go:1260-1371]` |
| Tool stream bypass detected after response-rule buffering | Protocol-aware early bypass before any buffering | D-22/D-28 make immediate emission and no semantic rewriting mandatory. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:45-55]` |

**Deprecated/outdated for this phase:**

- Legacy OpenAI `functions`/`function_call` fields are intentionally unsupported. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:20-23]`
- The prior roadmap report is historical input only; current checkout and current official docs supersede it where they differ. `[VERIFIED: Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md:1-20]`

## Assumptions Log (RESOLVED/LOCKED)

| # | Resolved / locked claim | Section | Execution rule |
|---|---|---|---|
| A1 | **LOCKED:** Tool-protocol traffic bypasses semantic-cache reads and writes; Phase 28 does not make the cache tool-aware. | Architecture / Streaming | Enforce the narrow protocol isolation guard; do not expand cache scope. |
| A2 | **LOCKED:** Model/choice capability uses exact model overrides and fail-closed routing; unverified model/mode combinations are not qualified. | Routing | Unknown or unverified support is ineligible and explicit selection fails before upstream I/O. |
| A3 | **LOCKED:** Tenant-defined tool names are sensitive/high-cardinality and are excluded from logs, metrics, traces, and error messages. | Security and Logging | Retain names only in public protocol fields and provider wire mappings. |
| A4 | **LOCKED:** The final full-suite gate reuses the Phase 27 provisioned environment; if it is unavailable, the Phase 28 gate fails and records the missing dependency. | Environment / Validation | Focused tests may run locally, but local success cannot substitute for the final gate. |

## Open Questions (RESOLVED)

1. **Configured provider/model capability policy — RESOLVED.**
   - Decision: use exact model overrides for choice-mode support, with no model-name inference. Routing is fail-closed: every requested model/choice-mode combination must be verified; an unverified mode is ineligible, and explicit selection fails with a stable 400 before upstream I/O.
   - Basis: the repository currently copies adapter-wide capabilities to every model, while D-17 requires minimal model-level granularity. `[VERIFIED: internal/providers/catalog.go:54-66]` `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:37-38]`

2. **Operational treatment of tenant-defined tool names — RESOLVED.**
   - Decision: classify tenant-defined tool names as sensitive/high-cardinality. They must not appear in logs, metrics, traces, or error messages; they remain only in public protocol fields and provider wire mappings.
   - Basis: raw arguments/results are already prohibited and high-cardinality/sensitive labels must be avoided. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:44-44]`

3. **Phase 27 provisioned environment for the final full-suite gate — RESOLVED.**
   - Decision: reuse the Phase 27 provisioned PostgreSQL/pgvector, Redis Stack/RediSearch, Qdrant, and scheduler environment for the Phase 28 full-suite gate. If it is unavailable, the phase gate fails and records the unavailable dependency; no local or partial result is promoted to a passing gate. Focused tests remain runnable locally.
   - Basis: Phase 27’s verification records a passing full suite after provisioning these dependencies. `[VERIFIED: .planning/phases/27-stream-terminal-settlement-consistency/27-VERIFICATION.md:114-132]`

## Environment Availability

| Dependency | Required By | Available | Version / State | Gate rule |
|---|---|---:|---|---|
| Go | Build and all automated tests | Yes | `go1.26.1 windows/amd64` `[VERIFIED: current-session go version output]` | None needed |
| Go module cache | Pinned Anthropic/Gemini SDK type inspection and builds | Yes | Anthropic `v1.50.1`; Gemini `v1.60.0` `[VERIFIED: current-session module-cache inspection]` | `go mod download` in a network-enabled environment |
| Provider credentials | Optional live smoke only | Not verified | Not inspected to avoid secret exposure | Deterministic mock-server contract tests; live smoke remains optional under D-29 |
| PostgreSQL/pgvector | Full repository integration suite | No in current local run | Configured dependency host was unreachable. `[VERIFIED: current-session go test output]` | Reuse the Phase 27 provisioned environment; if unavailable, record gate failure |
| Redis Stack/RediSearch | Full repository integration suite | No in current local run | Configured dependency host was unreachable. `[VERIFIED: current-session go test output]` | Reuse the Phase 27 provisioned environment; if unavailable, record gate failure |
| Qdrant | Full repository integration suite | No in current local run | Configured dependency host was unreachable. `[VERIFIED: current-session go test output]` | Reuse the Phase 27 provisioned environment; if unavailable, record gate failure |
| Scheduler Python/ONNX worker | Full repository integration suite | No in current local run | Worker smoke signal unavailable. `[VERIFIED: current-session go test output]` | Reuse the Phase 27 provisioned environment; if unavailable, record gate failure |

**Full-suite gate rule:** PostgreSQL/pgvector, Redis Stack/RediSearch, Qdrant, and the scheduler worker are required for the final full-suite gate and must use the established Phase 27 provisioned-environment path. If that environment is unavailable, the Phase 28 gate fails and records the missing dependency; focused protocol tests remain runnable locally but cannot substitute for the gate. `[VERIFIED: .planning/phases/27-stream-terminal-settlement-consistency/27-VERIFICATION.md:126-132]`

## Validation Architecture

### Test Framework

| Property | Value |
|---|---|
| Framework | Go `testing` with existing package tests, `httptest` provider fixtures, and integration tests |
| Config file | None; commands are package-scoped `go test` invocations |
| Quick run command | `GOCACHE="$PWD/.tmp/go-build" go test -timeout 60s ./internal/llm ./internal/http/handlers ./internal/providers/adaptertest ./internal/providers/openai ./internal/providers/anthropic ./internal/providers/gemini ./internal/routing ./internal/gateway` |
| Integration command | `GOCACHE="$PWD/.tmp/go-build" go test -timeout 60s ./tests/integration -run "Test(ChatCompletionsStream|ChatCompletions|OpenAI|Tool)"` |
| Full suite command | `GOCACHE="$PWD/.tmp/go-build" go test -timeout 60s ./...` in the provisioned dependency environment |

The current focused package command passed in this session, and the named chat integration command passed in `0.676s`. `[VERIFIED: current-session go test output]` The current local full suite completed within the mandatory 60-second timeout but failed on unavailable PostgreSQL/pgvector, Redis Stack, Qdrant, and scheduler-worker dependencies rather than on the focused tool-adjacent packages. This is recorded as an unavailable final-gate prerequisite, not as a passing Phase 28 gate. `[VERIFIED: current-session go test output]`

### Phase Requirements -> Test Map

| Req ID / Decision Set | Behavior | Test Type | Automated Command | File Exists? |
|---|---|---|---|---|
| TOOL-F01; D-01-D-08 | Boundary accepts only modern function tools, strong choices, and valid call/result histories | Unit + HTTP contract | `go test -timeout 60s ./internal/llm ./internal/http/handlers -run Tool` | No - Wave 0 files recommended |
| TOOL-F01; D-09-D-14 | IDs, indexes, names, arguments, parallel calls, and state transitions remain stable | Unit state-machine | `go test -timeout 60s ./internal/providers/toolstream -run Tool` | No - Wave 0 package recommended |
| TOOL-F01; D-15-D-16 | All three providers map definitions, all choices, history/results, complete responses, and streams | Shared adapter contract + provider unit | `go test -timeout 60s ./internal/providers/adaptertest ./internal/providers/openai ./internal/providers/anthropic ./internal/providers/gemini -run Tool` | Partial harness exists; tool matrix absent `[VERIFIED: internal/providers/adaptertest/harness.go:13-166]` |
| TOOL-F01; D-17-D-20 | Model/mode capability preflight, override failure, round-robin safety, and Fusion rejection occur before I/O | Router unit + gateway integration | `go test -timeout 60s ./internal/routing ./internal/gateway -run "Tool|Fusion|Override|RoundRobin"` | Existing suites; scenarios absent |
| TOOL-F01; D-21 | Errors/logs/metrics expose stable low-cardinality metadata only | Unit + captured log/metric assertions | `go test -timeout 60s ./internal/gateway ./internal/observability -run Tool` | No tool-specific assertions |
| TOOL-F01; D-22-D-28 | Fragments emit immediately; malformed streams fail once; response rules do not rewrite/buffer; Phase 27 finalizer remains sole authority | Stream unit + gateway integration | `go test -timeout 60s ./internal/providers/... ./internal/gateway -run "Tool|Terminal|ResponseRule"` | Phase 27 terminal tests exist; tool cases absent `[VERIFIED: internal/gateway/stream_terminal_test.go:81-180]` |
| TOOL-F01; D-29-D-33 | Deterministic contract matrix is merge gate; focused then full suite; no new I/O/buffering/cardinality regression | Package + integration + static review | quick, integration, then full-suite commands above | Wave 0 additions required |

### Shared Contract Matrix

The adapter harness should define provider-neutral scenarios once, then allow only explicitly documented wire-shape assertions per adapter. D-30 requires the shared matrix. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:58-58]`

Required scenarios:

1. Definitions with opaque nested schema keywords survive mapping.
2. Omitted, `auto`, `none`, `required`, and specified-function choices map faithfully.
3. Assistant call history plus one and multiple out-of-order tool-result continuations map correctly.
4. Non-stream calls preserve upstream IDs, generate only missing IDs, and normalize `tool_calls` finish state.
5. Streaming single and interleaved calls emit first identity fragments immediately and argument fragments in order.
6. Duplicate/unknown IDs, name/index drift, invalid final JSON, post-completion fragments, and finish conflicts become `provider_bad_response`.
7. Pre-visible malformed streams may use existing safe fallback; post-visible failures never switch provider.
8. Response-rule and semantic-cache guards leave arguments/results byte-for-byte untouched and do not buffer tool fragments.
9. Terminal error/success races still execute the Phase 27 lifecycle exactly once.

### Sampling Rate

- **Per task commit:** run the smallest affected package command with `-timeout 60s`.
- **Per wave merge:** run the focused package command plus named integration command.
- **Phase gate:** provision dependencies and require `go test -timeout 60s ./...` green before `$gsd-verify-work`.
- **Optional provider smoke:** one non-stream and one stream case per configured provider/model, never a merge gate. This is locked by D-29. `[VERIFIED: .planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md:57-57]`

### Wave 0 Gaps

- [ ] `internal/llm/tool_protocol_test.go` - boundary choice and conversation-state matrix.
- [ ] `internal/providers/toolstream/state_test.go` - shared identity/argument/terminal state machine.
- [ ] `internal/providers/adaptertest/tool_contract.go` - reusable provider-neutral scenarios.
- [ ] Provider-local tool contract tests for OpenAI-compatible, Anthropic, and Gemini wire mapping.
- [ ] `internal/routing/router_tool_test.go` - capability, override, round-robin, and Fusion preflight.
- [ ] `internal/gateway/service_tool_test.go` and `service_stream_tool_test.go` - cache/rules/fallback/finalizer integration.
- [ ] `internal/http/handlers/chat_tool_test.go` and `tests/integration/chat_tools_test.go` - public HTTP/SSE contract.

These are recommended new test locations; names may be merged into existing files if doing so preserves the repository’s 500-line hard limit.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---|---:|---|
| V2 Authentication | No new Phase 28 behavior | Preserve existing gateway authentication; tool payloads do not create a new identity mechanism. |
| V3 Session Management | No new Phase 28 behavior | Tool-call IDs are protocol correlation identifiers, not sessions or authorization tokens. |
| V4 Access Control | Indirect | Provider/model eligibility and explicit override restrictions are enforced before upstream I/O. |
| V5 Validation, Sanitization and Encoding | Yes | Structural boundary validation, valid JSON argument strings, legal turn sequencing, and safe JSON/SSE serialization. |
| V6 Stored Cryptography | Limited | Use existing cryptographically suitable UUID generation for opaque missing IDs; do not invent content-derived identifiers. |
| V7 Error Handling and Logging | Yes | Stable error codes, no raw arguments/results, no high-cardinality identifiers or tool names. |
| V13 API and Web Service | Yes | Reject unsupported modes/topologies before I/O; treat upstream content as untrusted and validate normalized state. |

The ASVS project defines verification requirements for web application security controls. `[CITED: https://owasp.org/www-project-application-security-verification-standard/]` The table above is a Phase 28 applicability assessment, not a claim of full-project ASVS compliance.

### Known Threat Patterns for the Tool Protocol

| Pattern | STRIDE | Standard Mitigation |
|---|---|---|
| Malformed or deeply structured external JSON | Tampering / Denial of Service | Decode with bounded HTTP limits, validate envelope and final arguments, reject illegal state early. |
| Argument/result leakage through logs or labels | Information Disclosure | Never log raw schema/argument/result content or call IDs; keep metric dimensions bounded. |
| Call-ID collision or identity substitution | Spoofing / Tampering | Preserve upstream IDs, generate opaque collision-resistant missing IDs, reject duplicate/unknown/reused IDs. |
| Tool name drift across stream fragments | Tampering | Bind name to normalized index on first fragment and reject any drift. |
| Provider switching after visible fragments | Tampering / Repudiation | Disable fallback after first visible event; finalize the current provider error exactly once. |
| Gateway accidentally executing tool instructions | Elevation of Privilege | No execution, registry lookup, URL follow, MCP dispatch, or agent orchestration in Phase 28. |

## Recommended Plan Decomposition

### Plan 28-01: Normalized Public Contract and Boundary Validation

- Add strong internal choice and protocol-requirement types.
- Validate tools, choice, assistant calls, result sequencing, duplicates, names, and JSON argument strings.
- Add handler/LLM Wave 0 tests first.
- Covers D-01 through D-08 and creates the common predicate used downstream.

### Plan 28-02: Capability and Routing Preflight

- Extend capability data with choice-mode support and conservative model overrides.
- Enforce ordinary, override, round-robin, capacity-auto, and Fusion behavior before adapter I/O.
- Add stable `unsupported_tool_calling` and `unsupported_tool_choice` errors.
- Covers D-16 through D-20.

### Plan 28-03: Shared Stream State Machine and Contract Harness

- Build the immutable per-index verifier and injected missing-ID generator.
- Create the provider-neutral request/response/stream/error matrix.
- Keep this plan independent of provider wire details so all adapters use the same invariants.
- Covers D-09 through D-14 and D-30.

### Plan 28-04: OpenAI-Compatible Adapter Completion

- Apply validated request choice/definitions/history/results without semantic changes.
- Normalize and verify complete and streaming tool calls, final JSON, and finish reasons.
- Prove direct compatibility with public HTTP/SSE serialization.
- Covers the OpenAI portions of D-15, D-23 through D-26.

### Plan 28-05: Anthropic Adapter Completion

- Preserve complete schemas, map all choices, mixed assistant `tool_use` history, and `tool_result` continuation.
- Wire block-index streaming into the shared verifier and parse `partial_json` at block completion.
- Covers the Anthropic portions of D-15, D-23 through D-26.

### Plan 28-06: Gemini Adapter Completion

- Map all choices through `ToolConfig`, represent function-call history/function responses, and preserve provider IDs.
- Emit complete FunctionCall parts immediately through the shared normalizer; generate IDs only when absent.
- Normalize function-call terminal semantics to public `tool_calls`.
- Covers the Gemini portions of D-15, D-23 through D-26.

### Plan 28-07: Gateway Integration, Observability, and Phase Gate

- Add early response-rule and narrow semantic-cache bypasses for protocol traffic.
- Route adapter protocol failures through existing fallback visibility rules and `terminalFinalizer`.
- Add safe logs/metrics, full integration cases, static no-I/O/no-unbounded-buffer checks, and provisioned full-suite verification.
- Covers D-19, D-21, D-22, D-27 through D-33.

Plans 28-01 through 28-03 establish shared contracts; plans 28-04 through 28-06 can then execute in parallel; plan 28-07 integrates and gates the phase. This dependency order prevents adapters from inventing incompatible intermediate types.

## Sources

### Primary: Current Checkout and Pinned Source

- `internal/llm/types.go` - current public-neutral request, response, tool-call, and stream-event types.
- `internal/http/handlers/chat.go` and `chat_stream.go` - current HTTP validation and SSE serialization.
- `internal/providers/{openai,anthropic,gemini}/adapter.go` - current provider mapping and stream behavior.
- `internal/providers/capabilities.go`, `catalog.go`, and `internal/routing/router.go` - current capability and selection behavior.
- `internal/gateway/service.go`, `service_stream.go`, and `service_stream_terminal.go` - current cache, rules, fallback, and terminal flow.
- Pinned Anthropic SDK `v1.50.1` and Gemini SDK `v1.60.0` module-cache source - actual serialization/types used by this checkout.

### Primary: Official Protocol Documentation

- OpenAI Chat Completions create reference: https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create
- OpenAI function calling guide: https://developers.openai.com/api/docs/guides/function-calling
- OpenAI Chat Completions streaming events: https://developers.openai.com/api/reference/resources/chat/subresources/completions/streaming-events
- Anthropic tool-use implementation: https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use
- Anthropic streaming messages: https://platform.claude.com/docs/en/build-with-claude/streaming
- Gemini function calling: https://ai.google.dev/gemini-api/docs/function-calling
- Gemini GenerateContent API: https://ai.google.dev/api/generate-content
- OWASP ASVS: https://owasp.org/www-project-application-security-verification-standard/

### Secondary

- None. External protocol claims in this document use official provider documentation only.

### Tertiary

- None. The resolved/locked execution assumptions are isolated in the Assumptions Log.

## Metadata

**Confidence breakdown:**

- Standard stack: HIGH - exact versions and serialization types were read from `go.mod` and the installed pinned module source.
- Current gap inventory: HIGH - current symbols and tests were inspected after CodeGraph discovery.
- External protocol mapping: MEDIUM - current official provider documentation was consulted, but live configured-model behavior remains unverified.
- Architecture: HIGH - it follows D-01 through D-33 and existing ownership boundaries.
- Pitfalls: HIGH - each principal pitfall is tied to current source or a locked decision.
- Environment/full-suite status: HIGH for this session; availability can change between planning and execution.

**Research date:** 2026-09-22

**Valid until:** 2026-10-06 for external provider protocol details; recheck official documentation and configured model capability before execution under D-33.
