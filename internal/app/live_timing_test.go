//go:build phase29preflight

package app

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/observability"
)

type liveStage struct {
	Type      string  `json:"type"`
	ID        string  `json:"request_id"`
	Name      string  `json:"name"`
	StartMS   float64 `json:"start_ms"`
	ElapsedMS float64 `json:"elapsed_ms"`
}

type liveTiming struct {
	*liveRecorder
	stages  []liveStage
	onStage func(observability.StageMeasurement)
}

func (r *liveTiming) RecordStage(sample observability.StageMeasurement) {
	r.mu.Lock()
	r.stages = append(r.stages, liveStage{Type: "stage", ID: sample.ID, Name: sample.Name,
		StartMS: float64(sample.Started.Sub(r.started).Microseconds()) / 1000, ElapsedMS: float64(sample.Elapsed.Microseconds()) / 1000})
	r.mu.Unlock()
	if r.onStage != nil {
		r.onStage(sample)
	}
}

func (r *liveTiming) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := req.Header.Get("X-Request-ID")
		if id == "" {
			tID := time.Now().UnixNano()
			id = "timing-" + time.Unix(0, tID).Format("150405.000000000")
		}
		ctx := observability.WithTimingID(req.Context(), id)
		clone := req.Clone(ctx)
		clone.Header.Set("X-Request-ID", id)
		defer observability.Stage(ctx, "http_handler")()
		next.ServeHTTP(w, clone)
	})
}

type liveTimingTransport struct {
	base     http.RoundTripper
	recorder *liveTiming
}

func (transport liveTimingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	id := observability.TimingID(req.Context())
	if id == "" {
		return transport.base.RoundTrip(req)
	}
	kind := "upstream"
	if strings.Contains(req.URL.Path, "embeddings") {
		kind = "embedding"
	}
	started := time.Now()
	trace := transport.clientTrace(id, kind, started)
	response, err := transport.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	transport.recorder.RecordStage(observability.StageMeasurement{ID: id, Name: kind + "_headers", Started: started, Elapsed: time.Since(started)})
	if err != nil {
		return nil, err
	}
	response.Body = &liveTimingBody{ReadCloser: response.Body, finish: func() {
		transport.recorder.RecordStage(observability.StageMeasurement{ID: id, Name: kind + "_http", Started: started, Elapsed: time.Since(started)})
	}}
	return response, nil
}

func (transport liveTimingTransport) clientTrace(id, kind string, started time.Time) *httptrace.ClientTrace {
	record := func(event string) {
		transport.recorder.RecordStage(observability.StageMeasurement{ID: id, Name: "net_" + kind + "/" + event, Started: started, Elapsed: time.Since(started)})
	}
	return &httptrace.ClientTrace{
		GetConn: func(string) { record("get_connection") },
		GotConn: func(info httptrace.GotConnInfo) {
			event := "got_connection_new"
			if info.Reused {
				event = "got_connection_reused"
			}
			record(event)
		},
		DNSStart: func(httptrace.DNSStartInfo) { record("dns_start") }, DNSDone: func(httptrace.DNSDoneInfo) { record("dns_done") },
		ConnectStart: func(string, string) { record("connect_start") }, ConnectDone: func(string, string, error) { record("connect_done") },
		TLSHandshakeStart: func() { record("tls_start") }, TLSHandshakeDone: func(tls.ConnectionState, error) { record("tls_done") },
		WroteRequest: func(httptrace.WroteRequestInfo) { record("request_written") }, GotFirstResponseByte: func() { record("first_byte") },
	}
}

type liveTimingBody struct {
	io.ReadCloser
	once   sync.Once
	finish func()
}

func (body *liveTimingBody) Read(buffer []byte) (int, error) {
	n, err := body.ReadCloser.Read(buffer)
	if err != nil {
		body.once.Do(body.finish)
	}
	return n, err
}

func (body *liveTimingBody) Close() error {
	defer body.once.Do(body.finish)
	return body.ReadCloser.Close()
}

func installLiveTiming(t *testing.T, application *App, origin time.Time) *liveTiming {
	t.Helper()
	recorder := &liveTiming{liveRecorder: &liveRecorder{StubMetrics: observability.NewStubMetrics(), started: origin}}
	if os.Getenv("PHASE29_TIMING_DISABLED") == "true" {
		return recorder
	}
	previousMetrics, previousTransport := observability.DefaultMetrics, http.DefaultTransport
	observability.DefaultMetrics = recorder
	http.DefaultTransport = liveTimingTransport{base: previousTransport, recorder: recorder}
	if application != nil {
		application.Router = recorder.handler(application.Router)
	}
	t.Cleanup(func() { observability.DefaultMetrics = previousMetrics; http.DefaultTransport = previousTransport })
	return recorder
}

func (r *liveTiming) dump(t *testing.T) {
	r.mu.Lock()
	stages, operations := append([]liveStage(nil), r.stages...), append([]liveSample(nil), r.samples...)
	r.mu.Unlock()
	for _, sample := range stages {
		shipLogJSON(t, sample)
	}
	for _, sample := range operations {
		shipLogJSON(t, sample)
	}
}

func timingContext(id string) context.Context {
	return observability.WithTimingID(context.Background(), id)
}
