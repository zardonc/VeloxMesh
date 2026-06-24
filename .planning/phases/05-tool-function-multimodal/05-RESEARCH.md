# Phase 05 Research: Tool/Function Calling and Multimodal Capabilities

## Domain
Implement advanced LLM capabilities (Tool Calling, Multimodal), and pre-design architecture for future heuristic rules (Chain of Responsibility).

## Current Architecture Assessment
- **`llm/types.go`**: Currently strictly text-based. `Message.Content` is a `string`. `LLMRequest` lacks `Tools` and `ToolChoice`. Responses and streaming Deltas do not have ToolCall support.
- **`gateway/service.go`**: Routing and health-check loops are implemented. Admission controller, cache, and semantic caching are in the path. No pipeline pattern exists for payload manipulation.
- **`providers/adapter.go`**: Defines `Complete` and `Stream`. Adapters take `llm.LLMRequest` and return `llm.LLMResponse` or a stream channel.

## Proposed Implementation Details

### 1. Types Expansion (`llm/types.go`)
- **Tools**: Add `Tool` and `Function` structs. Add `Tools []Tool` and `ToolChoice any` to `LLMRequest`.
- **Messages**: 
  - Add `ToolCalls []ToolCall` and `ToolCallID string` to `Message`.
  - Modify `Content` to support multimodal. Since Go is statically typed and OpenAI allows `content` to be `string` or `[]object`, we can change `Content` to `any` and handle parsing in the HTTP handler, or keep `Content string` and add `MultiContent []ContentPart` where `ContentPart` has `Type` ("text" or "image_url") and `ImageURL` struct.
- **Streaming**: Add `ToolCalls []ToolCallChunk` to `Delta` to support streaming tool calls.

### 2. Pluggable Rules Architecture (Pipeline)
- Create a new package `internal/pipeline`.
- Define a `Chain` or `Pipeline` that holds a list of `Rule` implementations.
- A `Rule` interface should process `*llm.LLMRequest` and `*llm.LLMResponse` (or have Pre/Post methods).
- In `gateway.Service.HandleChatCompletion` and `HandleChatCompletionStream`, insert a call to `Pipeline.ExecutePre(ctx, req)` before adapter selection, and `Pipeline.ExecutePost(ctx, resp)` after.

### 3. Adapters (OpenAI, Anthropic, Gemini)
- **Tool Calling**:
  - Anthropic uses `tools` array and `tool_choice`, but the structures differ slightly. The adapter must translate `llm.Tool` to Anthropic's format.
  - Gemini uses `tools` with `functionDeclarations`.
- **Streaming Tool Calls**:
  - The decision in `05-CONTEXT.md` explicitly mandates that the Gateway must aggregate and translate different Provider tool chunks into the standard OpenAI chunk format.
  - The Anthropic/Gemini adapters' `Stream` method will need stateful chunk processors to buffer function names/arguments and yield OpenAI-compatible `Delta.ToolCalls`.
- **Multimodal (Pass-through)**:
  - Base64 images are not decoded or validated by the gateway. The HTTP handler parses JSON as-is, stores it in `MultiContent`, and passes it to adapters.
  - Adapters map `MultiContent` to the upstream provider's image message format (e.g., Anthropic's `type: "image", source: {...}`).

## Required Files for Phase 5
1. `internal/llm/types.go` (Modify: Add Tool/Multimodal structs)
2. `internal/pipeline/pipeline.go` (New: Chain of responsibility pattern)
3. `internal/gateway/service.go` (Modify: Integrate pipeline)
4. `internal/http/handlers.go` or equivalent (Modify: Parse new payload fields)
5. `internal/providers/openai/...` (Modify: Map tools/multimodal)
6. `internal/providers/anthropic/...` (Modify: Map tools/multimodal, handle stream aggregation)
7. `internal/providers/gemini/...` (Modify: Map tools/multimodal, handle stream aggregation)
