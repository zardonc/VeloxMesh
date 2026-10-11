//go:build phase29preflight

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Failure cases and protocol are preregistered in phase29-first-batch.md.
const primaryExtendedWarmups = 8

func TestLivePrimaryOne01(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne02(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne03(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne04(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne05(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne06(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne07(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne08(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne09(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne10(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne11(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryOne12(t *testing.T)   { primaryWarmupLoad(t, 1) }
func TestLivePrimaryEight01(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight02(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight03(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight04(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight05(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight06(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight07(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight08(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight09(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight10(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight11(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }
func TestLivePrimaryEight12(t *testing.T) { primaryWarmupLoad(t, primaryExtendedWarmups) }

func primaryWarmupLoad(t *testing.T, warmCount int) {
	t.Helper()
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", "off")
	t.Setenv("PHASE29_MEMO_CAPACITY", "0")
	t.Setenv("PHASE29_MEMO_TTL", "")
	application, token := liveApplication(t, env)
	recorder := installLiveTiming(t, application, time.Now())
	t.Cleanup(func() { application.Close(); recorder.dump(t) })
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: server.URL,
		token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: liveWarmupIndex, origin: time.Now()}
	for index := 0; index < warmCount; index++ {
		warm := shipRequest(options)
		shipLogJSON(t, map[string]any{"type": "warmup", "ordinal": index + 1, "sample": warm})
		if !warm.OK {
			t.Fatal("primary warmup failed; formal measurement not started")
		}
	}
	time.Sleep(controlledQuietPeriod)
	shipLogJSON(t, map[string]any{"type": "effective_cache_profile", "mode": "off", "memo_capacity": "0",
		"warm_count": warmCount, "quiet_ms": controlledQuietPeriod.Milliseconds()})
	shipLogJSON(t, map[string]any{"type": "measurement_start", "utc": time.Now().UTC().Format(time.RFC3339Nano)})
	formalOptions := options
	formalOptions.origin = time.Now()
	failures := shipScheduledLoad(t, application, formalOptions)
	shipLogJSON(t, map[string]any{"type": "measurement_end", "utc": time.Now().UTC().Format(time.RFC3339Nano)})
	if failures != 0 {
		t.Fatalf("formal load failed requests=%d; samples retained", failures)
	}
}
