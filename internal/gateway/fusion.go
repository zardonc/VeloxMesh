package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

func (s *Service) executeFusion(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision) (*llm.LLMResponse, error) {
	if len(decision.FusionProviders) == 0 {
		return nil, errors.NewGatewayError("fusion_no_providers", "No healthy providers available for fusion", 503)
	}
	if decision.FusionJudge == "" {
		return nil, errors.NewGatewayError("fusion_no_judge", "Fusion strategy requires a judge model", 400)
	}

	start := time.Now()

	// 1. Run members in parallel
	var wg sync.WaitGroup
	results := make([]string, len(decision.FusionProviders))
	errs := make([]error, len(decision.FusionProviders))
	var promptTokens, completionTokens int

	var mu sync.Mutex

	for i, adapter := range decision.FusionProviders {
		wg.Add(1)
		go func(idx int, p providers.ProviderAdapter) {
			defer wg.Done()
			// Need a modified request for the member (non-streaming)
			memberReq := *req
			memberReq.Stream = false
			memberReq.Model = p.Models()[0] // use the adapter's first model or just leave it if adapter resolves it

			mResp, err := p.Complete(ctx, &memberReq)
			if err != nil {
				errs[idx] = err
				return
			}

			mu.Lock()
			if mResp.Usage != nil {
				promptTokens += mResp.Usage.PromptTokens
				completionTokens += mResp.Usage.CompletionTokens
			}
			if len(mResp.Choices) > 0 {
				results[idx] = mResp.Choices[0].Message.Content
			}
			mu.Unlock()
		}(i, adapter)
	}
	wg.Wait()

	// Gather successful results
	var validResults []string
	for i, res := range results {
		if errs[i] == nil && res != "" {
			validResults = append(validResults, fmt.Sprintf("Response %d:\n%s", i+1, res))
		}
	}

	if len(validResults) == 0 {
		return nil, errors.NewGatewayError("fusion_failed", "All fusion members failed to return a response", 502)
	}

	// 2. Run the judge
	judgePrompt := fmt.Sprintf("Synthesize the following responses into a single, high-quality answer. If there are contradictions, resolve them logically.\n\n%s", strings.Join(validResults, "\n\n---\n\n"))

	judgeReq := *req
	// replace the last user message or append a system message?
	// Let's create a new messages slice
	var judgeMsgs []llm.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			judgeMsgs = append(judgeMsgs, m)
		}
	}
	// append the user query
	originalQuery := ""
	if len(req.Messages) > 0 {
		originalQuery = req.Messages[len(req.Messages)-1].Content
	}
	judgeMsgs = append(judgeMsgs, llm.Message{Role: "user", Content: "Original Query: " + originalQuery + "\n\n" + judgePrompt})
	judgeReq.Messages = judgeMsgs
	judgeReq.Model = decision.FusionJudge

	// 3. Select judge provider
	judgeAdapter, judgeDecision, err := s.router.SelectExcluding(ctx, &judgeReq, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to route to judge model: %w", err)
	}

	judgeResp, err := judgeAdapter.Complete(ctx, &judgeReq)
	latency := time.Since(start)

	if err != nil {
		s.cb.RecordResult(judgeDecision.ProviderID, false)
		return nil, fmt.Errorf("judge model failed: %w", err)
	}
	s.cb.RecordResult(judgeDecision.ProviderID, true)

	// Update tokens
	if judgeResp.Usage != nil {
		promptTokens += judgeResp.Usage.PromptTokens
		completionTokens += judgeResp.Usage.CompletionTokens
	}

	judgeResp.GatewayID = req.RequestID
	judgeResp.Model = req.Model
	judgeResp.Strategy = "combo:fusion"
	judgeResp.Provider = "fusion-ensemble"
	judgeResp.Usage = &llm.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}

	s.settle(ctx, req, decision, judgeResp.Usage, latency) // log usage for the combo

	return judgeResp, nil
}

