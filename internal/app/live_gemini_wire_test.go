//go:build phase29preflight

package app

// Two real stream attempts: compare emitted request content/headers and
// connection reuse while preserving each native/gateway failure independently.
// The gateway test also performs its normal nonstreaming tool continuation.
import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"testing"
	"time"
)

type geminiWireRequest struct {
	body        []byte
	rawBody     []byte
	header      http.Header
	method, url string
}

type geminiWireTransport struct {
	base     http.RoundTripper
	target   string
	test     *testing.T
	mu       sync.Mutex
	requests []geminiWireRequest
}

func (transport *geminiWireTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != transport.target || !strings.Contains(request.URL.Path, "streamGenerateContent") {
		return transport.base.RoundTrip(request)
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	if err := request.Body.Close(); err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	transport.mu.Lock()
	transport.requests = append(transport.requests, geminiWireRequest{body: canonical, rawBody: body, header: request.Header.Clone(), method: request.Method, url: request.URL.String()})
	index := len(transport.requests)
	transport.mu.Unlock()
	started := time.Now()
	trace := geminiWireTrace(transport.test, index, started)
	clone := request.Clone(httptrace.WithClientTrace(request.Context(), trace))
	clone.Body = io.NopCloser(bytes.NewReader(body))
	response, err := transport.base.RoundTrip(clone)
	if err != nil {
		transport.test.Logf("wire attempt=%d body_sha256=%x headers_ms=%.3f error_type=%T", index, sha256.Sum256(canonical), float64(time.Since(started).Microseconds())/1000, err)
		return nil, err
	}
	requestID := response.Header.Get("X-Request-ID")
	transport.test.Logf("wire attempt=%d body_sha256=%x status=%d headers_ms=%.3f request_id_present=%t request_id_sha256=%x", index, sha256.Sum256(canonical), response.StatusCode, float64(time.Since(started).Microseconds())/1000, requestID != "", sha256.Sum256([]byte(requestID)))
	return response, nil
}

func TestLiveGeminiWireComparison(t *testing.T) {
	runGeminiWireComparison(t, false)
}

func runGeminiWireComparison(t *testing.T, gatewayFirst bool) {
	env := liveEnvironment(t)
	if env["SHIP_PROVIDER_TYPE"] != "gemini" {
		t.Fatal("explicit Gemini provider required")
	}
	endpoint := installGeminiTimeline(t, env, false)
	transport := &geminiWireTransport{base: endpoint.base, target: endpoint.target, test: t}
	http.DefaultTransport = transport
	native := func(t *testing.T) { liveGeminiNativeToolStream(t, env) }
	gateway := func(t *testing.T) { liveToolStreaming(t, newLiveChainWithEnvironment(t, env)) }
	if gatewayFirst {
		t.Run("gateway", gateway)
		t.Run("native", native)
	} else {
		t.Run("native", native)
		t.Run("gateway", gateway)
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if len(transport.requests) != 2 {
		t.Fatalf("expected two real attempts, got %d", len(transport.requests))
	}
	first, second := transport.requests[0], transport.requests[1]
	rawEqual := bytes.Equal(first.rawBody, second.rawBody)
	equal := bytes.Equal(first.body, second.body) && first.method == second.method && first.url == second.url
	headersEqual := first.header.Get("Content-Type") == second.header.Get("Content-Type") && first.header.Get("X-Goog-Api-Key") == second.header.Get("X-Goog-Api-Key") && first.header.Get("Authorization") == second.header.Get("Authorization") && first.header.Get("User-Agent") == second.header.Get("User-Agent")
	t.Logf("wire gateway_first=%t raw_body_equal=%t canonical_body_equal=%t endpoint_method_equal=%t auth_content_type_user_agent_equal=%t first_raw_sha256=%x second_raw_sha256=%x", gatewayFirst, rawEqual, equal, first.method == second.method && first.url == second.url, headersEqual, sha256.Sum256(first.rawBody), sha256.Sum256(second.rawBody))
	if !rawEqual || !equal || !headersEqual {
		t.Fatal("native/gateway wire parameters differ; no causal equivalence claim")
	}
}

func geminiWireTrace(t *testing.T, index int, started time.Time) *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			t.Logf("wire attempt=%d connection_reused=%t idle_ms=%.3f", index, info.Reused, float64(info.IdleTime.Microseconds())/1000)
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			t.Logf("wire attempt=%d request_written_ms=%.3f error_type=%T", index, float64(time.Since(started).Microseconds())/1000, info.Err)
		},
		GotFirstResponseByte: func() {
			t.Logf("wire attempt=%d first_byte_ms=%.3f", index, float64(time.Since(started).Microseconds())/1000)
		},
	}
}
