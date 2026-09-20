package scheduler

import (
	"strings"

	"veloxmesh/internal/llm"
)

func ClassifyRequestKind(req *llm.LLMRequest) RequestKind {
	text := lowerText(req)
	if len(req.Tools) > 0 || strings.Contains(text, "tool") || strings.Contains(text, "function") {
		return RequestKindToolCall
	}
	if hasAny(text, "```", "func ", "class ", "import ", "package ", "write code", "implement") {
		return RequestKindCodeGen
	}
	if hasAny(text, "review this code", "find bug", "bug?", "code review") {
		return RequestKindCodeReview
	}
	if hasAny(text, "summarize", "summary", "tl;dr") {
		return RequestKindSummarization
	}
	if hasAny(text, "translate", "translation") {
		return RequestKindTranslation
	}
	if hasAny(text, "json", "yaml", "table", "schema", "structured") {
		return RequestKindStructuredOutput
	}
	if hasAny(text, "step by step", "reason through", "analyze") {
		return RequestKindMultiStep
	}
	if hasAny(text, "retrieved_context", "context:", "sources:") {
		return RequestKindRAG
	}
	if hasAny(text, "story", "poem", "creative") {
		return RequestKindCreative
	}
	return RequestKindSimpleQA
}

func lowerText(req *llm.LLMRequest) string {
	var b strings.Builder
	for _, msg := range req.Messages {
		b.WriteString(strings.ToLower(msg.Content))
		b.WriteByte(' ')
		for _, part := range msg.MultiContent {
			b.WriteString(strings.ToLower(part.Text))
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func hasAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
