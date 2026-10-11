package adaptertest

import (
	"context"
	"errors"
	"strings"
	"testing"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

func TestToolContractRunsFiveChoiceModesWithDefinitionsAndHistory(t *testing.T) {
	fixture := ToolContractFixture{
		Complete: func(_ context.Context, request *llm.LLMRequest) (*llm.LLMResponse, error) {
			if len(request.Tools) != 1 || len(request.Messages) != 3 {
				t.Fatalf("contract request lost definitions or history: %#v", request)
			}
			return &llm.LLMResponse{Choices: []llm.Choice{{
				Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
					ID: "call-1", Type: llm.ToolTypeFunction,
					Function: llm.FunctionCall{Name: "lookup", Arguments: "{}"},
				}}},
				FinishReason: "tool_calls",
			}}}, nil
		},
	}

	for _, choice := range toolChoiceMatrix() {
		t.Run(choice.name, func(t *testing.T) {
			observed := make([]ToolContractCompletion, 0, 1)
			err := RunToolContract(context.Background(), fixture, ToolContractCase{
				Name:                 "non-stream-" + choice.name,
				Request:              toolContractRequest(choice.choice, false),
				ExpectedFinishReason: "tool_calls",
			}, func(completion ToolContractCompletion) { observed = append(observed, completion) })
			if err != nil {
				t.Fatalf("RunToolContract() error = %v", err)
			}
			if len(observed) != 1 || observed[0].FinishReason != "tool_calls" || observed[0].Err != nil {
				t.Fatalf("completion observer = %#v", observed)
			}
		})
	}
}

func TestToolContractRunsStreamingFragmentsWithoutProviderWireAssumptions(t *testing.T) {
	index := 0
	id := "call-1"
	name := "lookup"
	toolType := llm.ToolTypeFunction
	arguments := "{}"
	fixture := ToolContractFixture{
		Stream: func(_ context.Context, _ *llm.LLMRequest) ([]llm.StreamEvent, error) {
			return []llm.StreamEvent{
				{ToolCalls: []llm.ToolCallChunk{{Index: &index, ID: &id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}}}},
				{ToolCalls: []llm.ToolCallChunk{{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &arguments}}}},
				{FinishReason: "tool_calls"},
			}, nil
		},
	}
	observed := make([]ToolContractCompletion, 0, 1)
	err := RunToolContract(context.Background(), fixture, ToolContractCase{
		Name: "streaming-tool-call", Request: toolContractRequest(nil, true), Stream: true,
		ExpectedFinishReason: "tool_calls",
	}, func(completion ToolContractCompletion) { observed = append(observed, completion) })
	if err != nil {
		t.Fatalf("RunToolContract() error = %v", err)
	}
	if len(observed) != 1 || observed[0].FinishReason != "tool_calls" || observed[0].Err != nil {
		t.Fatalf("stream completion observer = %#v", observed)
	}
}

func TestToolContractProjectsProviderErrorsWithoutPayloadLeak(t *testing.T) {
	fixture := ToolContractFixture{
		Complete: func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
			return nil, errors.New("provider payload token=secret-value")
		},
	}
	observed := make([]ToolContractCompletion, 0, 1)
	err := RunToolContract(context.Background(), fixture, ToolContractCase{
		Name: "malformed-upstream", Request: toolContractRequest(nil, false),
		ExpectedErrorCode: gatewayerrors.ProviderBadResponse,
	}, func(completion ToolContractCompletion) { observed = append(observed, completion) })
	assertToolContractProviderError(t, err)
	if len(observed) != 1 || observed[0].Err == nil {
		t.Fatalf("error completion observer = %#v", observed)
	}
}

func TestToolContractRequiresOneCallerOwnedCompletionObserver(t *testing.T) {
	fixture := ToolContractFixture{
		Complete: func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
			return &llm.LLMResponse{Choices: []llm.Choice{{
				Message: llm.Message{Role: llm.RoleAssistant}, FinishReason: "stop",
			}}}, nil
		},
	}
	observerCalls := 0
	err := RunToolContract(context.Background(), fixture, ToolContractCase{
		Name: "terminal-owner-seam", Request: toolContractRequest(nil, false), ExpectedFinishReason: "stop",
	}, func(ToolContractCompletion) { observerCalls++ })
	if err != nil {
		t.Fatal(err)
	}
	if observerCalls != 1 {
		t.Fatalf("completion observer calls=%d, want one caller-owned handoff", observerCalls)
	}
}

type choiceCase struct {
	name   string
	choice *llm.ToolChoice
}

func toolChoiceMatrix() []choiceCase {
	return []choiceCase{
		{name: "omitted", choice: nil},
		{name: "auto", choice: &llm.ToolChoice{Mode: llm.ToolChoiceAuto}},
		{name: "none", choice: &llm.ToolChoice{Mode: llm.ToolChoiceNone}},
		{name: "required", choice: &llm.ToolChoice{Mode: llm.ToolChoiceRequired}},
		{name: "named", choice: &llm.ToolChoice{Mode: llm.ToolChoiceNamed, FunctionName: "lookup"}},
	}
}

func toolContractRequest(choice *llm.ToolChoice, stream bool) *llm.LLMRequest {
	return &llm.LLMRequest{
		Model: "offline-model", Stream: stream, ToolChoice: choice,
		Tools: []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{Name: "lookup"}}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "call lookup"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "prior-call", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: "{}"}}}},
			{Role: llm.RoleTool, ToolCallID: "prior-call", Content: "{}"},
		},
	}
}

func assertToolContractProviderError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("required provider error")
	}
	gatewayErr, ok := err.(*gatewayerrors.GatewayError)
	if !ok || gatewayErr.Code != gatewayerrors.ProviderBadResponse {
		t.Fatalf("error=%v, want provider_bad_response", err)
	}
	if strings.Contains(gatewayErr.Message, "secret-value") || strings.Contains(gatewayErr.Message, "token=") {
		t.Fatalf("provider payload leaked into error: %q", gatewayErr.Message)
	}
}

func TestToolContractRejectsMissingDefinitionsOrToolHistory(t *testing.T) {
	fixtureCalls := 0
	fixture := ToolContractFixture{
		Complete: func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
			fixtureCalls++
			return &llm.LLMResponse{Choices: []llm.Choice{{FinishReason: "stop"}}}, nil
		},
	}
	request := &llm.LLMRequest{
		Model:    "offline-model",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "call lookup"}},
	}
	observed := make([]ToolContractCompletion, 0, 1)
	err := RunToolContract(context.Background(), fixture, ToolContractCase{
		Name: "missing-tool-context", Request: request, ExpectedErrorCode: gatewayerrors.ProviderBadResponse,
	}, func(completion ToolContractCompletion) { observed = append(observed, completion) })
	assertToolContractProviderError(t, err)
	if fixtureCalls != 0 || len(observed) != 1 || observed[0].Err == nil {
		t.Fatalf("fixture calls=%d completion=%#v", fixtureCalls, observed)
	}
}
