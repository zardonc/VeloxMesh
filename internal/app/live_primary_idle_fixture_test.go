//go:build phase29preflight

package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Failure contract: a sent request can fail before headers, mid-body, on close,
// with a non-200 status or invalid success body. Preserve every outcome and a
// partial-body hash without retaining credentials, headers or response payloads.
func TestPrimaryIdleRecordingFailures(t *testing.T) {
	cases := []struct {
		name     string
		response *http.Response
		err      error
		class    string
	}{
		{"transport_eof", nil, &url.Error{Op: "Post", URL: "http://user:secret@localhost/?token=secret", Err: io.EOF}, "eof"},
		{"partial_body", primaryIdleFixtureResponse(http.StatusOK, &primaryIdleFaultBody{}), nil, "unexpected_eof"},
		{"close_failure", primaryIdleFixtureResponse(http.StatusOK, primaryIdleCloseFailure{}), nil, "connection_closed"},
		{"http_failure", primaryIdleFixtureResponse(http.StatusBadGateway, io.NopCloser(strings.NewReader(`{"error":"secret-body"}`))), nil, "http_status"},
		{"malformed_success", primaryIdleFixtureResponse(http.StatusOK, io.NopCloser(strings.NewReader(`secret-body`))), nil, "invalid_response"},
		{"empty_success", primaryIdleFixtureResponse(http.StatusOK, io.NopCloser(strings.NewReader(`{"choices":[]}`))), nil, "invalid_response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := primaryIdleReadResponse(tc.response, tc.err)
			if result.Success || result.ErrorClass != tc.class || result.Error == "" {
				t.Fatalf("failure lost: %+v", result)
			}
			if tc.response != nil && (result.ResponseHash == "" || result.Status != tc.response.StatusCode) {
				t.Fatalf("response evidence lost: %+v", result)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "Authorization") {
				t.Fatal("recording retained sensitive data")
			}
		})
	}
}

func TestPrimaryIdleRecordingSuccess(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"secret-body"}}]}`
	response := primaryIdleFixtureResponse(http.StatusOK, io.NopCloser(strings.NewReader(body)))
	response.Header.Set("Connection", "keep-alive")
	response.Header.Set("Keep-Alive", "timeout=5")
	response.Header.Set("Authorization", "Bearer secret-header")
	result := primaryIdleReadResponse(response, nil)
	digest := sha256.Sum256([]byte(body))
	if !result.Success || !result.BodyComplete || result.ResponseHash != hex.EncodeToString(digest[:]) || result.ResponseBytes != len(body) {
		t.Fatalf("incomplete successful response: %+v", result)
	}
	if result.KeepAlive != "timeout=5" || result.Connection != "keep-alive" || result.Error != "" {
		t.Fatalf("keepalive/result changed: %+v", result)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "Authorization") {
		t.Fatal("recording retained response payload or authorization")
	}
}

func TestPrimaryIdleRecordingConnection(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	before := time.Now().UTC()
	result := primaryIdleObserveConnection(httptrace.GotConnInfo{Conn: local, Reused: true, WasIdle: true, IdleTime: 5250 * time.Millisecond})
	if result.UTC.Before(before) || result.UTC.Location() != time.UTC || result.Local != local.LocalAddr().String() || result.Remote != local.RemoteAddr().String() {
		t.Fatalf("connection identity/time lost: %+v", result)
	}
	if !result.Reused || !result.WasIdle || result.IdleMS != 5250 {
		t.Fatalf("idle facts lost: %+v", result)
	}
}

func primaryIdleFixtureResponse(status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}
}

type primaryIdleFaultBody struct{}

func (body *primaryIdleFaultBody) Read(buffer []byte) (int, error) {
	return copy(buffer, "partial-secret-body"), io.ErrUnexpectedEOF
}

func (*primaryIdleFaultBody) Close() error { return nil }

type primaryIdleCloseFailure struct{}

func (primaryIdleCloseFailure) Read(buffer []byte) (int, error) { return 0, io.EOF }
func (primaryIdleCloseFailure) Close() error                    { return io.ErrClosedPipe }
