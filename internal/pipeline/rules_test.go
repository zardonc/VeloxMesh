package pipeline

import (
	"context"
	"strings"
	"testing"
	verrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

func TestPIIHandler(t *testing.T) {
	registry := NewRegistry()
	RegisterAll(registry)

	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RulePII] = RuleConfig{Enabled: true}
	p := New(registry, cfg)

	ctx := context.Background()
	scope := RequestScope{UserID: "u1"}
	state := &RunState{PIIMappings: make(map[string]string)}

	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "My email is test@example.com and phone is 555-123-4567."},
		},
	}

	err := p.ProcessRequest(ctx, scope, state, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reqText := req.Messages[0].Content
	if strings.Contains(reqText, "test@example.com") {
		t.Errorf("expected email to be redacted")
	}
	if strings.Contains(reqText, "555-123-4567") {
		t.Errorf("expected phone to be redacted")
	}
	if !strings.Contains(reqText, "{{PII_EMAIL_0}}") {
		t.Errorf("expected email placeholder")
	}

	resp := &llm.LLMResponse{
		Choices: []llm.Choice{
			{Message: llm.Message{Role: llm.RoleAssistant, Content: "I have recorded {{PII_EMAIL_0}} and {{PII_PHONE_0}}."}},
		},
	}
	err = p.ProcessResponse(ctx, scope, state, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(resp.Choices[0].Message.Content, "test@example.com") || !strings.Contains(resp.Choices[0].Message.Content, "555-123-4567") {
		t.Errorf("expected PII to be restored, got: %s", resp.Choices[0].Message.Content)
	}
}

func TestPIIHandlerUsesRequestScopedCounters(t *testing.T) {
	registry := NewRegistry()
	RegisterAll(registry)
	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RulePII] = RuleConfig{Enabled: true}
	p := New(registry, cfg)
	state := &RunState{}
	req := &llm.LLMRequest{Messages: []llm.Message{
		{Role: "user", Content: "First: first@example.com"},
		{Role: "user", Content: "Second: second@example.com"},
	}}

	if err := p.ProcessRequest(context.Background(), RequestScope{}, state, req); err != nil {
		t.Fatalf("ProcessRequest: %v", err)
	}
	if !strings.Contains(req.Messages[0].Content, "{{PII_EMAIL_0}}") || !strings.Contains(req.Messages[1].Content, "{{PII_EMAIL_1}}") {
		t.Fatalf("expected unique email placeholders, got %#v", req.Messages)
	}

	resp := &llm.LLMResponse{Choices: []llm.Choice{{Message: llm.Message{Content: "{{PII_EMAIL_0}} / {{PII_EMAIL_1}}"}}}}
	if err := p.ProcessResponse(context.Background(), RequestScope{}, state, resp); err != nil {
		t.Fatalf("ProcessResponse: %v", err)
	}
	if resp.Choices[0].Message.Content != "first@example.com / second@example.com" {
		t.Fatalf("unexpected restore: %q", resp.Choices[0].Message.Content)
	}
}

func TestRTKAndHeadroom(t *testing.T) {
	registry := NewRegistry()
	RegisterAll(registry)

	cfg := DefaultSemanticPipelineConfig()
	// Disabled at first
	p := New(registry, cfg)

	ctx := context.Background()
	scope := RequestScope{}
	state := &RunState{}
	req := &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Hello world this is a long message that might be truncated if RTK was enabled and the threshold was low."},
		},
	}

	_ = p.ProcessRequest(ctx, scope, state, req)
	if req.MaxTokens != nil {
		t.Errorf("expected MaxTokens to be unchanged when Headroom disabled")
	}
	if strings.Contains(req.Messages[0].Content, "TRUNCATED") {
		t.Errorf("expected no truncation when RTK disabled")
	}

	// Enable them
	cfg.Rules[RuleHeadroom] = RuleConfig{
		Enabled: true,
		Options: map[string]interface{}{
			"response_headroom_tokens": 1000,
			"context_window_tokens":    4000,
		},
	}
	cfg.Rules[RuleRTK] = RuleConfig{
		Enabled: true,
		Options: map[string]interface{}{
			"max_prompt_tokens": 5, // very low to force truncation
		},
	}
	p = New(registry, cfg)

	// Need multiple messages to test truncation logic (it preserves the newest user message)
	req = &llm.LLMRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Old message that should be truncated because it's too long."},
			{Role: "assistant", Content: "Okay"},
			{Role: "user", Content: "New message"}, // preserved
		},
	}
	_ = p.ProcessRequest(ctx, scope, state, req)

	if req.MaxTokens == nil || *req.MaxTokens != 3000 {
		t.Errorf("expected MaxTokens 3000, got %v", req.MaxTokens)
	}
	if !strings.Contains(req.Messages[0].Content, "TRUNCATED") {
		t.Errorf("expected older message to be truncated")
	}
	if req.Messages[2].Content != "New message" {
		t.Errorf("expected newest user message to be preserved")
	}
}

