package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/genai"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

// Malformed upstream pointer arrays must fail closed in both response modes.
func TestGeminiNullResponseMembersFailClosed(t *testing.T) {
	for name, body := range map[string]string{
		"candidate": `{"candidates":[null],"usageMetadata":{"totalTokenCount":1}}`,
		"part":      `{"candidates":[{"content":{"parts":[null]},"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":1}}`,
	} {
		var response genai.GenerateContentResponse
		if err := json.Unmarshal([]byte(body), &response); err != nil {
			t.Fatal(err)
		}
		t.Run(name+" complete", func(t *testing.T) {
			defer rejectGeminiPanic(t)
			_, err := geminiChoice(response.Candidates[0], nil)
			assertGeminiNullResponseError(t, err)
		})
		t.Run(name+" stream", func(t *testing.T) {
			defer rejectGeminiPanic(t)
			adapter := &Adapter{id: "test-provider"}
			events := make(chan llm.StreamEvent, 4)
			adapter.forwardStream(context.Background(), func(yield func(*genai.GenerateContentResponse, error) bool) {
				yield(&response, nil)
			}, events)
			count := 0
			for event := range events {
				count++
				if event.Done || event.Usage != nil {
					t.Fatal("invalid upstream response produced completion/usage")
				}
				assertGeminiNullResponseError(t, event.Error)
			}
			if count != 1 {
				t.Fatalf("expected one terminal error, got %d events", count)
			}
		})
	}
}

func rejectGeminiPanic(t *testing.T) {
	t.Helper()
	if value := recover(); value != nil {
		t.Errorf("malformed provider response panicked: %v", value)
	}
}

func assertGeminiNullResponseError(t *testing.T, err error) {
	t.Helper()
	if err == nil || gatewayerrors.TranslateError(err).Code != gatewayerrors.ProviderBadResponse {
		t.Fatalf("expected provider_bad_response, got %v", err)
	}
}
