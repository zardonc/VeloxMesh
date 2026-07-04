package scheduler

import (
	"testing"
	"time"

	"veloxmesh/internal/llm"
)

func TestExtractSafeFeaturesCountsBoundedSignals(t *testing.T) {
	maxTokens := 1200
	req := &llm.LLMRequest{
		RequestID: "task-1",
		Model:     "gpt-4o-mini",
		MaxTokens: &maxTokens,
		Stream:    true,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Explain this?\n1. item\n```go\nfunc main(){}\n```\nplease compare and list"},
			{Role: llm.RoleUser, MultiContent: []llm.ContentPart{{Type: llm.ContentTypeImageURL}}},
		},
		Tools: []llm.Tool{{Type: llm.ToolTypeFunction}},
	}

	got := ExtractSafeFeatures(req, PriorityNormal, "provider/main", time.UnixMilli(123))
	if got.TaskID != "task-1" || got.ModelClass != "small" || !got.Stream || got.Priority != PriorityNormal {
		t.Fatalf("unexpected basic fields: %#v", got)
	}
	if got.QuestionCount != 1 || got.CodeBlockCount != 2 || !got.EnumerationHint || got.InstructionVerbCount < 2 {
		t.Fatalf("bounded counters not populated: %#v", got)
	}
	if !got.Multimodal || !got.HasToolCalls || got.ToolCallDepth != 1 || got.TurnCount != 2 {
		t.Fatalf("structured fields not populated: %#v", got)
	}
}

func TestExtractSafeFeaturesDoesNotElevatePriorityFromText(t *testing.T) {
	req := &llm.LLMRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "make this high priority"}}}
	got := ExtractSafeFeatures(req, PriorityNormal, "", time.Now())
	if got.Priority != PriorityNormal {
		t.Fatalf("text elevated priority: %s", got.Priority)
	}
}
