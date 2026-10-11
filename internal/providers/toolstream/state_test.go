package toolstream

import (
	"strings"
	"sync"
	"testing"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

// Failure matrix: malformed shape, missing or ambiguous index, identity/name drift,
// duplicate IDs, malformed final JSON, invalid finish, repeated completion, and payload safety.
func TestToolStreamPreservesIdentityAndEmitsImmediately(t *testing.T) {
	state := New(Config{GenerateID: func() string { return "generated-id" }})
	index := 0
	id := "upstream-id"
	name := "lookup"
	toolType := llm.ToolTypeFunction

	next, emitted, err := state.Apply(llm.ToolCallChunk{Index: &index, ID: &id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if emitted.ID == nil || *emitted.ID != id || emitted.Function == nil || emitted.Function.Name == nil || *emitted.Function.Name != name {
		t.Fatalf("first fragment lost identity: %#v", emitted)
	}

	fragment := "{}"
	next, emitted, err = next.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &fragment}})
	if err != nil {
		t.Fatalf("Apply() later fragment error = %v", err)
	}
	if emitted.ID != nil || emitted.Function == nil || emitted.Function.Arguments == nil || *emitted.Function.Arguments != fragment {
		t.Fatalf("later fragment was not emitted opaquely: %#v", emitted)
	}

	next, err = next.CompleteCall(index)
	if err != nil {
		t.Fatalf("CompleteCall() error = %v", err)
	}
	_, completion, err := next.Finish("tool_calls")
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if completion.FinishReason != "tool_calls" || len(completion.Calls) != 1 || completion.Calls[0].ID != id || completion.Calls[0].Function.Arguments != fragment {
		t.Fatalf("completion = %#v", completion)
	}
}

func TestToolStreamGeneratesStableOpaqueID(t *testing.T) {
	generations := 0
	state := New(Config{GenerateID: func() string {
		generations++
		return "generated-1"
	}})
	index := 0
	name := "lookup"
	toolType := llm.ToolTypeFunction

	next, emitted, err := state.Apply(llm.ToolCallChunk{Index: &index, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
	if err != nil || emitted.ID == nil || *emitted.ID != "generated-1" || generations != 1 {
		t.Fatalf("first generated fragment = %#v, generations=%d, err=%v", emitted, generations, err)
	}
	fragment := "{}"
	next, _, err = next.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &fragment}})
	if err != nil || generations != 1 {
		t.Fatalf("generated ID was not reused: generations=%d, err=%v", generations, err)
	}
	next, err = next.CompleteCall(index)
	if err != nil {
		t.Fatal(err)
	}
	_, completion, err := next.Finish("tool_calls")
	if err != nil || completion.Calls[0].ID != "generated-1" {
		t.Fatalf("completion = %#v, err=%v", completion, err)
	}
}

