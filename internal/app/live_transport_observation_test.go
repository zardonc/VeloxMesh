//go:build phase29preflight

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"veloxmesh/internal/observability"
)

type liveObservationRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip liveObservationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return roundTrip(req)
}

func liveObservationRecorder() *liveTiming {
	return &liveTiming{liveRecorder: &liveRecorder{StubMetrics: observability.NewStubMetrics(), started: time.Now()}}
}

func TestLiveTransportObservationClassifiesFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"eof", io.EOF}, {"unexpected_eof", io.ErrUnexpectedEOF},
		{"deadline", context.DeadlineExceeded}, {"deadline", os.ErrDeadlineExceeded},
		{"cancelled", context.Canceled}, {"connection_reset", syscall.ECONNRESET},
		{"connection_refused", syscall.ECONNREFUSED}, {"connection_closed", net.ErrClosed},
		{"connection_closed", io.ErrClosedPipe}, {"connection_closed", syscall.EPIPE},
		{"other", errors.New("secret-error-marker")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := liveObservationRecorder()
			failure := &url.Error{Op: "Post", URL: "https://secret-url-marker/", Err: fmt.Errorf("secret-error-marker: %w", tc.err)}
			transport := liveTimingTransport{recorder: recorder, base: liveObservationRoundTripper(func(*http.Request) (*http.Response, error) {
				return nil, failure
			})}
			req, _ := http.NewRequestWithContext(timingContext("diag-failure"), http.MethodPost, "https://secret-url-marker/", strings.NewReader("secret-body-marker"))
			req.Header.Set("Authorization", "Bearer secret-header-marker")
			response, err := transport.RoundTrip(req)
			if response != nil || err != failure {
				t.Fatal("observation changed the transport failure")
			}
			if len(recorder.transports) != 1 {
				t.Fatalf("transport observations = %d, want 1", len(recorder.transports))
			}
			sample := recorder.transports[0]
			if sample.Type != "transport" || sample.ID != "diag-failure" || sample.Kind != "upstream" || sample.Event != "round_trip" || sample.Success || sample.ErrorClass != tc.name {
				t.Fatalf("incorrect failure observation: %+v", sample)
			}
			if len(recorder.stages) != 1 || recorder.stages[0].Name != "upstream_headers" {
				t.Fatal("legacy stage changed")
			}
			liveAssertObservationSanitized(t, recorder)
		})
	}
}

func TestLiveTransportObservationRetainsWriteFailure(t *testing.T) {
	recorder := liveObservationRecorder()
	transport := liveTimingTransport{recorder: recorder, base: liveObservationRoundTripper(func(req *http.Request) (*http.Response, error) {
		trace := httptrace.ContextClientTrace(req.Context())
		trace.GotConn(httptrace.GotConnInfo{Reused: true, WasIdle: true, IdleTime: 125 * time.Millisecond})
		trace.WroteRequest(httptrace.WroteRequestInfo{Err: fmt.Errorf("secret-error-marker: %w", syscall.ECONNRESET)})
		return nil, io.ErrUnexpectedEOF
	})}
	req, _ := http.NewRequestWithContext(timingContext("diag-write"), http.MethodPost, "https://example.invalid/embeddings", nil)
	_, _ = transport.RoundTrip(req)
	if len(recorder.transports) != 3 {
		t.Fatalf("transport observations = %d, want 3", len(recorder.transports))
	}
	connection, write, result := recorder.transports[0], recorder.transports[1], recorder.transports[2]
	if connection.Event != "got_connection" || connection.Connection == nil || !connection.Connection.Reused || !connection.Connection.WasIdle || connection.Connection.IdleMS != 125 {
		t.Fatalf("missing connection facts: %+v", connection)
	}
	if write.Event != "wrote_request" || write.Success || write.ErrorClass != "connection_reset" || result.ErrorClass != "unexpected_eof" {
		t.Fatalf("write failure was lost: write=%+v result=%+v", write, result)
	}
	for i, name := range []string{"net_embedding/got_connection_reused", "net_embedding/request_written", "embedding_headers"} {
		if recorder.stages[i].Name != name || recorder.transports[i].Kind != "embedding" {
			t.Fatalf("legacy stage or embedding kind changed at %d", i)
		}
	}
	liveAssertObservationSanitized(t, recorder)
}

func TestLiveTransportObservationKeepAlive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { _, _ = io.WriteString(w, "ok") }))
	t.Cleanup(server.Close)
	base := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(base.CloseIdleConnections)
	recorder := liveObservationRecorder()
	client := &http.Client{Transport: liveTimingTransport{base: base, recorder: recorder}}
	for range 2 {
		req, _ := http.NewRequestWithContext(timingContext("diag-success"), http.MethodGet, server.URL, nil)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
	connections, writes, results := 0, 0, 0
	for _, sample := range recorder.transports {
		if !sample.Success || sample.ErrorClass != "" {
			t.Fatalf("successful traffic marked failed: %+v", sample)
		}
		switch sample.Event {
		case "got_connection":
			if sample.Connection == nil || sample.Connection.Reused != (connections == 1) || sample.Connection.WasIdle != (connections == 1) || sample.Connection.IdleMS < 0 {
				t.Fatalf("wrong connection reuse facts: %+v", sample)
			}
			connections++
		case "wrote_request":
			writes++
		case "round_trip":
			results++
		}
	}
	if connections != 2 || writes != 2 || results != 2 {
		t.Fatalf("connections/writes/results = %d/%d/%d", connections, writes, results)
	}
}

func TestLiveTransportObservationSkipsUncorrelated(t *testing.T) {
	recorder := liveObservationRecorder()
	transport := liveTimingTransport{recorder: recorder, base: liveObservationRoundTripper(func(req *http.Request) (*http.Response, error) {
		if httptrace.ContextClientTrace(req.Context()) != nil {
			t.Fatal("uncorrelated request gained a trace")
		}
		return nil, io.EOF
	})}
	req, _ := http.NewRequest(http.MethodGet, "https://example.invalid/", nil)
	_, err := transport.RoundTrip(req)
	if !errors.Is(err, io.EOF) || len(recorder.stages) != 0 || len(recorder.transports) != 0 {
		t.Fatal("uncorrelated transport behavior changed")
	}
}

func liveAssertObservationSanitized(t *testing.T, recorder *liveTiming) {
	t.Helper()
	encoded, err := json.Marshal(recorder.transports)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-error-marker", "secret-url-marker", "secret-header-marker", "secret-body-marker", "Authorization", "Bearer"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("transport observation contains sensitive input")
		}
	}
}
