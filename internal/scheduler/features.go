package scheduler

import (
	"math"
	"strings"
	"time"

	"veloxmesh/internal/llm"
)

func ExtractSafeFeatures(req *llm.LLMRequest, priority PriorityClass, routeHint string, enqueue time.Time) TaskFeature {
	text := lowerText(req)
	words := strings.Fields(text)
	return TaskFeature{
		TaskID:                   req.RequestID,
		ModelClass:               modelClass(req.Model),
		EstimatedInputTokens:     int64(len(words)),
		EstimatedOutputTokens:    estimatedOutput(req),
		Stream:                   req.Stream,
		Priority:                 priority,
		TimeoutClass:             timeoutClass(req),
		EnqueueTimeMs:            enqueue.UnixMilli(),
		RequestKind:              ClassifyRequestKind(req),
		RouteHint:                lowCardinality(routeHint),
		HasToolCalls:             len(req.Tools) > 0,
		ToolCallDepth:            toolDepth(req),
		TurnCount:                int32(len(req.Messages)),
		Multimodal:               hasMultimodal(req),
		QuestionCount:            int32(strings.Count(text, "?")),
		CodeBlockCount:           int32(strings.Count(text, "```")),
		EnumerationHint:          hasAny(text, "1.", "2.", "- ", "first", "second", "步骤", "列出"),
		InstructionVerbCount:     instructionVerbCount(text),
		MaxSentenceLengthBucket:  bucket(maxSentenceLength(words)),
		VocabularyRichnessBucket: bucket(vocabularyRichness(words)),
		ConfidenceHint:           1,
		UncertaintyHint:          0,
		CoverageLevel:            SemanticCoverageNone,
	}
}

func modelClass(model string) string {
	model = strings.ToLower(model)
	switch {
	case strings.Contains(model, "mini"), strings.Contains(model, "haiku"), strings.Contains(model, "flash"):
		return "small"
	case strings.Contains(model, "opus"), strings.Contains(model, "gpt-4"), strings.Contains(model, "pro"):
		return "large"
	default:
		return "standard"
	}
}

func estimatedOutput(req *llm.LLMRequest) int64 {
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		return int64(*req.MaxTokens)
	}
	return 256
}

func timeoutClass(req *llm.LLMRequest) string {
	if req.MaxTokens != nil && *req.MaxTokens > 1000 {
		return "long"
	}
	return "standard"
}

func lowCardinality(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ':' || r == '/' || r == '.' })
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	return parts[0]
}

func toolDepth(req *llm.LLMRequest) int32 {
	if len(req.Tools) == 0 {
		return 0
	}
	return 1
}

func hasMultimodal(req *llm.LLMRequest) bool {
	for _, msg := range req.Messages {
		for _, part := range msg.MultiContent {
			if part.Type == llm.ContentTypeImageURL {
				return true
			}
		}
	}
	return false
}

func instructionVerbCount(text string) int32 {
	var count int32
	for _, verb := range []string{"write", "explain", "compare", "list", "analyze", "summarize", "translate", "implement"} {
		count += int32(strings.Count(text, verb))
	}
	return count
}

func maxSentenceLength(words []string) int {
	maxLen, cur := 0, 0
	for _, word := range words {
		cur++
		for _, r := range word {
			if sentenceDelimiter(r) {
				maxLen = max(maxLen, cur)
				cur = 0
				break
			}
		}
	}
	return max(maxLen, cur)
}

func sentenceDelimiter(r rune) bool {
	return r == '.' || r == '?' || r == '!' || r == '。' || r == '？' || r == '！'
}

func vocabularyRichness(words []string) int {
	if len(words) == 0 {
		return 0
	}
	seen := map[string]struct{}{}
	for _, word := range words {
		seen[word] = struct{}{}
	}
	return int(math.Round(float64(len(seen)) / float64(len(words)) * 100))
}

func bucket(value int) int32 {
	switch {
	case value <= 5:
		return 1
	case value <= 20:
		return 2
	case value <= 50:
		return 3
	default:
		return 4
	}
}
