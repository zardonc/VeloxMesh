# Phase 05 Discussion Log

## Discussed Areas

### 工具调用 (Tool Calling) 的流式传输处理
- **Options presented:**
  - 网关负责聚合和转换
  - 透传底层原始流
- **User selected:** 网关负责聚合和转换

### 多模态大文件处理策略
- **Options presented:**
  - 作为不透明载荷直接透传
  - 在网关层解码并校验
- **User selected:** 作为不透明载荷直接透传

### “启发式规则”的预留架构模式
- **Options presented:**
  - 显式的职责链模式 (Chain of Responsibility / Pipeline)
  - 洋葱模型 (HTTP Middleware)
- **User selected:** 显式的职责链模式 (Chain of Responsibility / Pipeline)

### 工具定义的内部表示 (Internal Representation)
- **Options presented:**
  - 强类型 Go 结构体 (`llm.Tool`)
  - 弱类型 JSON/Map 传递
- **User selected:** 强类型 Go 结构体 (`llm.Tool`)
