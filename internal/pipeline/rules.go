package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"veloxmesh/internal/llm"
	verrors "veloxmesh/internal/errors"
)

//
// 1. Filter Handler
//
type FilterHandler struct{}

func (h *FilterHandler) Name() RuleName { return RuleFilter }
func (h *FilterHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	action, _ := config.Options["request_action"].(string)
	if action == "reject" {
		return verrors.ErrPolicyBlocked
	}
	return nil
}
func (h *FilterHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	action, _ := config.Options["response_action"].(string)
	if action == "block" {
		return verrors.ErrPolicyBlocked
	}
	if action == "replace" {
		replacement, _ := config.Options["replacement"].(string)
		if len(resp.Choices) > 0 {
			resp.Choices[0].Message.Content = replacement
		} else {
			resp.Choices = append(resp.Choices, llm.Choice{Message: llm.Message{Role: llm.RoleAssistant, Content: replacement}})
		}
	}
	return nil
}

//
// 2. PII Handler
//
type PIIHandler struct{}

func (h *PIIHandler) Name() RuleName { return RulePII }

var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
var phoneRegex = regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`)

func (h *PIIHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	if state.PIIMappings == nil {
		state.PIIMappings = make(map[string]string)
	}

	for i, msg := range req.Messages {
		if msg.Role == "user" {
			text := msg.Content
			// Replace emails
			emails := emailRegex.FindAllString(text, -1)
			for j, email := range emails {
				placeholder := fmt.Sprintf("{{PII_EMAIL_%d}}", j)
				state.PIIMappings[placeholder] = email
				text = strings.ReplaceAll(text, email, placeholder)
			}
			
			// Replace phones
			phones := phoneRegex.FindAllString(text, -1)
			for j, phone := range phones {
				placeholder := fmt.Sprintf("{{PII_PHONE_%d}}", j)
				state.PIIMappings[placeholder] = phone
				text = strings.ReplaceAll(text, phone, placeholder)
			}
			req.Messages[i].Content = text
		}
	}
	return nil
}
func (h *PIIHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	if state.PIIMappings != nil && len(resp.Choices) > 0 {
		for placeholder, original := range state.PIIMappings {
			resp.Choices[0].Message.Content = strings.ReplaceAll(resp.Choices[0].Message.Content, placeholder, original)
		}
	}
	return nil
}

//
// 3. Rewrite Handler
//
type RewriteHandler struct{}

func (h *RewriteHandler) Name() RuleName { return RuleRewrite }
func (h *RewriteHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	prefix, _ := config.Options["prefix"].(string)
	suffix, _ := config.Options["suffix"].(string)
	
	var replacements map[string]string
	if reps, ok := config.Options["replacements"].(map[string]interface{}); ok {
		replacements = make(map[string]string)
		for k, v := range reps {
			if vs, ok := v.(string); ok {
				replacements[k] = vs
			}
		}
	}

	for i, msg := range req.Messages {
		if msg.Role == "user" {
			text := msg.Content
			for old, newStr := range replacements {
				text = strings.ReplaceAll(text, old, newStr)
			}
			if prefix != "" {
				text = prefix + text
			}
			if suffix != "" {
				text = text + suffix
			}
			req.Messages[i].Content = text
		}
	}
	return nil
}
func (h *RewriteHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	return nil
}

//
// 4. RTK Handler
//
type RTKHandler struct{}

func (h *RTKHandler) Name() RuleName { return RuleRTK }
func (h *RTKHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	maxPromptTokensRaw := config.Options["max_prompt_tokens"]
	if maxPromptTokensRaw == nil {
		return nil
	}
	var maxPromptTokens int
	switch v := maxPromptTokensRaw.(type) {
	case float64:
		maxPromptTokens = int(v)
	case int:
		maxPromptTokens = v
	default:
		return nil
	}

	// Simple token estimation: 1 word ~ 1.3 tokens
	estimateTokens := func(text string) int {
		words := len(strings.Fields(text))
		return int(float64(words) * 1.3)
	}

	totalTokens := 0
	for _, msg := range req.Messages {
		totalTokens += estimateTokens(msg.Content)
	}

	if totalTokens <= maxPromptTokens {
		return nil
	}

	// Needs truncation. Preserve system/tool messages and newest user message.
	// Truncate older user/assistant messages.
	// This is a naive implementation for the test.
	
	// Find the last user message
	lastUserIdx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	for i := 0; i < len(req.Messages); i++ {
		msg := req.Messages[i]
		if msg.Role == "system" || msg.Role == "tool" || i == lastUserIdx {
			continue // preserve
		}
		
		// If it's an older message, we just truncate it for simplicity if we are still over limit
		currentTotal := 0
		for _, m := range req.Messages {
			currentTotal += estimateTokens(m.Content)
		}
		
		if currentTotal > maxPromptTokens {
			req.Messages[i].Content = "[TRUNCATED BY RTK]"
		}
	}

	return nil
}
func (h *RTKHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	return nil
}

//
// 5. Headroom Handler
//
type HeadroomHandler struct{}

func (h *HeadroomHandler) Name() RuleName { return RuleHeadroom }
func (h *HeadroomHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	hrTokensRaw := config.Options["response_headroom_tokens"]
	cwTokensRaw := config.Options["context_window_tokens"]
	if hrTokensRaw == nil || cwTokensRaw == nil {
		return nil
	}
	
	var hrTokens, cwTokens int
	if v, ok := hrTokensRaw.(float64); ok { hrTokens = int(v) }
	if v, ok := hrTokensRaw.(int); ok { hrTokens = v }
	if v, ok := cwTokensRaw.(float64); ok { cwTokens = int(v) }
	if v, ok := cwTokensRaw.(int); ok { cwTokens = v }

	if hrTokens > 0 && cwTokens > hrTokens {
		val := cwTokens - hrTokens
		req.MaxTokens = &val
	}
	
	return nil
}
func (h *HeadroomHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	return nil
}

//
// 6. Caveman Handler
//
type CavemanHandler struct{}

func (h *CavemanHandler) Name() RuleName { return RuleCaveman }
func (h *CavemanHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	req.Messages = append([]llm.Message{
		{Role: "system", Content: "Speak like a caveman."},
	}, req.Messages...)

	rewrite, _ := config.Options["rewrite_request_text"].(bool)
	if rewrite {
		for i, msg := range req.Messages {
			if msg.Role == "user" {
				req.Messages[i].Content = strings.ToUpper(msg.Content) + " UGH!"
			}
		}
	}
	return nil
}
func (h *CavemanHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	if len(resp.Choices) > 0 {
		resp.Choices[0].Message.Content = strings.ToUpper(resp.Choices[0].Message.Content) + " UGH."
	}
	return nil
}

//
// 7. Ponytail Handler
//
type PonytailHandler struct{}

func (h *PonytailHandler) Name() RuleName { return RulePonytail }
func (h *PonytailHandler) ProcessRequest(ctx context.Context, scope RequestScope, state *RunState, req *llm.LLMRequest, config RuleConfig) error {
	req.Messages = append([]llm.Message{
		{Role: "system", Content: "Speak like a corporate manager."},
	}, req.Messages...)

	rewrite, _ := config.Options["rewrite_request_text"].(bool)
	if rewrite {
		for i, msg := range req.Messages {
			if msg.Role == "user" {
				req.Messages[i].Content = "Circling back to this: " + msg.Content
			}
		}
	}
	return nil
}
func (h *PonytailHandler) ProcessResponse(ctx context.Context, scope RequestScope, state *RunState, resp *llm.LLMResponse, config RuleConfig) error {
	if len(resp.Choices) > 0 {
		resp.Choices[0].Message.Content = "Synergistically speaking, " + resp.Choices[0].Message.Content
	}
	return nil
}

// RegisterAll registers all rule handlers into the given registry.
func RegisterAll(r *Registry) {
	r.Register(&FilterHandler{})
	r.Register(&PIIHandler{})
	r.Register(&RewriteHandler{})
	r.Register(&RTKHandler{})
	r.Register(&HeadroomHandler{})
	r.Register(&CavemanHandler{})
	r.Register(&PonytailHandler{})
}
