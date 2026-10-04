//go:build phase29preflight

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"
)

type liveGuardProxy struct {
	url     string
	entered chan struct{}
}

func newLiveGuardProxy(t *testing.T, target, mode string) *liveGuardProxy {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = ""
	entered := make(chan struct{}, 16)
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.FlushInterval = -1
	proxy.ModifyResponse = func(response *http.Response) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		if mode == "headers" {
			return liveGuardDelay(response.Request.Context(), liveGuardFaultDelay)
		}
		response.Body = &liveGuardBody{ReadCloser: response.Body, reader: bufio.NewReader(response.Body), ctx: response.Request.Context(), mode: mode, started: time.Now()}
		return nil
	}
	proxy.ErrorHandler = func(writer http.ResponseWriter, request *http.Request, err error) {
		t.Logf("real provider fault relay: %v", err)
		writer.WriteHeader(http.StatusBadGateway)
	}
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	return &liveGuardProxy{url: server.URL, entered: entered}
}

func liveGuardDelay(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type liveGuardBody struct {
	io.ReadCloser
	reader  *bufio.Reader
	ctx     context.Context
	mode    string
	started time.Time
	read    bool
	stalled bool
	pending []byte
}

func (body *liveGuardBody) Read(buffer []byte) (int, error) {
	if body.mode == "heartbeats" && time.Since(body.started) < liveGuardFaultDelay {
		if err := liveGuardDelay(body.ctx, 100*time.Millisecond); err != nil {
			return 0, err
		}
		// Transport fault noise only; no completion or tool payload is synthesized.
		return copy(buffer, ": fault-probe heartbeat\n\n"), nil
	}
	if (body.mode == "body" && !body.read) || (body.mode == "partial" && body.read && !body.stalled) {
		body.stalled = true
		if err := liveGuardDelay(body.ctx, liveGuardFaultDelay); err != nil {
			return 0, err
		}
	}
	if body.mode == "stream_idle" {
		return body.readStream(buffer)
	}
	if body.mode == "partial" && !body.read && len(buffer) > 4 {
		buffer = buffer[:4]
	}
	body.read = true
	return body.reader.Read(buffer)
}

func (body *liveGuardBody) readStream(buffer []byte) (int, error) {
	if len(body.pending) > 0 {
		n := copy(buffer, body.pending)
		body.pending = body.pending[n:]
		return n, nil
	}
	if body.stalled {
		if err := liveGuardDelay(body.ctx, liveGuardFaultDelay); err != nil {
			return 0, err
		}
	}
	line, err := body.reader.ReadBytes('\n')
	body.stalled = liveMeaningfulSSE(line)
	n := copy(buffer, line)
	body.pending = line[n:]
	return n, err
}

func liveMeaningfulSSE(line []byte) bool {
	text := strings.TrimSpace(string(line))
	if !strings.HasPrefix(text, "data:") {
		return false
	}
	var value struct {
		Choices []struct {
			Delta struct {
				Content   string
				ToolCalls []json.RawMessage `json:"tool_calls"`
			}
		}
	}
	if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(text, "data:"))), &value) != nil {
		return false
	}
	return len(value.Choices) > 0 && (value.Choices[0].Delta.Content != "" || len(value.Choices[0].Delta.ToolCalls) > 0)
}
