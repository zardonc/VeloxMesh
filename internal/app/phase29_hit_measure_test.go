//go:build phase29preflight

package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"veloxmesh/internal/observability"
)

func TestPhase29LocalHitReadMeasure(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("isolated opt-in hit-path measurement")
	}
	env := liveEnvironment(t)
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	t.Setenv("PHASE29_MODE", "on")
	_, dsn, keyID := liveRepository(t)
	database := liveDatabase{dsn: dsn, keyID: keyID}
	application, token := liveApplicationWithDatabase(t, env, database)
	server := httptest.NewServer(application.Router)
	defer server.Close()
	options := liveRequestOptions{client: &http.Client{Timeout: 10 * time.Second}, url: server.URL, token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], system: liveAcceptanceSystem, question: "How long is the free trial?", origin: time.Now(), concurrent: 1}
	warm := liveRequest(options)
	if !warm.OK || warm.Hit {
		t.Fatalf("hit-path warmup: %+v", warm)
	}
	liveWaitStored(t, application, database, options.question)
	recorder := &liveRecorder{StubMetrics: observability.NewStubMetrics(), started: time.Now()}
	previous := observability.DefaultMetrics
	observability.DefaultMetrics = recorder
	defer func() { observability.DefaultMetrics = previous }()
	options.origin = recorder.started
	const requests = 100
	var hits int
	for index := 0; index < requests; index++ {
		sample := liveRequest(options)
		recorder.add(sample)
		if sample.OK && sample.Hit {
			hits++
		}
	}
	elapsed := time.Since(recorder.started)
	application.Close()
	liveSaveSamples(t, recorder, map[string]any{"type": "metadata", "model": os.Getenv("PHASE29_MODEL"), "mode": "hit-path-only", "count": requests, "elapsed_ms": float64(elapsed.Microseconds()) / 1000, "max_concurrency": 1})
	t.Logf("exact repeat hits=%d/%d, elapsed_ms=%.3f; component sample only, not low-hit release gate", hits, requests, float64(elapsed.Microseconds())/1000)
	if hits != requests {
		t.Fatal("hit-path repeat did not consistently hit; inspect raw failures")
	}
}
