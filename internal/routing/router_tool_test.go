package routing_test

import (
	"context"
	"testing"

	"veloxmesh/internal/config"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type toolSpyAdapter struct {
	id           string
	models       []string
	capabilities providers.CapabilitySet
	completeCall int
}

func (a *toolSpyAdapter) ID() string { return a.id }

func (a *toolSpyAdapter) Models() []string { return append([]string(nil), a.models...) }

func (a *toolSpyAdapter) Complete(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
	a.completeCall++
	return nil, nil
}

func (a *toolSpyAdapter) Capabilities() providers.CapabilitySet { return a.capabilities.Clone() }

func (a *toolSpyAdapter) HealthCheck(context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}

type toolProtocolFixture struct {
	definitions bool
	assistant   bool
	results     bool
	streaming   bool
	choices     map[string]bool
}

func TestHealthAwareRouter_ToolProtocolFiltersAutomaticCandidates(t *testing.T) {
	for _, tc := range automaticToolCases() {
		t.Run(tc.name, func(t *testing.T) {
			incompatible := newToolSpyAdapter("incompatible", "shared", tc.missing)
			compatible := newToolSpyAdapter("compatible", "shared", tc.supports)
			router := newToolRouter(t, "round-robin", nil, incompatible, compatible)

			adapter, _, err := router.SelectExcluding(context.Background(), tc.req, map[string]bool{})
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if adapter.ID() != compatible.ID() {
				t.Fatalf("Select() chose %q, want compatible provider %q", adapter.ID(), compatible.ID())
			}
			assertNoUpstreamCalls(t, incompatible, compatible)
		})
	}
}

type automaticToolCase struct {
	name     string
	req      *llm.LLMRequest
	missing  toolProtocolFixture
	supports toolProtocolFixture
}

func automaticToolCases() []automaticToolCase {
	full := fullToolProtocolFixture()
	return []automaticToolCase{
		{name: "definitions", req: toolRequest(llm.ToolProtocolRequirements{HasDefinitions: true}), missing: toolProtocolFixture{}, supports: full},
		{name: "omitted choice", req: toolRequest(llm.ToolProtocolRequirements{HasDefinitions: true}), missing: fixtureWithChoice("omitted", false), supports: fixtureWithChoice("omitted", true)},
		{name: "auto choice", req: toolRequest(explicitChoice(llm.ToolChoiceAuto)), missing: fixtureWithChoice("auto", false), supports: fixtureWithChoice("auto", true)},
		{name: "none choice", req: toolRequest(explicitChoice(llm.ToolChoiceNone)), missing: fixtureWithChoice("none", false), supports: fixtureWithChoice("none", true)},
		{name: "required choice", req: toolRequest(explicitChoice(llm.ToolChoiceRequired)), missing: fixtureWithChoice("required", false), supports: fixtureWithChoice("required", true)},
		{name: "named function choice", req: toolRequest(explicitChoice(llm.ToolChoiceNamed)), missing: fixtureWithChoice("named", false), supports: fixtureWithChoice("named", true)},
		{name: "tool results", req: toolRequest(llm.ToolProtocolRequirements{HasDefinitions: true, HasToolResult: true}), missing: fixtureWithResults(false), supports: fixtureWithResults(true)},
		{name: "assistant tool calls", req: toolRequest(llm.ToolProtocolRequirements{HasAssistantToolCall: true}), missing: fixtureWithAssistant(false), supports: fixtureWithAssistant(true)},
		{name: "streamed tool deltas", req: streamToolRequest(), missing: fixtureWithoutStreaming(), supports: full},
	}
}

func TestHealthAwareRouter_ToolProtocolRejectsExhaustedCandidatesBeforeIO(t *testing.T) {
	adapter := newToolSpyAdapter("incompatible", "shared", fixtureWithChoice("auto", false))
	router := newToolRouter(t, "round-robin", nil, adapter)

	_, _, err := router.SelectExcluding(context.Background(), toolRequest(explicitChoice(llm.ToolChoiceAuto)), map[string]bool{})
	assertGatewayCode(t, err, gatewayerrors.UnsupportedToolChoice)
	assertNoUpstreamCalls(t, adapter)
}
func TestHealthAwareRouter_ToolProtocolRejectsExplicitOverridesBeforeIO(t *testing.T) {
	choiceAdapter := newToolSpyAdapter("choice", "shared", fixtureWithChoice("named", false))
	choiceReq := toolRequest(explicitChoice(llm.ToolChoiceNamed))
	choiceReq.RouteOverride = choiceAdapter.ID()
	_, _, err := newToolRouter(t, "round-robin", nil, choiceAdapter).Select(context.Background(), choiceReq)
	assertGatewayCode(t, err, gatewayerrors.UnsupportedToolChoice)
	assertNoUpstreamCalls(t, choiceAdapter)

	callingAdapter := newToolSpyAdapter("calling", "shared", toolProtocolFixture{})
	callingReq := toolRequest(llm.ToolProtocolRequirements{HasDefinitions: true})
	callingReq.RouteOverride = callingAdapter.ID()
	_, _, err = newToolRouter(t, "round-robin", nil, callingAdapter).Select(context.Background(), callingReq)
	assertGatewayCode(t, err, gatewayerrors.UnsupportedToolCalling)
	assertNoUpstreamCalls(t, callingAdapter)
}

func TestHealthAwareRouter_ToolProtocolDoesNotBypassComboSelection(t *testing.T) {
	t.Run("capacity fallback", func(t *testing.T) {
		first := newToolSpyAdapter("first", "first-model", toolProtocolFixture{})
		second := newToolSpyAdapter("second", "second-model", fixtureWithChoice("required", false))
		combo := providers.Combo{ID: "capacity", Name: "combo", Strategy: "capacity-auto-switch", Members: []string{"first-model", "second-model"}}
		router := newToolRouter(t, "round-robin", []providers.Combo{combo}, first, second)

		req := toolRequest(explicitChoice(llm.ToolChoiceRequired))
		req.Model = combo.Name
		_, _, err := router.Select(context.Background(), req)
		assertGatewayCode(t, err, gatewayerrors.UnsupportedToolChoice)
		assertNoUpstreamCalls(t, first, second)
	})

	t.Run("round robin member", func(t *testing.T) {
		first := newToolSpyAdapter("first", "first-model", toolProtocolFixture{})
		second := newToolSpyAdapter("second", "second-model", fullToolProtocolFixture())
		combo := providers.Combo{ID: "round-robin", Name: "combo", Strategy: "round-robin", Members: []string{"first-model", "second-model"}}
		router := newToolRouter(t, "round-robin", []providers.Combo{combo}, first, second)

		req := toolRequest(explicitChoice(llm.ToolChoiceAuto))
		req.Model = combo.Name
		_, _, err := router.Select(context.Background(), req)
		assertGatewayCode(t, err, gatewayerrors.UnsupportedToolChoice)
		assertNoUpstreamCalls(t, first, second)
	})
}

func TestHealthAwareRouter_ToolProtocolRejectsFusionBeforeIO(t *testing.T) {
	for _, req := range fusionToolRequests() {
		first := newToolSpyAdapter("first", "first-model", fullToolProtocolFixture())
		second := newToolSpyAdapter("second", "second-model", fullToolProtocolFixture())
		combo := providers.Combo{ID: "fusion", Name: "combo", Strategy: "fusion", Members: []string{"first-model", "second-model"}, Judge: "judge"}
		router := newToolRouter(t, "round-robin", []providers.Combo{combo}, first, second)

		req.Model = combo.Name
		_, decision, err := router.Select(context.Background(), req)
		assertGatewayCode(t, err, gatewayerrors.UnsupportedToolCalling)
		if decision.IsFusion {
			t.Fatal("Fusion decision was returned for a tool protocol request")
		}
		assertNoUpstreamCalls(t, first, second)
	}
}

func TestToolCapabilitySnapshotsAreCloneIsolated(t *testing.T) {
	adapter := newToolSpyAdapter("provider", "shared", fullToolProtocolFixture())
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)

	first := registry.EligibleProviders("shared", providers.OperationChatCompletions)
	if len(first) != 1 {
		t.Fatalf("EligibleProviders() length = %d, want 1", len(first))
	}
	mutateChoiceSupport(&first[0].Capabilities, "auto", false)

	second := registry.EligibleProviders("shared", providers.OperationChatCompletions)
	if !toolChoiceSupported(second[0].Capabilities, "auto") {
		t.Fatal("mutating a returned capability snapshot changed catalog state")
	}
}

