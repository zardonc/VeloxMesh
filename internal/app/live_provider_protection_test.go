//go:build phase29preflight

package app

// Failure matrix: no upstream byte; headers then stalled/partial body; SSE
// heartbeats without content; mid-stream silence; saturation; cancel/recovery.
// The proxy forwards the actual model's bytes; it does not fabricate answers.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/config"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

const liveGuardDeadline = time.Second
const liveGuardFaultDelay = 2 * time.Second
const liveGuardSlack = time.Second

func TestLiveProviderDeadlineProtection(t *testing.T) {
	env := liveEnvironment(t)
	for _, fault := range []struct {
		mode, code string
		stream     bool
	}{
		{"headers", "provider_first_byte_timeout", false},
		{"body", "provider_first_content_timeout", false},
		{"partial", "provider_first_content_timeout", false},
		{"heartbeats", "provider_first_content_timeout", true},
		{"stream_idle", "provider_stream_idle_timeout", true},
	} {
		t.Run(fault.mode, func(t *testing.T) {
			proxy := newLiveGuardProxy(t, env["SANS_BASE_URL"], fault.mode)
			profile := config.ProviderProtectionConfig{FirstByteTimeout: "1s", FirstContentTimeout: "3s", StreamIdleTimeout: "1s", TotalTimeout: "5s", MaxInflight: 1}
			if fault.mode != "headers" {
				profile.FirstByteTimeout, profile.FirstContentTimeout = "3s", "1s"
			}
			chain := liveProtectedChain(t, liveProtectionInputs{env: env, proxy: proxy, profile: profile})
			started := time.Now()
			body, status := liveGuardCall(t, chain, fault.stream)
			if !bytes.Contains(body, []byte(fault.code)) || (!fault.stream && status != http.StatusGatewayTimeout) {
				t.Fatalf("deadline classification missing: status=%d body=%s", status, body)
			}
			if time.Since(started) > liveGuardDeadline+liveGuardSlack {
				t.Fatal("phase deadline did not bound the real request")
			}
			liveAssertSettlement(t, chain, 0)
			shipLogJSON(t, map[string]any{"type": "provider_protection", "fault": fault.mode, "expected_error": fault.code, "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000})
		})
	}
}

type liveProtectionInputs struct {
	env              map[string]string
	proxy            *liveGuardProxy
	profile          config.ProviderProtectionConfig
	protectEmbedding bool
}

func liveProtectedChain(t *testing.T, inputs liveProtectionInputs) liveChain {
	t.Helper()
	t.Setenv("PHASE29_MODE", "off")
	profiles := map[string]config.ProviderProtectionConfig{"sans-primary": inputs.profile}
	if inputs.protectEmbedding {
		profiles["phase29-embedding"] = inputs.profile
	}
	encoded, err := json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PHASE29_PROVIDER_PROTECTION", string(encoded))
	env := maps.Clone(inputs.env)
	env["SANS_BASE_URL"] = inputs.proxy.url + "/v1"
	return newLiveChainWithEnvironment(t, env)
}

func TestLiveProviderSharedResource(t *testing.T) {
	env := liveEnvironment(t)
	proxy := newLiveGuardProxy(t, env["SANS_BASE_URL"], "headers")
	chain := liveProtectedChain(t, liveProtectionInputs{env: env, proxy: proxy, protectEmbedding: true,
		profile: config.ProviderProtectionConfig{TotalTimeout: "5s", MaxInflight: 1, ResourceGroup: "isolated-shared-backend"}})
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, index: 1, origin: time.Now()}
	done := make(chan shipSample, 1)
	go func() { done <- shipRequest(options) }()
	select {
	case <-proxy.entered:
	case <-time.After(liveGuardFaultDelay):
		t.Fatal("shared resource probe unavailable")
	}
	adapter, err := chain.app.RuntimeProviderManager.Snapshot().Registry.Get("phase29-embedding")
	if err != nil {
		t.Fatal(err)
	}
	embed, ok := adapter.(providers.EmbedAdapter)
	if !ok {
		t.Fatal("protection lost embedding capability")
	}
	request := &llm.EmbeddingRequest{Model: os.Getenv("PHASE29_MODEL"), Input: []string{"shared capacity probe"}}
	if response, err := embed.Embed(context.Background(), request); response != nil || err == nil || !strings.Contains(err.Error(), "provider_concurrency_full") {
		t.Fatalf("separate provider IDs oversubscribed shared backend: %v", err)
	}
	if sample := <-done; !sample.OK {
		t.Fatalf("held shared attempt failed: %+v", sample)
	}
	if response, err := embed.Embed(context.Background(), request); err != nil || response == nil || len(response.Data) != 1 {
		t.Fatalf("shared embedding permit failed to recover: %v", err)
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "shared_provider_resource", "limit": 1, "rejected_embedding": true, "recovered": true})
}

func liveGuardCall(t *testing.T, chain liveChain, stream bool) ([]byte, int) {
	t.Helper()
	body, err := shipRequestBody(liveRequestOptions{model: chain.model, index: 1})
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatal(err)
	}
	request["stream"] = stream
	body, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, chain.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+chain.token)
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return result, response.StatusCode
}

func TestLiveProviderConcurrencyRecovery(t *testing.T) {
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
		t.Fatal("real provider did not enter the held attempt")
	}
	started := time.Now()
	rejected := shipRequest(options)
	if rejected.Status != http.StatusTooManyRequests || !strings.Contains(rejected.Error, "provider_concurrency_full") || time.Since(started) > liveGuardSlack {
		t.Fatalf("saturation did not fast-fail: %+v", rejected)
	}
	if first := <-done; !first.OK {
		t.Fatalf("admitted request failed: %+v", first)
	}
	recovered := shipRequest(options)
	if !recovered.OK {
		t.Fatalf("permit did not recover: %+v", recovered)
	}
	liveAssertSettlement(t, chain, 2)
	shipLogJSON(t, map[string]any{"type": "provider_concurrency", "limit": 1, "rejected": 1, "recovered": true, "settlements": 2})
}

func TestLiveProviderCancellationRecovery(t *testing.T) {
	env := liveEnvironment(t)
	proxy := newLiveGuardProxy(t, env["SANS_BASE_URL"], "headers")
	chain := liveProtectedChain(t, liveProtectionInputs{env: env, proxy: proxy,
		profile: config.ProviderProtectionConfig{TotalTimeout: "5s", MaxInflight: 1}})
	body, err := shipRequestBody(liveRequestOptions{model: chain.model, index: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, chain.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+chain.token)
	request.Header.Set("Content-Type", "application/json")
	done := make(chan error, 1)
	go func() {
		response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(request)
		if response != nil {
			response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-proxy.entered:
	case <-time.After(liveGuardFaultDelay):
		t.Fatal("real cancellation attempt unavailable")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, index: 2, origin: time.Now()}
	deadline := time.Now().Add(liveGuardFaultDelay + liveGuardSlack)
	for {
		sample := shipRequest(options)
		if sample.OK {
			break
		}
		if sample.Status != http.StatusTooManyRequests || time.Now().After(deadline) {
			t.Fatalf("cancelled permit did not recover: %+v", sample)
		}
		time.Sleep(livePollInterval)
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "provider_cancellation", "recovered": true, "settlements": 1})
}