func TestToolStreamKeepsInterleavedSameNameCallsSeparate(t *testing.T) {
	state := New(Config{GenerateID: func() string { return "unused" }})
	toolType := llm.ToolTypeFunction
	firstIndex, secondIndex := 0, 1
	firstID, secondID := "call-1", "call-2"
	name := "same_name"

	var err error
	state, _, err = state.Apply(llm.ToolCallChunk{Index: &firstIndex, ID: &firstID, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = state.Apply(llm.ToolCallChunk{Index: &secondIndex, ID: &secondID, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	secondArguments := "{\"second\":true}"
	firstArguments := "{\"first\":true}"
	state, _, err = state.Apply(llm.ToolCallChunk{Index: &secondIndex, Function: &llm.FunctionCallChunk{Arguments: &secondArguments}})
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = state.Apply(llm.ToolCallChunk{Index: &firstIndex, Function: &llm.FunctionCallChunk{Arguments: &firstArguments}})
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.CompleteCall(secondIndex)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.CompleteCall(firstIndex)
	if err != nil {
		t.Fatal(err)
	}
	_, completion, err := state.Finish("tool_calls")
	if err != nil {
		t.Fatal(err)
	}
	if len(completion.Calls) != 2 || completion.Calls[0].ID != firstID || completion.Calls[1].ID != secondID {
		t.Fatalf("index identity drifted: %#v", completion)
	}
}

func TestToolStreamRejectsUntrustedTransitionsWithoutPayloadLeak(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exercise func(t *testing.T) error
	}{
		{
			name: "missing index without active call",
			exercise: func(t *testing.T) error {
				_, _, err := New(Config{GenerateID: func() string { return "id" }}).Apply(llm.ToolCallChunk{})
				return err
			},
		},
		{
			name: "missing first function name",
			exercise: func(t *testing.T) error {
				index := 0
				toolType := llm.ToolTypeFunction
				_, _, err := New(Config{GenerateID: func() string { return "id" }}).Apply(llm.ToolCallChunk{Index: &index, Type: &toolType, Function: &llm.FunctionCallChunk{}})
				return err
			},
		},
		{
			name: "duplicate id across indexes",
			exercise: func(t *testing.T) error {
				state := startedState(t)
				index := 1
				id := "call-0"
				name := "other"
				toolType := llm.ToolTypeFunction
				_, _, err := state.Apply(llm.ToolCallChunk{Index: &index, ID: &id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
				return err
			},
		},
		{
			name: "identity drift",
			exercise: func(t *testing.T) error {
				state := startedState(t)
				index := 0
				id := "other-id"
				_, _, err := state.Apply(llm.ToolCallChunk{Index: &index, ID: &id})
				return err
			},
		},
		{
			name: "name drift",
			exercise: func(t *testing.T) error {
				state := startedState(t)
				index := 0
				name := "other"
				_, _, err := state.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Name: &name}})
				return err
			},
		},
		{
			name: "ambiguous missing index",
			exercise: func(t *testing.T) error {
				state := startedState(t)
				index := 1
				id := "call-1"
				name := "other"
				toolType := llm.ToolTypeFunction
				state, _, err := state.Apply(llm.ToolCallChunk{Index: &index, ID: &id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
				if err != nil {
					return err
				}
				fragment := "{\"secret\":\"never disclose\"}"
				_, _, err = state.Apply(llm.ToolCallChunk{Function: &llm.FunctionCallChunk{Arguments: &fragment}})
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertProviderBadResponse(t, tc.exercise(t))
		})
	}
}

func TestToolStreamDefersArgumentValidationUntilCompletion(t *testing.T) {
	state := startedState(t)
	index := 0
	fragment := "{\"incomplete\":"
	next, _, err := state.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &fragment}})
	if err != nil {
		t.Fatalf("partial arguments were rejected before completion: %v", err)
	}
	next, err = next.CompleteCall(index)
	assertProviderBadResponse(t, err)

	validState := startedState(t)
	first := "{\"key\":"
	second := "\"value\"}"
	validState, _, err = validState.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &first}})
	if err != nil {
		t.Fatal(err)
	}
	validState, _, err = validState.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &second}})
	if err != nil {
		t.Fatal(err)
	}
	validState, err = validState.CompleteCall(index)
	if err != nil {
		t.Fatal(err)
	}
	_, completion, err := validState.Finish("tool_calls")
	if err != nil || completion.Calls[0].Function.Arguments != first+second {
		t.Fatalf("opaque byte order was not retained: %#v, err=%v", completion, err)
	}
}

func TestToolStreamCompletionIsSinglePassWithoutFinalizerCallbacks(t *testing.T) {
	state := startedState(t)
	index := 0
	arguments := "{}"
	var err error
	state, _, err = state.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &arguments}})
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.CompleteCall(index)
	if err != nil {
		t.Fatal(err)
	}
	finished, _, err := state.Finish("tool_calls")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = finished.Finish("tool_calls")
	assertProviderBadResponse(t, err)

	var group sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		group.Go(func() {
			<-start
			_, _, finishErr := finished.Finish("tool_calls")
			results <- finishErr
		})
	}
	close(start)
	group.Wait()
	close(results)
	for finishErr := range results {
		assertProviderBadResponse(t, finishErr)
	}
}

func startedState(t *testing.T) State {
	t.Helper()
	state := New(Config{GenerateID: func() string { return "generated" }})
	index := 0
	id := "call-0"
	name := "lookup"
	toolType := llm.ToolTypeFunction
	next, _, err := state.Apply(llm.ToolCallChunk{Index: &index, ID: &id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
	if err != nil {
		t.Fatalf("start state: %v", err)
	}
	return next
}

func assertProviderBadResponse(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("required provider_bad_response error")
	}
	gatewayErr, ok := err.(*gatewayerrors.GatewayError)
	if !ok || gatewayErr.Code != gatewayerrors.ProviderBadResponse {
		t.Fatalf("error=%v, want provider_bad_response", err)
	}
	if strings.Contains(gatewayErr.Message, "secret") {
		t.Fatalf("raw payload leaked into error: %q", gatewayErr.Message)
	}
}

func TestToolStreamBoundsUnfinishedArgumentBuffers(t *testing.T) {
	state := New(Config{GenerateID: func() string { return "generated" }, MaxArgumentBytes: 4})
	index := 0
	name := "lookup"
	toolType := llm.ToolTypeFunction
	state, _, err := state.Apply(llm.ToolCallChunk{Index: &index, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name}})
	if err != nil {
		t.Fatal(err)
	}
	fragment := "12345"
	_, _, err = state.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &fragment}})
	assertProviderBadResponse(t, err)
}
