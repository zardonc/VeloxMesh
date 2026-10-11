//go:build phase29preflight

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/llm"
)

const (
	primaryIdleDefaultTimeout = 90 * time.Second
	primaryIdleBoundedTimeout = 4 * time.Second
	primaryIdleRequestLimit   = 3 * time.Second
	primaryIdleLowerGap       = 4 * time.Second
	primaryIdleBoundaryGap    = 5250 * time.Millisecond
	primaryIdleUpperGap       = 6500 * time.Millisecond
	primaryIdleBodyLimit      = 1 << 20
	primaryIdleMaxTokens      = 16
	primaryIdlePrompt         = "Reply with OK."
	primaryIdleRequests       = 6
)

type primaryIdleResponse struct {
	Status        int    `json:"status"`
	Success       bool   `json:"success"`
	BodyComplete  bool   `json:"body_complete"`
	ResponseHash  string `json:"response_sha256,omitempty"`
	ResponseBytes int    `json:"response_bytes"`
	Connection    string `json:"connection_header,omitempty"`
	KeepAlive     string `json:"keep_alive,omitempty"`
	Protocol      string `json:"protocol,omitempty"`
	ResponseClose bool   `json:"response_close"`
	ErrorClass    string `json:"error_class,omitempty"`
	Error         string `json:"error,omitempty"`
}

type primaryIdleConnection struct {
	UTC     time.Time `json:"utc"`
	Local   string    `json:"local_address"`
	Remote  string    `json:"remote_address"`
	Reused  bool      `json:"reused"`
	WasIdle bool      `json:"was_idle"`
	IdleMS  float64   `json:"idle_ms"`
}

func primaryIdleReadResponse(response *http.Response, requestErr error) primaryIdleResponse {
	if response == nil {
		if requestErr == nil {
			requestErr = errors.New("missing HTTP response")
		}
		return primaryIdleFailure(primaryIdleResponse{}, requestErr, liveTransportErrorClass(requestErr))
	}
	result := primaryIdleResponse{Status: response.StatusCode, Connection: response.Header.Get("Connection"),
		KeepAlive: response.Header.Get("Keep-Alive"), Protocol: response.Proto, ResponseClose: response.Close}
	if response.Body == nil {
		return primaryIdleFailure(result, errors.New("missing response body"), "invalid_response")
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, primaryIdleBodyLimit+1))
	closeErr := response.Body.Close()
	result.ResponseHash, result.ResponseBytes = primaryIdleHash(body), len(body)
	result.BodyComplete = readErr == nil && len(body) <= primaryIdleBodyLimit
	if err := errors.Join(requestErr, readErr, closeErr); err != nil {
		return primaryIdleFailure(result, err, liveTransportErrorClass(err))
	}
	if len(body) > primaryIdleBodyLimit {
		return primaryIdleFailure(result, errors.New("response exceeds diagnostic body limit"), "body_limit")
	}
	if response.StatusCode != http.StatusOK {
		return primaryIdleFailure(result, fmt.Errorf("HTTP status %d", response.StatusCode), "http_status")
	}
	var completion llm.ChatCompletionResponse
	if err := json.Unmarshal(body, &completion); err != nil || len(completion.Choices) == 0 {
		return primaryIdleFailure(result, errors.New("invalid chat completion response"), "invalid_response")
	}
	result.Success = true
	return result
}

func primaryIdleObserveConnection(info httptrace.GotConnInfo) primaryIdleConnection {
	result := primaryIdleConnection{UTC: time.Now().UTC(), Reused: info.Reused, WasIdle: info.WasIdle,
		IdleMS: float64(info.IdleTime) / float64(time.Millisecond)}
	if info.Conn != nil {
		result.Local, result.Remote = info.Conn.LocalAddr().String(), info.Conn.RemoteAddr().String()
	}
	return result
}

func primaryIdleFailure(result primaryIdleResponse, err error, class string) primaryIdleResponse {
	result.ErrorClass, result.Error = class, primaryIdleSafeError(err)
	return result
}

func primaryIdleSafeError(err error) string {
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		return primaryIdleSafeError(requestErr.Err)
	}
	return err.Error()
}

