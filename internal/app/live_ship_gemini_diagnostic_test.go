//go:build phase29preflight

package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"

	"veloxmesh/internal/llm"
)

func TestLiveGeminiNativeContinuation(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("native real provider diagnostic opt-in")
	}
	env := liveEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: env["SANS_PRIMARY_API_KEY"], Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL: env["SANS_BASE_URL"]}})
	if err != nil {
		t.Fatal("native client setup failed")
	}
	user := &genai.Content{Role: "user", Parts: []*genai.Part{{Text: "Use the add function to compute 19 plus 23. Return only the result after receiving the tool output."}}}
	function := &genai.FunctionDeclaration{Name: "add", Description: "Add two integers.", Parameters: &genai.Schema{
		Type: genai.TypeObject, Properties: map[string]*genai.Schema{"a": {Type: genai.TypeInteger}, "b": {Type: genai.TypeInteger}}, Required: []string{"a", "b"},
	}}
	config := &genai.GenerateContentConfig{Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{function}}},
		ToolConfig: &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny}},
	}
	first, err := client.Models.GenerateContent(ctx, env["SANS_PRIMARY_DEFAULT_MODEL"], []*genai.Content{user}, config)
	if err != nil {
		liveNativeError(t, err)
	}
	if len(first.Candidates) != 1 || first.Candidates[0].Content == nil {
		t.Fatal("invalid native candidate")
	}
	content := first.Candidates[0].Content
	output := liveNativeCalculation(t, content)
	nextConfig := &genai.GenerateContentConfig{Tools: config.Tools, ToolConfig: &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeNone}}}
	second, err := client.Models.GenerateContent(ctx, env["SANS_PRIMARY_DEFAULT_MODEL"], []*genai.Content{user, content, output}, nextConfig)
	if err != nil {
		liveNativeError(t, err)
	}
	liveAssertNativeAnswer(t, second)
	t.Log("direct native continuation passed with the real provider's complete content retained")
	liveAssertSignatureRequired(t, nativeSignatureCheck{client: client, contents: []*genai.Content{user, content, output}, config: nextConfig})
}

func liveAssertNativeAnswer(t *testing.T, response *genai.GenerateContentResponse) {
	t.Helper()
	if len(response.Candidates) != 1 || response.Candidates[0].Content == nil {
		t.Fatal("invalid native continuation")
	}
	var answer strings.Builder
	for _, part := range response.Candidates[0].Content.Parts {
		answer.WriteString(part.Text)
	}
	if !strings.Contains(answer.String(), "42") {
		t.Fatal("native continuation did not return calculation result")
	}
}

func TestLiveGeminiNativeToolStream(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real native stream diagnostic opt-in")
	}
	liveGeminiNativeToolStream(t, liveEnvironment(t))
}

func liveGeminiNativeToolStream(t *testing.T, env map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: env["SANS_PRIMARY_API_KEY"], Backend: genai.BackendGeminiAPI, HTTPOptions: genai.HTTPOptions{BaseURL: env["SANS_BASE_URL"]}})
	if err != nil {
		t.Fatal("native stream client setup failed")
	}
	request := liveToolRequest(liveChain{model: env["SANS_PRIMARY_DEFAULT_MODEL"]}, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	function := request.Tools[0].Function
	config := &genai.GenerateContentConfig{Tools: []*genai.Tool{{FunctionDeclarations: []*genai.FunctionDeclaration{{Name: function.Name, Description: function.Description, ParametersJsonSchema: function.Parameters}}}},
		ToolConfig: &genai.ToolConfig{FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny}},
	}
	user := &genai.Content{Role: "user", Parts: []*genai.Part{{Text: request.Messages[0].Content}}}
	started := time.Now()
	parts := []*genai.Part{}
	responses := 0
	for response, err := range client.Models.GenerateContentStream(ctx, request.Model, []*genai.Content{user}, config) {
		if err != nil {
			t.Logf("direct native stream responses=%d elapsed_ms=%.3f", responses, float64(time.Since(started).Microseconds())/1000)
			liveNativeError(t, err)
		}
		responses++
		if responses == 1 {
			t.Logf("direct native stream first_response_ms=%.3f", float64(time.Since(started).Microseconds())/1000)
		}
		if len(response.Candidates) > 0 && response.Candidates[0].Content != nil {
			parts = append(parts, response.Candidates[0].Content.Parts...)
		}
	}
	liveNativeCalculation(t, &genai.Content{Role: "model", Parts: parts})
	t.Logf("direct native stream responses=%d complete_ms=%.3f", responses, float64(time.Since(started).Microseconds())/1000)
}

type nativeSignatureCheck struct {
	client   *genai.Client
	contents []*genai.Content
	config   *genai.GenerateContentConfig
}

func liveAssertSignatureRequired(t *testing.T, check nativeSignatureCheck) {
	t.Helper()
	contents := check.contents
	parts := make([]*genai.Part, 0, len(contents[1].Parts))
	for _, original := range contents[1].Parts {
		part := *original
		part.ThoughtSignature = nil
		parts = append(parts, &part)
	}
	withoutSignature := []*genai.Content{contents[0], {Role: contents[1].Role, Parts: parts}, contents[2]}
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	_, err := check.client.Models.GenerateContent(ctx, os.Getenv("SANS_PRIMARY_DEFAULT_MODEL"), withoutSignature, check.config)
	var apiError genai.APIError
	if !errors.As(err, &apiError) || apiError.Code != http.StatusBadRequest || !strings.Contains(strings.ToLower(apiError.Message), "signature") {
		t.Fatal("signature removal did not reproduce the provider rejection")
	}
	t.Log("real provider rejects the same continuation without opaque signature: HTTP=400 signature_required=true")
}

func liveNativeCalculation(t *testing.T, content *genai.Content) *genai.Content {
	t.Helper()
	var signatures, calls int
	var output *genai.Part
	for _, part := range content.Parts {
		if len(part.ThoughtSignature) > 0 {
			signatures++
		}
		if part.FunctionCall == nil {
			continue
		}
		calls++
		call := part.FunctionCall
		a, aOK := call.Args["a"].(float64)
		b, bOK := call.Args["b"].(float64)
		if !aOK || !bOK || a != 19 || b != 23 || call.Name != "add" {
			t.Fatal("unexpected native calculation inputs")
		}
		output = &genai.Part{FunctionResponse: &genai.FunctionResponse{ID: call.ID, Name: call.Name, Response: map[string]any{"result": a + b}}}
	}
	if calls != 1 {
		t.Fatal("native function call count mismatch")
	}
	t.Logf("native function_calls=%d parts_with_opaque_signature=%d", calls, signatures)
	return &genai.Content{Role: "user", Parts: []*genai.Part{output}}
}

func liveNativeError(t *testing.T, err error) {
	t.Helper()
	var apiError genai.APIError
	if errors.As(err, &apiError) {
		message := strings.ToLower(apiError.Message)
		t.Fatalf("native API status=%d mentions_signature=%t mentions_function_id=%t", apiError.Code, strings.Contains(message, "signature"), strings.Contains(message, "function") && strings.Contains(message, "id"))
	}
	t.Fatal("native provider transport or SDK error")
}
