//go:build phase29preflight

package app

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

const liveLowHitEvery = 21
const liveLoadBlockSize = 100
const liveWarmupIndex = 999
const liveLoadScheduleLimit = 45 * time.Second

type shipSample struct {
	Type       string  `json:"type"`
	RequestID  string  `json:"request_id,omitempty"`
	Model      string  `json:"model"`
	Index      int     `json:"index"`
	StartMS    float64 `json:"start_ms"`
	ElapsedMS  float64 `json:"elapsed_ms"`
	Status     int     `json:"status"`
	OK         bool    `json:"ok"`
	Hit        bool    `json:"hit"`
	Concurrent int64   `json:"concurrent"`
	Error      string  `json:"error,omitempty"`
	AnswerHash string  `json:"answer_hash,omitempty"`
	PlannedMS  float64 `json:"planned_ms"`
}

var liveRequestSequence atomic.Int64

func shipRequest(options liveRequestOptions) shipSample {
	body, err := shipRequestBody(options)
	sample := shipSample{Type: "client", Index: options.index, Concurrent: options.concurrent, Model: options.model}
	if err != nil {
		sample.Error = err.Error()
		return sample
	}
	endpoint := options.url + "/v1/chat/completions"
	if options.direct {
		endpoint = strings.TrimRight(options.url, "/") + "/chat/completions"
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		sample.Error = err.Error()
		return sample
	}
	req.Header.Set("Authorization", "Bearer "+options.token)
	req.Header.Set("Content-Type", "application/json")
	sample.RequestID = fmt.Sprintf("diag-%d", liveRequestSequence.Add(1))
	req.Header.Set("X-Request-ID", sample.RequestID)
	if options.direct {
		req = req.WithContext(timingContext(sample.RequestID))
	}
	started := time.Now()
	sample.StartMS = float64(started.Sub(options.origin).Microseconds()) / 1000
	response, err := options.client.Do(req)
	if err != nil {
		sample.Error = "transport_error: " + err.Error()
	} else {
		sample = shipReadResponse(sample, response)
	}
	sample.ElapsedMS = float64(time.Since(started).Microseconds()) / 1000
	return sample
}

func shipRequestBody(options liveRequestOptions) ([]byte, error) {
	temperature, maxTokens := 0.0, 256
	system, question := options.system, options.question
	if system == "" {
		system = liveFAQSystem
	}
	if question == "" {
		question = fmt.Sprintf("How long is the trial for plan %d?", options.index)
	}
	return json.Marshal(llm.ChatCompletionRequest{Model: options.model, Temperature: &temperature, MaxTokens: &maxTokens,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: system}, {Role: llm.RoleUser, Content: question}}})
}

func shipReadResponse(sample shipSample, response *http.Response) shipSample {
	defer response.Body.Close()
	sample.Status, sample.Hit = response.StatusCode, response.Header.Get("X-Cache-Hit") == "true"
	body, err := io.ReadAll(response.Body)
	if err != nil {
		sample.Error = "body_read_error: " + err.Error()
		return sample
	}
	if response.StatusCode != http.StatusOK {
		var problem gatewayerrors.GatewayError
		if err := json.Unmarshal(body, &problem); err != nil {
			sample.Error = "non_json_error_response"
		} else {
			sample.Error = problem.Code
		}
		return sample
	}
	var result llm.ChatCompletionResponse
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) != 1 || result.Choices[0].Message.Content == "" {
		sample.Error = "invalid_completion"
		return sample
	}
	digest := sha256.Sum256([]byte(result.Choices[0].Message.Content))
	sample.AnswerHash, sample.OK = hex.EncodeToString(digest[:]), true
	return sample
}

func shipLogJSON(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SAMPLE %s", encoded)
}

func shipLoad(t *testing.T, mode string) {
	t.Helper()
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real application load opt-in")
	}
	env := liveEnvironment(t)
	shipLoadEnvironment(t, mode, env)
}

func shipLoadEnvironment(t *testing.T, mode string, env map[string]string) {
	t.Helper()
	t.Setenv("PHASE29_MODE", mode)
	application, token := liveApplication(t, env)
	shipLogJSON(t, map[string]any{"type": "effective_cache_profile", "memo_capacity": os.Getenv("PHASE29_MEMO_CAPACITY"),
		"memo_ttl": os.Getenv("PHASE29_MEMO_TTL"), "input_prefix": os.Getenv("PHASE29_INPUT_PREFIX")})
	recorder := installLiveTiming(t, application, time.Now())
	t.Cleanup(func() { application.Close(); recorder.dump(t) })
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: server.URL, token: token,
		model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: liveWarmupIndex, origin: time.Now()}
	warm := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "warmup", "sample": warm})
	if !warm.OK {
		t.Fatal("real primary warmup failed; no valid latency gate")
	}
	options.origin = time.Now()
	failures := shipScheduledLoad(t, application, options)
	if failures != 0 {
		t.Fatalf("real load failed requests=%d", failures)
	}
}

