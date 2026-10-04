//go:build phase29preflight

package app

// Failure cases: a Redis deadline is misreported as model failure; a known
// unhealthy provider is admitted after dependency recovery; local state is lost.
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/observability"
)

func TestLiveHealthSnapshotDelayMatrix(t *testing.T) {
	liveEnvironment(t)
	for _, delay := range []time.Duration{40, 60, 100} {
		for _, unhealthy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dms/unhealthy=%t", delay, unhealthy), func(t *testing.T) {
				liveSnapshotDelayCase(t, delay*time.Millisecond, unhealthy)
			})
		}
	}
}

func liveSnapshotDelayCase(t *testing.T, delay time.Duration, unhealthy bool) {
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	client, err := hotstate.NewRedisClient(context.Background(), proxy.listener.Addr().String(), "", 0, fmt.Sprint("health-diagnostic-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var logs bytes.Buffer
	store := health.NewRedisStoreWithOptions(client, health.RedisStoreOptions{TTL: "1m", Logger: slog.New(slog.NewJSONHandler(&logs, nil))})
	store.EnsureProvider("diagnostic", 1, 1)
	if unhealthy {
		store.EndRequest("diagnostic", time.Millisecond, errors.New("injected provider failure"))
	}
	before := store.Snapshot("diagnostic")
	proxy.delay.Store(int64(delay))
	started := time.Now()
	during := store.Snapshot("diagnostic")
	elapsed := time.Since(started)
	after := store.Snapshot("diagnostic")
	local := store.Snapshots()["diagnostic"]
	if before.Status != after.Status || before.Status != local.Status || after.TotalFailures != before.TotalFailures {
		t.Fatal("dependency timeout changed provider health or counters")
	}
	if delay > 50*time.Millisecond && (during.Status != health.StatusUnhealthy || logs.Len() == 0) {
		t.Fatal("expected logged fail-closed snapshot deadline")
	}
	if delay < 50*time.Millisecond && during.Status != before.Status {
		t.Fatal("below-budget snapshot changed health")
	}
	shipLogJSON(t, map[string]any{"type": "health_snapshot_delay", "delay_ms": delay.Milliseconds(), "elapsed_ms": float64(elapsed.Microseconds()) / 1000,
		"before": before.Status, "during": during.Status, "after": after.Status, "local": local.Status, "errors": logs.String()})
}

func TestLiveRoutingHealthDependencyError(t *testing.T) {
	env := liveEnvironment(t)
	for _, delay := range []time.Duration{40, 60, 100} {
		t.Run(fmt.Sprintf("%dms", delay), func(t *testing.T) {
			liveRoutingDependencyCase(t, env, delay*time.Millisecond)
		})
	}
}

func liveRoutingDependencyCase(t *testing.T, env map[string]string, delay time.Duration) {
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	t.Setenv("PHASE29_REDIS_ADDR", proxy.listener.Addr().String())
	t.Setenv("PHASE29_REUSE_MODE", "disabled")
	chain := newLiveChainWithEnvironment(t, env)
	var armed atomic.Bool
	chain.timing.onStage = func(stage observability.StageMeasurement) {
		if stage.Name == "request_rules" && armed.CompareAndSwap(false, true) {
			proxy.delay.Store(int64(delay))
		}
	}
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, index: liveWarmupIndex, origin: time.Now()}
	during := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "routing_dependency_fault", "delay_ms": delay.Milliseconds(), "sample": during})
	if !armed.Load() || proxy.delay.Load() != 0 {
		t.Fatal("routing fault was not consumed")
	}
	expectedSettlements := 1
	if delay > 50*time.Millisecond {
		if during.Status != http.StatusServiceUnavailable || during.Error != "health_state_unavailable" {
			t.Errorf("Redis read failure should be explicit dependency error: %+v", during)
		}
	} else {
		expectedSettlements++
		if !during.OK {
			t.Errorf("within-budget health read failed: %+v", during)
		}
	}
	recovered := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "routing_dependency_recovery", "delay_ms": delay.Milliseconds(), "sample": recovered})
	if !recovered.OK {
		t.Fatal("routing did not recover after dependency delay")
	}
	liveAssertSettlement(t, chain, expectedSettlements)
}
