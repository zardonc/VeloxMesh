//go:build phase29preflight

package app

import (
	"net/http"
	"testing"
)

func TestLiveGeminiWireFreshNativeFirst(t *testing.T) {
	liveGeminiWireCase(t, true, false)
}

func TestLiveGeminiWireFreshGatewayFirst(t *testing.T) {
	liveGeminiWireCase(t, true, true)
}

func TestLiveGeminiWireReuseNativeFirst(t *testing.T) {
	liveGeminiWireCase(t, false, false)
}

func TestLiveGeminiWireReuseGatewayFirst(t *testing.T) {
	liveGeminiWireCase(t, false, true)
}

func liveGeminiWireCase(t *testing.T, fresh, gatewayFirst bool) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("wire matrix requires an unwrapped transport")
	}
	transport := base.Clone()
	transport.DisableKeepAlives = fresh
	http.DefaultTransport = transport
	t.Cleanup(func() { transport.CloseIdleConnections(); http.DefaultTransport = base })
	t.Logf("wire_matrix keepalive_disabled=%t gateway_first=%t", fresh, gatewayFirst)
	runGeminiWireComparison(t, gatewayFirst)
}
