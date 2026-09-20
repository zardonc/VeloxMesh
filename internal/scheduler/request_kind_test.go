package scheduler

import (
	"testing"

	"veloxmesh/internal/llm"
)

func TestClassifyRequestKindBoundedValues(t *testing.T) {
	tests := map[RequestKind]*llm.LLMRequest{
		RequestKindSimpleQA:         {Messages: []llm.Message{{Content: "What is REST?"}}},
		RequestKindCodeGen:          {Messages: []llm.Message{{Content: "write code with ```go"}}},
		RequestKindCodeReview:       {Messages: []llm.Message{{Content: "review this code for bugs"}}},
		RequestKindSummarization:    {Messages: []llm.Message{{Content: "summarize this article"}}},
		RequestKindTranslation:      {Messages: []llm.Message{{Content: "translate this to French"}}},
		RequestKindStructuredOutput: {Messages: []llm.Message{{Content: "return JSON schema"}}},
		RequestKindMultiStep:        {Messages: []llm.Message{{Content: "analyze step by step"}}},
		RequestKindToolCall:         {Tools: []llm.Tool{{Type: llm.ToolTypeFunction}}},
		RequestKindRAG:              {Messages: []llm.Message{{Content: "use retrieved_context sources:"}}},
		RequestKindCreative:         {Messages: []llm.Message{{Content: "write a creative story"}}},
	}
	for want, req := range tests {
		if got := ClassifyRequestKind(req); got != want {
			t.Fatalf("ClassifyRequestKind=%s, want %s", got, want)
		}
	}
}
