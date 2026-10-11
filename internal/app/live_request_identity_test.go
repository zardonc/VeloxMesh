//go:build phase29preflight

package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"veloxmesh/internal/llm"
)

// Separate HTTP requests may share a client correlation ID, but must each bill.
func TestLiveRepeatedRequestIDSettlement(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", "off")
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			chain := newLiveChainWithEnvironment(t, env)
			for range 2 {
				liveRepeatedIDRequest(t, chain, stream)
			}
			liveAssertSettlement(t, chain, 2)
		})
	}
}

func liveRepeatedIDRequest(t *testing.T, chain liveChain, stream bool) {
	t.Helper()
	const correlationID = "same-client-correlation"
	payload := llm.ChatCompletionRequest{Model: chain.model, Stream: stream,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with a short greeting."}}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, chain.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+chain.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", correlationID)
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("X-Request-ID") != correlationID {
		t.Fatalf("request failed or correlation ID changed: status=%d", response.StatusCode)
	}
	if !stream {
		if result := liveDecodeChat(t, response); result.Usage.TotalTokens == 0 {
			t.Fatal("real response has no billable usage")
		}
		return
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		liveRejectSSEError(t, line)
	}
	if bytes.Count(data, []byte("data: [DONE]")) != 1 {
		t.Fatal("stream must complete exactly once")
	}
}