func (s *Service) executeFusionStream(ctx context.Context, req *llm.LLMRequest, decision routing.RoutingDecision) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	if len(decision.FusionProviders) == 0 {
		return nil, nil, errors.NewGatewayError("fusion_no_providers", "No healthy providers available for fusion", 503)
	}
	if decision.FusionJudge == "" {
		return nil, nil, errors.NewGatewayError("fusion_no_judge", "Fusion strategy requires a judge model", 400)
	}

	start := time.Now()

	// 1. Run members in parallel
	var wg sync.WaitGroup
	results := make([]string, len(decision.FusionProviders))
	errs := make([]error, len(decision.FusionProviders))
	var promptTokens, completionTokens int
	var mu sync.Mutex

	for i, adapter := range decision.FusionProviders {
		wg.Add(1)
		go func(idx int, p providers.ProviderAdapter) {
			defer wg.Done()
			memberReq := *req
			memberReq.Stream = false
			memberReq.Model = p.Models()[0]

			mResp, err := p.Complete(ctx, &memberReq)
			if err != nil {
				errs[idx] = err
				return
			}

			mu.Lock()
			if mResp.Usage != nil {
				promptTokens += mResp.Usage.PromptTokens
				completionTokens += mResp.Usage.CompletionTokens
			}
			if len(mResp.Choices) > 0 {
				results[idx] = mResp.Choices[0].Message.Content
			}
			mu.Unlock()
		}(i, adapter)
	}
	wg.Wait()

	// Gather successful results
	var validResults []string
	for i, res := range results {
		if errs[i] == nil && res != "" {
			validResults = append(validResults, fmt.Sprintf("Response %d:\n%s", i+1, res))
		}
	}

	if len(validResults) == 0 {
		return nil, nil, errors.NewGatewayError("fusion_failed", "All fusion members failed to return a response", 502)
	}

	// 2. Run the judge in stream mode
	judgePrompt := fmt.Sprintf("Synthesize the following responses into a single, high-quality answer. If there are contradictions, resolve them logically.\n\n%s", strings.Join(validResults, "\n\n---\n\n"))

	judgeReq := *req
	judgeReq.Stream = true
	var judgeMsgs []llm.Message
	for _, m := range req.Messages {
		if m.Role == "system" {
			judgeMsgs = append(judgeMsgs, m)
		}
	}
	originalQuery := ""
	if len(req.Messages) > 0 {
		originalQuery = req.Messages[len(req.Messages)-1].Content
	}
	judgeMsgs = append(judgeMsgs, llm.Message{Role: "user", Content: "Original Query: " + originalQuery + "\n\n" + judgePrompt})
	judgeReq.Messages = judgeMsgs
	judgeReq.Model = decision.FusionJudge

	judgeAdapter, judgeDecision, err := s.router.SelectExcluding(ctx, &judgeReq, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to route to judge model: %w", err)
	}

	streamAdapter, ok := judgeAdapter.(providers.StreamAdapter)
	if !ok {
		return nil, nil, fmt.Errorf("judge provider does not support streaming")
	}

	streamCh, err := streamAdapter.Stream(ctx, &judgeReq)
	if err != nil {
		s.cb.RecordResult(judgeDecision.ProviderID, false)
		return nil, nil, fmt.Errorf("judge model failed: %w", err)
	}
	s.cb.RecordResult(judgeDecision.ProviderID, true)

	judgeRespMeta := &llm.LLMResponse{
		GatewayID:    req.RequestID,
		Model:        req.Model,
		Provider:     "fusion-ensemble",
		Strategy:     "combo:fusion",
		AttemptCount: 1,
	}

	outCh := make(chan llm.StreamEvent)
	go func() {
		defer close(outCh)
		var finalUsage *llm.Usage
		for event := range streamCh {
			if event.Usage != nil {
				finalUsage = event.Usage
				event.Usage.PromptTokens += promptTokens
				event.Usage.CompletionTokens += completionTokens
				event.Usage.TotalTokens = event.Usage.PromptTokens + event.Usage.CompletionTokens
			}
			outCh <- event
		}

		latency := time.Since(start)
		if finalUsage != nil {
			s.settle(ctx, req, decision, finalUsage, latency)
		}
	}()

	return outCh, judgeRespMeta, nil
}
