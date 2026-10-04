//go:build phase29preflight

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Failure cases: failed primary warmup; missing asynchronous seed persistence;
// foreground HTTP errors; invalid schedule; memo enabled in the no-memo arm.
// The seed and its writes must finish before the measured schedule begins.
const controlledQuietPeriod = 2 * time.Second

func TestLiveControlledOff(t *testing.T)      { controlledLoad(t, "off", "0") }
func TestLiveControlledOffAfter(t *testing.T) { controlledLoad(t, "off", "0") }
func TestLiveControlledOn(t *testing.T)       { controlledLoad(t, "on", "0") }
func TestLiveControlledMemo(t *testing.T)     { controlledLoad(t, "on", "128") }

func controlledLoad(t *testing.T, mode, capacity string) {
	t.Helper()
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", mode)
	t.Setenv("PHASE29_MEMO_CAPACITY", capacity)
	ttl := ""
	if capacity != "0" {
		ttl = "1m"
	}
	t.Setenv("PHASE29_MEMO_TTL", ttl)
	application, token := liveApplication(t, env)
	recorder := installLiveTiming(t, application, time.Now())
	t.Cleanup(func() { application.Close(); recorder.dump(t) })
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: server.URL,
		token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: liveWarmupIndex, origin: time.Now()}
	warm := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "warmup", "sample": warm})
	if !warm.OK {
		t.Fatal("primary warmup failed; formal measurement not started")
	}
	if mode == "on" {
		liveWaitStoreCount(t, recorder, 1)
	}
	time.Sleep(controlledQuietPeriod)
	shipLogJSON(t, map[string]any{"type": "effective_cache_profile", "memo_capacity": capacity,
		"memo_ttl": ttl, "seed_persisted": mode == "on", "quiet_ms": controlledQuietPeriod.Milliseconds()})
	shipLogJSON(t, map[string]any{"type": "measurement_start", "utc": time.Now().UTC().Format(time.RFC3339Nano)})
	options.origin = time.Now()
	failures := shipScheduledLoad(t, application, options)
	shipLogJSON(t, map[string]any{"type": "measurement_end", "utc": time.Now().UTC().Format(time.RFC3339Nano)})
	if failures != 0 {
		t.Fatalf("formal load failed requests=%d; samples retained", failures)
	}
}
