# Phase 05 Context

## Domain
Implement advanced LLM capabilities (Tool Calling, Multimodal), and pre-design architecture for future heuristic rules.

## Decisions

### Tool Calling Streaming
- **Decision:** 网关负责聚合和转换。必须将不同 Provider 的工具调用 Chunk 统一转换为标准的 OpenAI 格式流返回。
- **Rationale:** 保证下游客户端拥有强一致性的 OpenAI 协议体验，网关承担适配复杂度的代价。

### Multimodal Payload Strategy
- **Decision:** 作为不透明载荷直接透传。网关不解码、不校验 Base64 图片/音频数据。
- **Rationale:** 最大程度降低网关层的 CPU 和内存开销，校验责任由底层 Provider 承担。

### Pluggable Rules Architecture
- **Decision:** 采用显式的职责链模式 (Chain of Responsibility / Pipeline)。
- **Rationale:** 构建独立于 HTTP 生命周期的针对 LLM 请求体/响应体的 Pipeline（如 `Pipeline.Execute(Payload)`），这为将来“启发式规则”对 Payload 内容做深度操作（如脱敏、压缩）预留了更好的切入点。

### Tool Internal Representation
- **Decision:** 采用强类型 Go 结构体 (`llm.Tool`)。
- **Rationale:** 在网关层定义标准的强类型内部表示，不仅方便各 Adapter 之间统一转换，也为未来的 Pipeline 规则读取、修改工具定义奠定基础。

## Canonical Refs
- `.planning/PROJECT.md`
- `.planning/REQUIREMENTS.md`
