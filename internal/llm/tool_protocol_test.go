package llm

import (
	"reflect"
	"testing"
)

// Failure matrix: choice must be closed; schemas must remain opaque; omitted
// choice must remain distinguishable from explicit values at the boundary.
func TestToolProtocolUsesClosedChoiceAndOpaqueParameters(t *testing.T) {
	requestType := reflect.TypeOf(ChatCompletionRequest{})
	choiceField, ok := requestType.FieldByName("ToolChoice")
	if !ok {
		t.Fatal("ChatCompletionRequest is missing ToolChoice")
	}
	if choiceField.Type.Kind() == reflect.Interface {
		t.Fatalf("ToolChoice must be a closed normalized type, got %s", choiceField.Type)
	}

	functionType := reflect.TypeOf(Function{})
	parametersField, ok := functionType.FieldByName("Parameters")
	if !ok {
		t.Fatal("Function is missing Parameters")
	}
	if parametersField.Type.Kind() == reflect.Interface {
		t.Fatalf("Parameters must preserve opaque JSON, got %s", parametersField.Type)
	}
}