func primaryIdleHash(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

type primaryIdleConfig struct {
	arm, endpoint, model, key string
	body                      []byte
	idleTimeout               time.Duration
}

type primaryIdlePair struct {
	config   primaryIdleConfig
	number   int
	gap      time.Duration
	base     *http.Transport
	recorder *liveTiming
}

type primaryIdleRequestRecord struct {
	Type          string                  `json:"type"`
	ID            string                  `json:"request_id"`
	Arm           string                  `json:"arm"`
	Pair          int                     `json:"pair"`
	Position      int                     `json:"position"`
	Endpoint      string                  `json:"endpoint"`
	Model         string                  `json:"model"`
	StartedUTC    time.Time               `json:"started_utc"`
	EndedUTC      time.Time               `json:"ended_utc"`
	ElapsedMS     float64                 `json:"elapsed_ms"`
	PlannedIdleMS float64                 `json:"planned_idle_ms"`
	ActualIdleMS  float64                 `json:"actual_idle_ms"`
	RequestHash   string                  `json:"request_sha256"`
	Connections   []primaryIdleConnection `json:"connections"`
	primaryIdleResponse
}

type primaryIdleTrace struct {
	mu          sync.Mutex
	connections []primaryIdleConnection
}

func (trace *primaryIdleTrace) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		observation := primaryIdleObserveConnection(info)
		trace.mu.Lock()
		trace.connections = append(trace.connections, observation)
		trace.mu.Unlock()
	}}
}

func (trace *primaryIdleTrace) snapshot() []primaryIdleConnection {
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return append([]primaryIdleConnection(nil), trace.connections...)
}

func TestLivePrimaryIdleDefault(t *testing.T) { runPrimaryIdle(t, primaryIdleDefaultTimeout) }
func TestLivePrimaryIdleBounded(t *testing.T) { runPrimaryIdle(t, primaryIdleBoundedTimeout) }

func runPrimaryIdle(t *testing.T, idleTimeout time.Duration) {
	t.Helper()
	env := liveEnvironment(t)
	if os.Getenv("PHASE29_PRIMARY_IDLE") != "true" {
		t.Skip("primary idle diagnostic requires explicit opt-in")
	}
	config, err := primaryIdleConfiguration(env, idleTimeout)
	if err != nil {
		t.Fatal(err)
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("primary idle diagnostic requires an unmodified default transport")
	}
	recorder := liveObservationRecorder()
	defer recorder.dump(t)
	shipLogJSON(t, map[string]any{"type": "primary_idle_plan", "arm": config.arm, "utc": time.Now().UTC(), "timing_origin_utc": recorder.started.UTC(),
		"endpoint": config.endpoint, "model": config.model, "requests": primaryIdleRequests, "acceptance": false,
		"idle_conn_timeout_ms": idleTimeout.Milliseconds(), "request_limit_ms": primaryIdleRequestLimit.Milliseconds(),
		"idle_gaps_ms": []int64{primaryIdleLowerGap.Milliseconds(), primaryIdleBoundaryGap.Milliseconds(), primaryIdleUpperGap.Milliseconds()}, "keepalive": true, "retry": false,
		"request_sha256": primaryIdleHash(config.body), "max_tokens": primaryIdleMaxTokens})
	failures := 0
	for index, gap := range []time.Duration{primaryIdleLowerGap, primaryIdleBoundaryGap, primaryIdleUpperGap} {
		failures += primaryIdleRunPair(t, primaryIdlePair{config: config, number: index + 1, gap: gap, base: base, recorder: recorder})
	}
	shipLogJSON(t, map[string]any{"type": "primary_idle_summary", "arm": config.arm, "utc": time.Now().UTC(),
		"requests": primaryIdleRequests, "failures": failures, "acceptance": false})
	if failures > 0 {
		t.Errorf("primary idle diagnostic retained %d failed requests out of %d", failures, primaryIdleRequests)
	}
}

