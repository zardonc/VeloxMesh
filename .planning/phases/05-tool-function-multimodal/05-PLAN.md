# Phase 05 Plan: Tool/Function Calling and Multimodal Capabilities

## Goal
Implement advanced LLM capabilities (Tool Calling, Multimodal), and pre-design architecture for future heuristic rules using an explicit Chain of Responsibility pattern.

## Context & Constraints
- **Tool Calling Streaming**: Gateway handles aggregation and converts to standard OpenAI format.
- **Multimodal Payload**: Pass-through opaque payloads without decoding/validating Base64 in the gateway.
- **Pluggable Rules Architecture**: Chain of Responsibility (Pipeline) pattern specifically for LLM payloads.
- **Tool Internal Representation**: Strongly typed Go structs (`llm.Tool`) in the routing layer.
- **Implementation Approach**: Favor borrowing types and logic from official provider SDKs (e.g., `sashabaranov/go-openai`, official Anthropic/Gemini SDKs) where applicable, rather than rewriting from scratch.

## Execution Steps

### 1. Extend LLM Types (`internal/llm/types.go`)
- [ ] Add `Function` and `Tool` structs (Borrow definitions from `go-openai` SDK).
- [ ] Add `Tools []Tool` and `ToolChoice any` to `LLMRequest`.
- [ ] Update `Message` to include `ToolCalls []ToolCall` and support multimodal content (`MultiContent []ContentPart`).
- [ ] Update `Delta` to include `ToolCalls []ToolCallChunk` for streaming.

### 2. Implement Pluggable Rules Pipeline (`internal/pipeline`)
- [ ] Create `internal/pipeline/pipeline.go`.
- [ ] Define `Rule` interface with `ProcessRequest(ctx, req)` and `ProcessResponse(ctx, resp)`.
- [ ] Define `Pipeline` struct that executes a chain of `Rule`s.
- [ ] Modify `gateway/service.go` to instantiate an empty `Pipeline` and execute it before sending the request to the adapter, and after receiving the response.

### 3. Update Adapters for Tool Calling & Multimodal
- [ ] **OpenAI Adapter** (`internal/providers/openai/adapter.go`): Map `llm.Tool` and multimodal content.
- [ ] **Anthropic Adapter** (`internal/providers/anthropic/adapter.go`): Translate `llm.Tool` to Anthropic format. Aggregate streaming tool chunks into standard OpenAI chunk format.
- [ ] **Gemini Adapter** (`internal/providers/gemini/adapter.go`): Translate `llm.Tool` to Gemini's `functionDeclarations`. Aggregate streaming tool chunks into standard OpenAI chunk format.

### 4. HTTP Handler Updates (`cmd/gateway/` or `internal/http/`)
- [ ] Update JSON unmarshaling in the chat completion handler to safely parse `content` as either a `string` or an array of objects (multimodal), mapping it to `llm.Message`.

## Verification Plan
- **Unit Tests**:
  - Test pipeline execution with mock rules.
  - Test Anthropic/Gemini streaming chunk aggregators to ensure they emit correct OpenAI tool chunks.
- **Integration**:
  - Send a tool-calling request to OpenAI, Anthropic, and Gemini through the gateway and verify valid responses.
  - Send a request with a Base64 image payload to an upstream provider and verify passthrough.
