//go:build phase29preflight

package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/openai"
)

type liveEmbeddingSample struct {
	Type      string  `json:"type"`
	ID        string  `json:"request_id"`
	ElapsedMS float64 `json:"elapsed_ms"`
	OK        bool    `json:"ok"`
	Dimension int     `json:"dimension"`
	Error     string  `json:"error,omitempty"`
}

func TestLiveEmbeddingStress(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real embedding opt-in")
	}
	liveEnvironment(t)
	recorder := installLiveTiming(t, nil, time.Now())
	adapter := openai.NewAdapter("diagnostic-embedding", os.Getenv("PHASE29_EMBEDDING_BASE_URL"), os.Getenv("PHASE29_EMBEDDING_API_KEY"), os.Getenv("PHASE29_MODEL"))
	count, concurrency := liveInteger(t, "PHASE29_COUNT"), liveInteger(t, "PHASE29_CLIENT_CONCURRENCY")
	const maxEmbeddingSamples = 1000
	if count < 1 || count > maxEmbeddingSamples || concurrency < 1 {
		t.Fatal("invalid bounded embedding stress inputs")
	}
	sem, samples := make(chan struct{}, concurrency), make([]liveEmbeddingSample, count)
	var group sync.WaitGroup
	for index := 0; index < count; index++ {
		sem <- struct{}{}
		group.Add(1)
		go func(index int) {
			defer group.Done()
			defer func() { <-sem }()
			samples[index] = liveEmbeddingRequest(adapter, index)
		}(index)
	}
	group.Wait()
	failures := 0
	for _, sample := range samples {
		shipLogJSON(t, sample)
		if !sample.OK {
			failures++
		}
	}
	recorder.dump(t)
	shipLogJSON(t, map[string]any{"type": "embedding_metadata", "count": count, "concurrency": concurrency, "failed": failures})
	if failures > 0 {
		t.Fatalf("real embedding stress failures=%d", failures)
	}
}

func liveEmbeddingRequest(adapter *openai.Adapter, index int) liveEmbeddingSample {
	id := fmt.Sprintf("embed-%d", index)
	const embeddingStressDeadline = 2 * time.Second
	ctx, cancel := context.WithTimeout(timingContext(id), embeddingStressDeadline)
	defer cancel()
	started := time.Now()
	response, err := adapter.Embed(ctx, &llm.EmbeddingRequest{Model: os.Getenv("PHASE29_MODEL"), Input: []string{fmt.Sprintf("How long is the trial for plan %d?", index+1)}})
	sample := liveEmbeddingSample{Type: "embedding_client", ID: id, ElapsedMS: float64(time.Since(started).Microseconds()) / 1000}
	if err != nil {
		sample.Error = err.Error()
		return sample
	}
	if response == nil || len(response.Data) != 1 {
		sample.Error = "invalid_embedding_count"
		return sample
	}
	vector := response.Data[0].Embedding
	sample.Dimension = len(vector)
	var norm float64
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			sample.Error = "non_finite_embedding"
			return sample
		}
		norm += float64(value) * float64(value)
	}
	sample.OK = len(vector) > 0 && norm > 0
	return sample
}