func primaryIdleConfiguration(env map[string]string, idleTimeout time.Duration) (primaryIdleConfig, error) {
	endpoint, err := url.Parse(env["SANS_BASE_URL"])
	if err != nil || endpoint == nil || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return primaryIdleConfig{}, errors.New("primary idle diagnostic requires a credential-free loopback HTTP base URL")
	}
	address := net.ParseIP(endpoint.Hostname())
	if endpoint.Hostname() != "localhost" && (address == nil || !address.IsLoopback()) {
		return primaryIdleConfig{}, errors.New("primary idle diagnostic only permits the existing loopback primary route")
	}
	arm, model := os.Getenv("PHASE29_PRIMARY_IDLE_ARM"), env["SANS_PRIMARY_DEFAULT_MODEL"]
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`).MatchString(arm) || strings.TrimSpace(model) == "" {
		return primaryIdleConfig{}, errors.New("primary idle diagnostic requires an arm label and injected primary model")
	}
	if providerType := env["SHIP_PROVIDER_TYPE"]; providerType != "" && providerType != "openai-compatible" {
		return primaryIdleConfig{}, errors.New("primary idle diagnostic requires an OpenAI-compatible primary")
	}
	body, err := json.Marshal(map[string]any{"model": model, "stream": false, "temperature": 0,
		"max_tokens": primaryIdleMaxTokens, "messages": []map[string]string{{"role": "user", "content": primaryIdlePrompt}}})
	if err != nil {
		return primaryIdleConfig{}, err
	}
	return primaryIdleConfig{arm: arm, endpoint: strings.TrimRight(endpoint.String(), "/") + "/chat/completions",
		model: model, key: env["SANS_PRIMARY_API_KEY"], body: body, idleTimeout: idleTimeout}, nil
}

func primaryIdleRunPair(t *testing.T, pair primaryIdlePair) int {
	t.Helper()
	transport := pair.base.Clone()
	transport.IdleConnTimeout, transport.DisableKeepAlives = pair.config.idleTimeout, false
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: primaryIdleRequestLimit, Transport: liveTimingTransport{base: transport, recorder: pair.recorder},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	first := primaryIdleRequest(client, pair, 1)
	shipLogJSON(t, first)
	time.Sleep(pair.gap)
	second := primaryIdleRequest(client, pair, 2)
	second.ActualIdleMS = float64(second.StartedUTC.Sub(first.EndedUTC)) / float64(time.Millisecond)
	shipLogJSON(t, second)
	failures := 0
	for _, result := range []primaryIdleRequestRecord{first, second} {
		if !result.Success {
			failures++
		}
	}
	return failures
}

func primaryIdleRequest(client *http.Client, pair primaryIdlePair, position int) primaryIdleRequestRecord {
	id := fmt.Sprintf("%s-p%d-r%d", pair.config.arm, pair.number, position)
	started := time.Now()
	trace := &primaryIdleTrace{}
	ctx, cancel := context.WithTimeout(timingContext(id), primaryIdleRequestLimit)
	defer cancel()
	ctx = httptrace.WithClientTrace(ctx, trace.clientTrace())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, pair.config.endpoint, bytes.NewReader(pair.config.body))
	var response *http.Response
	if err == nil {
		request.Header.Set("Content-Type", "application/json")
		if pair.config.key != "" {
			request.Header.Set("Authorization", "Bearer "+pair.config.key)
		}
		request.GetBody = nil // Do not replay even a zero-byte failed write in this bounded diagnostic.
		response, err = client.Do(request)
	}
	result := primaryIdleReadResponse(response, err)
	ended := time.Now()
	return primaryIdleRequestRecord{Type: "primary_idle_request", ID: id, Arm: pair.config.arm, Pair: pair.number, Position: position,
		Endpoint: pair.config.endpoint, Model: pair.config.model, StartedUTC: started.UTC(), EndedUTC: ended.UTC(),
		ElapsedMS: float64(ended.Sub(started)) / float64(time.Millisecond), PlannedIdleMS: float64(pair.gap) / float64(time.Millisecond),
		RequestHash: primaryIdleHash(pair.config.body), Connections: trace.snapshot(), primaryIdleResponse: result}
}