func TestHealthAwareRouter_TextAndStreamRequestsRemainCompatible(t *testing.T) {
	adapter := newToolSpyAdapter("provider", "shared", toolProtocolFixture{})
	router := newToolRouter(t, "round-robin", nil, adapter)

	for _, req := range []*llm.LLMRequest{{Model: "shared"}, {Model: "shared", Stream: true}} {
		selected, _, err := router.Select(context.Background(), req)
		if err != nil {
			t.Fatalf("Select() error = %v", err)
		}
		if selected.ID() != adapter.ID() {
			t.Fatalf("Select() provider = %q, want %q", selected.ID(), adapter.ID())
		}
	}
}

func newToolRouter(t *testing.T, strategy string, combos []providers.Combo, adapters ...*toolSpyAdapter) *routing.HealthAwareRouter {
	t.Helper()
	providerAdapters := make([]providers.ProviderAdapter, 0, len(adapters))
	store := health.NewInMemoryStore()
	for _, adapter := range adapters {
		providerAdapters = append(providerAdapters, adapter)
		store.EnsureProvider(adapter.ID(), 3, 1)
	}
	registry := providers.NewRegistry(&config.Config{}, providerAdapters, combos)
	return routing.NewHealthAwareRouter(registry, store, strategy, nil)
}