func TestFilter(t *testing.T) {
	registry := NewRegistry()
	RegisterAll(registry)

	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RuleFilter] = RuleConfig{
		Enabled: true,
		Options: map[string]interface{}{
			"request_action": "reject",
		},
	}
	p := New(registry, cfg)

	ctx := context.Background()
	err := p.ProcessRequest(ctx, RequestScope{}, &RunState{}, &llm.LLMRequest{})
	if err != verrors.ErrPolicyBlocked {
		t.Errorf("expected filter block on request, got %v", err)
	}

	cfg.Rules[RuleFilter] = RuleConfig{
		Enabled: true,
		Options: map[string]interface{}{
			"response_action": "replace",
			"replacement":     "blocked content",
		},
	}
	resp := &llm.LLMResponse{
		Choices: []llm.Choice{
			{Message: llm.Message{Content: "bad stuff"}},
		},
	}
	_ = p.ProcessResponse(ctx, RequestScope{}, &RunState{}, resp)
	if resp.Choices[0].Message.Content != "blocked content" {
		t.Errorf("expected response to be replaced, got %s", resp.Choices[0].Message.Content)
	}
}

func TestCavemanAndPonytail(t *testing.T) {
	registry := NewRegistry()
	RegisterAll(registry)

	ctx := context.Background()
	scope := RequestScope{}
	state := &RunState{}

	// Caveman tests
	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RuleCaveman] = RuleConfig{Enabled: true} // no rewrite_request_text
	cfg.Rules[RulePonytail] = RuleConfig{Enabled: false}
	p := New(registry, cfg)

	req := &llm.LLMRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	}
	_ = p.ProcessRequest(ctx, scope, state, req)
	if req.Messages[0].Role != "system" || !strings.Contains(req.Messages[0].Content, "caveman") {
		t.Errorf("expected caveman system prompt injected")
	}
	if req.Messages[1].Content != "hello" {
		t.Errorf("expected request text to NOT be rewritten, got %s", req.Messages[1].Content)
	}

	resp := &llm.LLMResponse{
		Choices: []llm.Choice{
			{Message: llm.Message{Content: "hello"}},
		},
	}
	_ = p.ProcessResponse(ctx, scope, state, resp)
	if !strings.Contains(resp.Choices[0].Message.Content, "UGH") {
		t.Errorf("expected response text to be rewritten, got %s", resp.Choices[0].Message.Content)
	}

	// Ponytail with rewrite_request_text
	cfg = DefaultSemanticPipelineConfig()
	cfg.Rules[RuleCaveman] = RuleConfig{Enabled: false}
	cfg.Rules[RulePonytail] = RuleConfig{
		Enabled: true,
		Options: map[string]interface{}{"rewrite_request_text": true},
	}
	p = New(registry, cfg)

	req = &llm.LLMRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	}
	_ = p.ProcessRequest(ctx, scope, state, req)
	if req.Messages[1].Content == "hello" || !strings.Contains(req.Messages[1].Content, "Circling back") {
		t.Errorf("expected request text to be rewritten, got %s", req.Messages[1].Content)
	}
}

func TestRewrite(t *testing.T) {
	registry := NewRegistry()
	RegisterAll(registry)

	cfg := DefaultSemanticPipelineConfig()
	cfg.Rules[RuleRewrite] = RuleConfig{
		Enabled: true,
		Options: map[string]interface{}{
			"prefix": "PREFIX ",
			"suffix": " SUFFIX",
			"replacements": map[string]interface{}{
				"bad": "good",
			},
		},
	}
	p := New(registry, cfg)
	req := &llm.LLMRequest{
		Messages: []llm.Message{{Role: "user", Content: "this is bad"}},
	}
	_ = p.ProcessRequest(context.Background(), RequestScope{}, &RunState{}, req)
	if req.Messages[0].Content != "PREFIX this is good SUFFIX" {
		t.Errorf("expected rewrites, got: %s", req.Messages[0].Content)
	}
}
