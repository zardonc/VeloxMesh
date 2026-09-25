package adaptertest

import (
	"context"
	"net/http"
	"strings"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

type ToolContractFixture struct {
	Complete func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error)
	Stream   func(context.Context, *llm.LLMRequest) ([]llm.StreamEvent, error)
}

type ToolContractCase struct {
	Name                 string
	Request              *llm.LLMRequest
	Stream               bool
	ExpectedFinishReason string
	ExpectedErrorCode    string
}

type ToolContractCompletion struct {
	FinishReason string
	Err          error
}

type ToolContractObserver func(ToolContractCompletion)

// RunToolContract gives callers one completion handoff; it does not own terminal lifecycle effects.
func RunToolContract(ctx context.Context, fixture ToolContractFixture, contract ToolContractCase, observe ToolContractObserver) (err error) {
	if observe == nil {
		return toolContractError()
	}

	completion := ToolContractCompletion{}
	defer func() {
		completion.Err = err
		observe(completion)
	}()

	finishReason, runErr := runToolFixture(ctx, fixture, contract)
	err = verifyToolContract(contract, finishReason, runErr)
	completion.FinishReason = finishReason
	return err
}

func runToolFixture(ctx context.Context, fixture ToolContractFixture, contract ToolContractCase) (string, error) {
	if invalidToolRequest(contract.Request) {
		return "", toolContractError()
	}
	if contract.Stream {
		return runToolStream(ctx, fixture.Stream, contract.Request)
	}
	return runToolCompletion(ctx, fixture.Complete, contract.Request)
}

func runToolCompletion(ctx context.Context, complete func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error), request *llm.LLMRequest) (string, error) {
	if complete == nil {
		return "", toolContractError()
	}

	response, err := complete(ctx, request)
	if err != nil || response == nil || len(response.Choices) == 0 {
		return "", toolContractError()
	}
	return response.Choices[0].FinishReason, nil
}

func runToolStream(ctx context.Context, stream func(context.Context, *llm.LLMRequest) ([]llm.StreamEvent, error), request *llm.LLMRequest) (string, error) {
	if stream == nil {
		return "", toolContractError()
	}

	events, err := stream(ctx, request)
	if err != nil {
		return "", toolContractError()
	}
	for _, event := range events {
		if event.Error != nil {
			return "", toolContractError()
		}
		if event.FinishReason != "" {
			return event.FinishReason, nil
		}
	}
	return "", toolContractError()
}

func invalidToolRequest(request *llm.LLMRequest) bool {
	return request == nil || len(request.Tools) == 0 || !validToolChoice(request.ToolChoice) || !hasToolHistory(request.Messages)
}

func validToolChoice(choice *llm.ToolChoice) bool {
	if choice == nil {
		return true
	}
	switch choice.Mode {
	case llm.ToolChoiceAuto, llm.ToolChoiceNone, llm.ToolChoiceRequired:
		return true
	case llm.ToolChoiceNamed:
		return strings.TrimSpace(choice.FunctionName) != ""
	default:
		return false
	}
}

func hasToolHistory(messages []llm.Message) bool {
	callIDs := make(map[string]struct{})
	for _, message := range messages {
		if message.Role != llm.RoleAssistant {
			continue
		}
		for _, call := range message.ToolCalls {
			if call.Type == llm.ToolTypeFunction && strings.TrimSpace(call.ID) != "" {
				callIDs[call.ID] = struct{}{}
			}
		}
	}
	for _, message := range messages {
		if message.Role == llm.RoleTool {
			if _, ok := callIDs[message.ToolCallID]; ok {
				return true
			}
		}
	}
	return false
}

func verifyToolContract(contract ToolContractCase, finishReason string, runErr error) error {
	if contract.ExpectedErrorCode != "" {
		if runErr == nil || !hasGatewayCode(runErr, contract.ExpectedErrorCode) {
			return toolContractError()
		}
		return runErr
	}
	if runErr != nil {
		return runErr
	}
	if contract.ExpectedFinishReason != "" && finishReason != contract.ExpectedFinishReason {
		return toolContractError()
	}
	return nil
}

func hasGatewayCode(err error, expectedCode string) bool {
	gatewayErr, ok := err.(*gatewayerrors.GatewayError)
	return ok && gatewayErr.Code == expectedCode
}

func toolContractError() error {
	return gatewayerrors.NewGatewayError(gatewayerrors.ProviderBadResponse, "invalid provider tool protocol response", http.StatusBadGateway)
}