func shipScheduledLoad(t *testing.T, application *App, options liveRequestOptions) int {
	t.Helper()
	count, concurrency := liveInteger(t, "PHASE29_COUNT"), liveInteger(t, "PHASE29_CLIENT_CONCURRENCY")
	interval := liveLoadInterval(t, count, concurrency)
	options.hitEvery = liveLowHitEvery
	if os.Getenv("PHASE29_PURE_MISS") == "true" {
		options.hitEvery = 0
	}
	sem, samples := make(chan struct{}, concurrency), make([]shipSample, count)
	modelLimits := liveModelLimits(options.models)
	var group sync.WaitGroup
	var active atomic.Int64
	for index := 1; index <= count; index++ {
		if delay := time.Until(options.origin.Add(time.Duration(index-1) * interval)); delay > 0 {
			time.Sleep(delay)
		}
		sem <- struct{}{}
		group.Add(1)
		go liveScheduledRequest{options: options, index: index, interval: interval, sem: sem, modelLimits: modelLimits, samples: samples, group: &group, active: &active}.execute()
	}
	group.Wait()
	foreground := time.Since(options.origin)
	application.Close()
	var failures int
	for _, sample := range samples {
		shipLogJSON(t, sample)
		if !sample.OK {
			failures++
		}
	}
	shipLogJSON(t, map[string]any{"type": "metadata", "model": os.Getenv("PHASE29_MODEL"), "mode": os.Getenv("PHASE29_MODE"), "count": count,
		"interval_ms": interval.Milliseconds(), "elapsed_ms": float64(foreground.Microseconds()) / 1000, "max_concurrency": concurrency, "failed": failures})
	if options.onSamples != nil {
		options.onSamples(samples)
	}
	return failures
}

type liveScheduledRequest struct {
	options     liveRequestOptions
	index       int
	interval    time.Duration
	sem         chan struct{}
	modelLimits map[string]chan struct{}
	samples     []shipSample
	group       *sync.WaitGroup
	active      *atomic.Int64
}

func liveModelLimits(models []string) map[string]chan struct{} {
	limits := make(map[string]chan struct{}, len(models))
	for _, model := range models {
		limits[model] = make(chan struct{}, 1)
	}
	return limits
}

func (task liveScheduledRequest) execute() {
	defer task.group.Done()
	defer func() { <-task.sem }()
	request := task.options
	request.index = task.index
	if len(request.models) > 0 {
		request.model = request.models[(task.index-1)%len(request.models)]
		limit := task.modelLimits[request.model]
		limit <- struct{}{}
		defer func() { <-limit }()
	}
	request.concurrent = task.active.Add(1)
	defer task.active.Add(-1)
	slot := (task.index-1)%liveLoadBlockSize + 1
	if request.hitEvery > 0 && slot%request.hitEvery == 0 {
		request.index = task.index - request.hitEvery + 1
	}
	task.samples[task.index-1] = shipRequest(request)
	task.samples[task.index-1].PlannedMS = float64(time.Duration(task.index-1)*task.interval) / float64(time.Millisecond)
}

func liveLoadInterval(t *testing.T, count, concurrency int) time.Duration {
	t.Helper()
	interval := time.Duration(liveInteger(t, "PHASE29_INTERVAL_MS")) * time.Millisecond
	if count < 1 || concurrency < 1 || interval < time.Millisecond || time.Duration(count)*interval > liveLoadScheduleLimit {
		t.Fatal("invalid bounded load inputs")
	}
	return interval
}

func TestLiveCacheLoadOff(t *testing.T)       { shipLoad(t, "off") }
func TestLiveCacheLoadOn(t *testing.T)        { shipLoad(t, "on") }
func TestLiveCacheLoadOnRepeat(t *testing.T)  { shipLoad(t, "on") }
func TestLiveCacheLoadOffRepeat(t *testing.T) { shipLoad(t, "off") }

func TestLiveUpstreamStreamDiagnostic(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real provider diagnostic opt-in")
	}
	env := liveEnvironment(t)
	payload := liveToolRequest(liveChain{model: env["SANS_PRIMARY_DEFAULT_MODEL"]}, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	payload.Stream = true
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(env["SANS_BASE_URL"], "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+env["SANS_PRIMARY_API_KEY"])
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatal("real upstream transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("direct real upstream HTTP=%d", response.StatusCode)
	}
	liveDiagnoseFrames(t, response.Body)
}

func liveDiagnoseFrames(t *testing.T, body io.Reader) {
	t.Helper()
	scanner := bufio.NewScanner(body)
	var done, frames, tools, usage int
	finishes := map[string]int{}
	positions := map[string]int{}
	rootKeys := map[string]int{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "data: [DONE]" {
			done++
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := []byte(strings.TrimPrefix(line, "data: "))
		chunk, root := liveReadUpstreamChunk(t, data)
		for key := range root {
			rootKeys[key]++
		}
		frames++
		if chunk.Usage != nil {
			usage++
			positions["usage"] = frames
		}
		for _, choice := range chunk.Choices {
			tools += len(choice.Delta.ToolCalls)
			if choice.FinishReason != nil && !(chunk.Usage != nil && choice.Delta.Content == "" && len(choice.Delta.ToolCalls) == 0) {
				finishes[*choice.FinishReason]++
				positions["finish"] = frames
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("direct_upstream frames=%d done=%d tools=%d usage=%d finishes=%v positions=%v root_fields=%v", frames, done, tools, usage, finishes, positions, rootKeys)
	liveAssertToolTerminal(t, map[string]int{"done": done, "tools": tools, "tool_calls": finishes["tool_calls"]})
}

func liveAssertToolTerminal(t *testing.T, counts map[string]int) {
	t.Helper()
	if counts["done"] != 1 || counts["tools"] == 0 || counts["tool_calls"] != 1 {
		t.Fatal("direct real upstream tool stream did not meet OpenAI protocol")
	}
}

func liveReadUpstreamChunk(t *testing.T, data []byte) (llm.ChatCompletionChunkResponse, map[string]json.RawMessage) {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal("non-JSON upstream SSE")
	}
	var chunk llm.ChatCompletionChunkResponse
	if err := json.Unmarshal(data, &chunk); err != nil {
		t.Fatal("invalid upstream chunk schema")
	}
	return chunk, root
}
