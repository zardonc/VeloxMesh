//go:build phase29preflight

package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/llm"
)

// Failure cases: no real nonterminal content; gate timeout; upstream HTTP error;
// cancelled delivery settles Usage or damages health; complete delivery fails to
// settle exactly once. Gating controls gateway delivery, not provider compute time.
const geminiGateLimit = 5 * time.Second

type geminiTimelineTransport struct {
	base      http.RoundTripper
	target    string
	test      *testing.T
	gate      bool
	interrupt bool
	delivered chan time.Time
	cancelled chan time.Time
}

func installGeminiTimeline(t *testing.T, env map[string]string, gate bool) geminiTimelineTransport {
	t.Helper()
	endpoint, err := url.Parse(env["SANS_BASE_URL"])
	if err != nil || endpoint.Host == "" {
		t.Fatal("explicit Gemini endpoint required")
	}
	previous := http.DefaultTransport
	transport := geminiTimelineTransport{base: previous, target: endpoint.Host, test: t, gate: gate,
		delivered: make(chan time.Time, 1), cancelled: make(chan time.Time, 1)}
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
	return transport
}

func (transport geminiTimelineTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != transport.target {
		return transport.base.RoundTrip(request)
	}
	started := time.Now()
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		transport.test.Logf("upstream transport_failed=true elapsed_ms=%.3f cancelled=%t error_type=%T", float64(time.Since(started).Microseconds())/1000, request.Context().Err() != nil, err)
		return nil, err
	}
	transport.test.Logf("upstream status=%d headers_ms=%.3f stream=%t", response.StatusCode, float64(time.Since(started).Microseconds())/1000, strings.Contains(request.URL.Path, "streamGenerateContent"))
	if response.StatusCode >= http.StatusBadRequest {
		return response, nil
	}
	if !strings.Contains(request.URL.Path, "streamGenerateContent") {
		return response, nil
	}
	reader, writer := io.Pipe()
	go transport.copyStream(request, response.Body, writer)
	clone := *response
	clone.Body = reader
	return &clone, nil
}

func (transport geminiTimelineTransport) copyStream(request *http.Request, body io.ReadCloser, writer *io.PipeWriter) {
	defer body.Close()
	if !transport.gate {
		_, err := io.Copy(writer, body)
		transport.test.Logf("upstream body_end_utc=%s error_type=%T", time.Now().UTC().Format(time.RFC3339Nano), err)
		writer.CloseWithError(err)
		return
	}
	err := transport.copyUntilContent(body, writer)
	if err != nil {
		writer.CloseWithError(err)
		return
	}
	transport.delivered <- time.Now()
	if transport.interrupt {
		writer.CloseWithError(io.ErrUnexpectedEOF)
		return
	}
	<-request.Context().Done()
	transport.cancelled <- time.Now()
	writer.CloseWithError(request.Context().Err())
}

func (transport geminiTimelineTransport) copyUntilContent(body io.Reader, writer io.Writer) error {
	reader := bufio.NewReader(body)
	for {
		frame, err := readGeminiFrame(reader)
		if err != nil {
			return err
		}
		content, terminal, err := geminiFrameState(frame)
		if err != nil {
			return err
		}
		if terminal {
			return fmt.Errorf("real provider completed before controlled cancellation window")
		}
		if _, err := writer.Write(frame); err != nil {
			return err
		}
		if content {
			transport.test.Logf("real_nonterminal_frame_sha256=%x delivered_utc=%s", sha256.Sum256(frame), time.Now().UTC().Format(time.RFC3339Nano))
			return nil
		}
	}
}

func readGeminiFrame(reader *bufio.Reader) ([]byte, error) {
	var frame []byte
	for {
		line, err := reader.ReadBytes('\n')
		frame = append(append([]byte(nil), frame...), line...)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(line)) == "" {
			return frame, nil
		}
	}
}

func geminiFrameState(frame []byte) (bool, bool, error) {
	var content, terminal bool
	for _, line := range strings.Split(string(frame), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Candidates []struct {
				FinishReason string `json:"finishReason"`
				Content      struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			return false, false, err
		}
		for _, candidate := range event.Candidates {
			terminal = terminal || candidate.FinishReason != ""
			for _, part := range candidate.Content.Parts {
				content = content || part.Text != ""
			}
		}
	}
	return content, terminal, nil
}

func TestLiveGeminiCancelBeforeComplete(t *testing.T) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	timeline := installGeminiTimeline(t, env, true)
	chain := newLiveChainWithEnvironment(t, env)
	response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "List the integers from one to one thousand, one per line."}}})
	defer response.Body.Close()
	liveReadFirstFragment(t, response.Body)
	select {
	case delivered := <-timeline.delivered:
		t.Logf("controlled_delivery_utc=%s", delivered.UTC().Format(time.RFC3339Nano))
	case <-time.After(geminiGateLimit):
		t.Fatal("real nonterminal delivery was not observed")
	}
	closed := time.Now()
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case cancelled := <-timeline.cancelled:
		t.Logf("client_close_utc=%s upstream_context_cancel_utc=%s propagation_ms=%.3f", closed.UTC().Format(time.RFC3339Nano), cancelled.UTC().Format(time.RFC3339Nano), float64(cancelled.Sub(closed).Microseconds())/1000)
	case <-time.After(geminiGateLimit):
		t.Fatal("upstream cancellation was not propagated")
	}
	time.Sleep(liveSettleWait)
	liveAssertSettlement(t, chain, 0)
	if health := chain.app.HealthStore().Snapshot("sans-primary"); health.TotalFailures != 0 || health.PendingRequests != 0 {
		t.Fatal("controlled cancellation damaged provider health or leaked pending request")
	}
}

func TestLiveGeminiCompleteBeforeCancel(t *testing.T) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	installGeminiTimeline(t, env, false)
	chain := newLiveChainWithEnvironment(t, env)
	response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with a short greeting."}}})
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "data: [DONE]") != 1 || strings.Contains(string(body), "\"error\"") {
		t.Fatal("completed stream lacked a clean terminal")
	}
	t.Logf("gateway_done_utc=%s body_sha256=%x", time.Now().UTC().Format(time.RFC3339Nano), sha256.Sum256(body))
	liveAssertSettlement(t, chain, 1)
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("client_close_after_settlement_utc=%s", time.Now().UTC().Format(time.RFC3339Nano))
	liveAssertSettlement(t, chain, 1)
}

func TestLiveGeminiToolTimeline(t *testing.T) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	installGeminiTimeline(t, env, false)
	liveToolStreaming(t, newLiveChainWithEnvironment(t, env))
}

func TestLiveGeminiNativeTimeline(t *testing.T) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	installGeminiTimeline(t, env, false)
	liveGeminiNativeToolStream(t, env)
}

func TestLiveGeminiInterruptedBody(t *testing.T) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	base := installGeminiTimeline(t, env, true)
	http.DefaultTransport = geminiTimelineTransport{base: base.base, target: base.target, test: t,
		gate: true, interrupt: true, delivered: base.delivered, cancelled: base.cancelled}
	chain := newLiveChainWithEnvironment(t, env)
	response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "List the integers from one to one thousand, one per line."}}})
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "provider_bad_response") {
		t.Fatal("interrupted real upstream body did not produce a provider protocol failure")
	}
	liveAssertSettlement(t, chain, 0)
}
