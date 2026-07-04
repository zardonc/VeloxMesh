package llm

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ContentType string

const (
	ContentTypeText     ContentType = "text"
	ContentTypeImageURL ContentType = "image_url"
)

type ContentPart struct {
	Type     ContentType `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *ImageURL   `json:"image_url,omitempty"`
}

type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type Message struct {
	Role         Role          `json:"role"`
	Content      string        `json:"content,omitempty"`
	MultiContent []ContentPart `json:"-"` // Handled via custom marshaling/unmarshaling in the handler if needed, or mapped accordingly. Wait, the plan says 'support multimodal content (MultiContent []ContentPart)'
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
}

type ToolType string

const (
	ToolTypeFunction ToolType = "function"
)

type Function struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"` // Usually JSON Schema
}

type Tool struct {
	Type     ToolType  `json:"type"`
	Function *Function `json:"function,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     ToolType     `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"` // JSON string
}

type ToolCallChunk struct {
	Index    *int               `json:"index,omitempty"`
	ID       *string            `json:"id,omitempty"`
	Type     *ToolType          `json:"type,omitempty"`
	Function *FunctionCallChunk `json:"function,omitempty"`
}

type FunctionCallChunk struct {
	Name      *string `json:"name,omitempty"`
	Arguments *string `json:"arguments,omitempty"`
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
}

type LLMRequest struct {
	RequestID     string
	Model         string
	Messages      []Message
	Temperature   *float64
	MaxTokens     *int
	Stream        bool
	PriorityClass string
	RouteOverride string
	Tools         []Tool
	ToolChoice    any
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Delta struct {
	Role      Role            `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []ToolCallChunk `json:"tool_calls,omitempty"`
}

type ChunkChoice struct {
	Index        int     `json:"index"`
	Delta        Delta   `json:"delta"`
	FinishReason *string `json:"finish_reason,omitempty"`
}

type ChatCompletionChunkResponse struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

type LLMResponse struct {
	GatewayID    string
	Model        string
	Provider     string
	Strategy     string
	Choices      []Choice
	AttemptCount int
	FallbackUsed bool
	Usage        *Usage
	CacheHit     bool
	CacheLevel   string
	QueueWaitMs  int64
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type StreamEvent struct {
	DeltaContent string
	FinishReason string
	Done         bool
	Usage        *Usage
	Provider     string
	Model        string
	Error        error
	ToolCalls    []ToolCallChunk
}

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type Embedding struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

type EmbeddingResponse struct {
	Model string      `json:"model"`
	Data  []Embedding `json:"data"`
	Usage *Usage      `json:"usage,omitempty"`
}