func newToolSpyAdapter(id, model string, fixture toolProtocolFixture) *toolSpyAdapter {
	return &toolSpyAdapter{id: id, models: []string{model}, capabilities: capabilitySet(fixture)}
}

func capabilitySet(fixture toolProtocolFixture) providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeOpenAICompatible,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
		Streaming:           true,
		ToolCalling:         true,
		ToolProtocol: providers.ToolProtocolCapability{
			Definitions:        fixture.definitions,
			AssistantToolCalls: fixture.assistant,
			ToolResults:        fixture.results,
			StreamingDeltas:    fixture.streaming,
			ChoiceModes:        toolChoiceModes(fixture.choices),
		},
	}
}

func toolChoiceModes(choices map[string]bool) map[providers.ToolChoiceCapabilityMode]bool {
	modes := make(map[providers.ToolChoiceCapabilityMode]bool, len(choices))
	for choice, supported := range choices {
		modes[providers.ToolChoiceCapabilityMode(choice)] = supported
	}
	return modes
}

func mutateChoiceSupport(capabilities *providers.CapabilitySet, choice string, supported bool) {
	capabilities.ToolProtocol.ChoiceModes[providers.ToolChoiceCapabilityMode(choice)] = supported
}

func toolChoiceSupported(capabilities providers.CapabilitySet, choice string) bool {
	return capabilities.ToolProtocol.ChoiceModes[providers.ToolChoiceCapabilityMode(choice)]
}
func toolRequest(requirements llm.ToolProtocolRequirements) *llm.LLMRequest {
	return &llm.LLMRequest{Model: "shared", ToolRequirements: requirements}
}

func explicitChoice(mode llm.ToolChoiceMode) llm.ToolProtocolRequirements {
	return llm.ToolProtocolRequirements{HasDefinitions: true, HasExplicitChoice: true, ChoiceMode: mode, NamedFunction: "weather"}
}

func fullToolProtocolFixture() toolProtocolFixture {
	return toolProtocolFixture{definitions: true, assistant: true, results: true, streaming: true, choices: allChoices()}
}

func fixtureWithChoice(choice string, supported bool) toolProtocolFixture {
	choices := allChoices()
	choices[choice] = supported
	return toolProtocolFixture{definitions: true, assistant: true, results: true, streaming: true, choices: choices}
}

func fixtureWithResults(supported bool) toolProtocolFixture {
	return toolProtocolFixture{definitions: true, assistant: true, results: supported, streaming: true, choices: allChoices()}
}
func fixtureWithAssistant(supported bool) toolProtocolFixture {
	return toolProtocolFixture{definitions: true, assistant: supported, results: true, streaming: true, choices: allChoices()}
}

func fixtureWithoutStreaming() toolProtocolFixture {
	return toolProtocolFixture{definitions: true, assistant: true, results: true, choices: allChoices()}
}

func allChoices() map[string]bool {
	return map[string]bool{"omitted": true, "auto": true, "none": true, "required": true, "named": true}
}

func streamToolRequest() *llm.LLMRequest {
	req := toolRequest(llm.ToolProtocolRequirements{HasDefinitions: true})
	req.Stream = true
	return req
}

func fusionToolRequests() []*llm.LLMRequest {
	return []*llm.LLMRequest{
		toolRequest(llm.ToolProtocolRequirements{HasDefinitions: true}),
		toolRequest(explicitChoice(llm.ToolChoiceAuto)),
		toolRequest(llm.ToolProtocolRequirements{HasAssistantToolCall: true}),
		toolRequest(llm.ToolProtocolRequirements{HasToolResult: true}),
	}
}

func assertGatewayCode(t *testing.T, err error, code string) {
	t.Helper()
	gatewayErr, ok := err.(*gatewayerrors.GatewayError)
	if !ok {
		t.Fatalf("error = %v, want gateway error code %q", err, code)
	}
	if gatewayErr.Code != code || gatewayErr.HTTPStatus < 400 || gatewayErr.HTTPStatus >= 500 {
		t.Fatalf("gateway error = (%q, %d), want 4xx %q", gatewayErr.Code, gatewayErr.HTTPStatus, code)
	}
}

func assertNoUpstreamCalls(t *testing.T, adapters ...*toolSpyAdapter) {
	t.Helper()
	for _, adapter := range adapters {
		if adapter.completeCall != 0 {
			t.Fatalf("provider %q received %d upstream calls", adapter.ID(), adapter.completeCall)
		}
	}
}
