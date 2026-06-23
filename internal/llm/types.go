package llm

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
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
	Role    Role   `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
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
