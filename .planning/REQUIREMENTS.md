# Milestone v5 Requirements

## Core Objective
Implement advanced LLM capabilities (Tool Calling, Multimodal), and introduce the "Combo" routing feature for combining multiple models. Pre-design the architecture to support future pluggable heuristic rules.

## Phase 5: Tool/Function Calling and Multimodal Capabilities
- **Tool Calling**: Support OpenAI-compatible `tools` and `tool_choice` parameters, mapping them to Anthropic/Gemini native formats.
- **Multimodal Support**: Support image/audio input formats, normalizing them for specific provider adapters.
- **Architecture Requirement**: Ensure that the architecture modifications pre-allocate pluggable extension points for future "heuristic rules" (e.g., prompt compression, input/output processing) to avoid large-scale refactoring later.

## Phase 6: Model Combo Feature
- **Custom Combos**: Users can define a "Combo" consisting of multiple provider models.
- **Combo as Model**: A Combo name can be used interchangeably with a model name in API requests.
- **Scheduling Strategies**:
  - **RR (Round Robin)**: Route requests sequentially across eligible models in the combo.
  - **Fusion**: Call multiple models simultaneously and return a synthesized or "best" result.
- **Capability-Based Routing**: 
  - If a request includes specific requirements (e.g., multimodal input) that some models in the combo do not support, the gateway must automatically filter and route only to the models capable of handling the request.

## Future / Long-Term Backlog
- Phase 7: Observability & Telemetry (OpenTelemetry, Prometheus metrics, Tracing)
- Phase 8: Multi-tenancy and Advanced Auth (API keys, Org-level quotas, Consumer management)
- Phase 9: Admin Console UI (Frontend for the existing backend Admin APIs)
- Long Term: User-configurable Pluggable Heuristic Rules System
