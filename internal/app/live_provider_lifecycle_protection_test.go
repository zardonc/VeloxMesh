//go:build phase29preflight

package app

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/config"
)

func TestLiveProviderOverallTimeout(t *testing.T) {
	env := liveEnvironment(t)
	proxy := newLiveGuardProxy(t, env["SANS_BASE_URL"], "body")
	chain := liveProtectedChain(t, liveProtectionInputs{env: env, proxy: proxy,
		profile: config.ProviderProtectionConfig{FirstByteTimeout: "3s", FirstContentTimeout: "3s", TotalTimeout: "1s", MaxInflight: 1}})
	started := time.Now()
	body, status := liveGuardCall(t, chain, false)
	if status != http.StatusGatewayTimeout || !bytes.Contains(body, []byte("provider_overall_timeout")) {
		t.Fatalf("overall deadline classification missing: status=%d body=%s", status, body)
	}
	if time.Since(started) > liveGuardDeadline+liveGuardSlack {
		t.Fatal("overall deadline did not bound response body wait")
	}
	liveAssertSettlement(t, chain, 0)
	shipLogJSON(t, map[string]any{"type": "provider_overall_timeout", "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000})
}

func TestLiveProviderRegistryCapacity(t *testing.T) {
	env := liveEnvironment(t)
	proxy := newLiveGuardProxy(t, env["SANS_BASE_URL"], "headers")
	chain := liveProtectedChain(t, liveProtectionInputs{env: env, proxy: proxy,
		profile: config.ProviderProtectionConfig{TotalTimeout: "5s", MaxInflight: 1}})
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, index: 1, origin: time.Now()}
	done := make(chan shipSample, 1)
	go func() { done <- shipRequest(options) }()
	select {
	case <-proxy.entered:
	case <-time.After(liveGuardFaultDelay):
		t.Fatal("registry replacement probe unavailable")
	}
	manager := chain.app.RuntimeProviderManager
	if err := manager.ActivateStatic(chain.app.Config.Providers, manager.Snapshot().Registry.List()); err != nil {
		t.Fatal(err)
	}
	if sample := shipRequest(options); sample.Status != http.StatusTooManyRequests || !strings.Contains(sample.Error, "provider_concurrency_full") {
		t.Fatalf("registry replacement reset active permits: %+v", sample)
	}
	if sample := <-done; !sample.OK {
		t.Fatalf("active attempt failed during registry replacement: %+v", sample)
	}
	if sample := shipRequest(options); !sample.OK {
		t.Fatalf("replacement registry did not recover capacity: %+v", sample)
	}
	liveAssertSettlement(t, chain, 2)
	shipLogJSON(t, map[string]any{"type": "provider_registry_capacity", "limit": 1, "active_permit_preserved": true, "recovered": true})
}
