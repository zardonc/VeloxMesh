//go:build phase29preflight

package app

// Retain real provider frames through its terminal, then inject a deterministic
// delivery fault. Any failure must prevent final Usage and successful Done.
import (
	"bufio"
	"io"
	"net/http"
	"strings"
	"testing"

	"veloxmesh/internal/llm"
)

type geminiTailTransport struct {
	base      geminiTimelineTransport
	duplicate bool
}

func (transport geminiTailTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if request.URL.Host != transport.base.target || !strings.Contains(request.URL.Path, "streamGenerateContent") || response.StatusCode != http.StatusOK {
		return response, nil
	}
	reader, writer := io.Pipe()
	clone := *response
	clone.Body = reader
	go func() {
		defer response.Body.Close()
		frame, err := copyGeminiTerminal(response.Body, writer)
		if err != nil {
			writer.CloseWithError(err)
			return
		}
		if transport.duplicate {
			_, err = writer.Write(frame)
			writer.CloseWithError(err)
			return
		}
		writer.CloseWithError(io.ErrUnexpectedEOF)
	}()
	return &clone, nil
}

func copyGeminiTerminal(body io.Reader, writer io.Writer) ([]byte, error) {
	reader := bufio.NewReader(body)
	for {
		frame, err := readGeminiFrame(reader)
		if err != nil {
			return nil, err
		}
		_, terminal, err := geminiFrameState(frame)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(frame); err != nil {
			return nil, err
		}
		if terminal {
			return frame, nil
		}
	}
}

func TestLiveGeminiTerminalTailFaults(t *testing.T) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	for name, duplicate := range map[string]bool{"truncated": false, "duplicate": true} {
		t.Run(name, func(t *testing.T) {
			base := installGeminiTimeline(t, env, false)
			http.DefaultTransport = geminiTailTransport{base: base, duplicate: duplicate}
			chain := newLiveChainWithEnvironment(t, env)
			response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Stream: true, Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with a short greeting."}}})
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "provider_bad_response") {
				t.Fatal("terminal tail fault was accepted")
			}
			liveAssertSettlement(t, chain, 0)
		})
	}
}
