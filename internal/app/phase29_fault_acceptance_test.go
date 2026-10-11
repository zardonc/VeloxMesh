//go:build phase29preflight

package app

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/observability"
)

func TestPhase29LocalEmbeddingFault(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("isolated opt-in fault acceptance")
	}
	env := liveEnvironment(t)
	upstream, err := url.Parse(os.Getenv("PHASE29_EMBEDDING_BASE_URL"))
	if err != nil || upstream.Host == "" {
		t.Fatal("invalid local embedding URL")
	}
	var failing atomic.Bool
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			http.Error(w, "injected isolated embedding failure", http.StatusServiceUnavailable)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer embeddingServer.Close()
	t.Setenv("PHASE29_EMBEDDING_BASE_URL", embeddingServer.URL)
	t.Setenv("PHASE29_MODE", "on")
	application, token := liveApplication(t, env)
	server := httptest.NewServer(application.Router)
	defer server.Close()
	recorder := &liveRecorder{StubMetrics: observability.NewStubMetrics(), started: time.Now()}
	previous := observability.DefaultMetrics
	observability.DefaultMetrics = recorder
	defer func() { observability.DefaultMetrics = previous }()
	failing.Store(true)
	response := liveRequest(liveRequestOptions{client: &http.Client{Timeout: 10 * time.Second}, url: server.URL, token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: 1, origin: recorder.started})
	application.Close()
	if !response.OK || response.Hit {
		t.Fatalf("fault forwarding: %+v", response)
	}
	liveLogEmbeddingFault(t, recorder, response)
}

func liveLogEmbeddingFault(t *testing.T, recorder *liveRecorder, response liveSample) {
	t.Helper()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	var failures int
	for _, sample := range recorder.samples {
		if sample.Type == "operation" && !sample.OK {
			failures++
		}
		if sample.Type == "outcome" {
			t.Logf("reason=%s", sample.Name)
		}
	}
	if failures == 0 {
		t.Fatal("injected embedding failure was not observed")
	}
	t.Logf("HTTP=200 cache_hit=false complete_response_ms=%.3f failed_optional_operations=%d", response.ElapsedMS, failures)
}
