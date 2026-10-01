package cache

// Failure matrix: embedding delay/error/nil/nonfinite/dimension; vector failure;
// repository failure; admission saturation; full/closed queue; write cancellation;
// deadline-limited shutdown; immutable version/model/usage snapshots; disabled I/O.
import (
	"context"
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/storage"
)

type boundedEmbed struct {
	mockEmbedAdapter
	started  chan struct{}
	block    bool
	release  chan struct{}
	response *llm.EmbeddingResponse
	err      error
	calls    atomic.Int64
}

func (a *boundedEmbed) Embed(ctx context.Context, _ *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	a.calls.Add(1)
	if a.started != nil {
		a.started <- struct{}{}
	}
	if a.block {
		if a.release != nil {
			<-a.release
			return nil, ctx.Err()
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return a.response, a.err
}

func boundsConfig() SemanticCacheConfig {
	return SemanticCacheConfig{Enabled: true, Threshold: .99, MaxCandidates: 10, TTL: time.Hour, EmbeddingModel: "test", VectorDimension: 2,
		ReadTimeout: 20 * time.Millisecond, ReadConcurrency: 1, WriteTimeout: 30 * time.Millisecond, WriteWorkers: 1, QueueCapacity: 1, ShutdownGrace: 20 * time.Millisecond}
}

func TestSemanticCacheReadDeadlineAndSaturation(t *testing.T) {
	adapter := &boundedEmbed{block: true, started: make(chan struct{}, 2), release: make(chan struct{})}
	t.Cleanup(func() { close(adapter.release) })
	svc := NewSemanticCacheService(boundsConfig(), &mockRepo{}, nil, adapter)
	t.Cleanup(svc.Close)
	done := make(chan error, 1)
	go func() { _, err := svc.Lookup(context.Background(), "scope", "model", "text"); done <- err }()
	<-adapter.started
	started := time.Now()
	if _, err := svc.Lookup(context.Background(), "scope", "model", "text"); err == nil {
		t.Fatal("saturated lookup must bypass")
	}
	if time.Since(started) > time.Second {
		t.Fatal("admission must not wait")
	}
	if err := <-done; err == nil {
		t.Fatal("deadline must be reported")
	}
	if time.Since(started) > time.Second || adapter.calls.Load() != 1 {
		t.Fatal("lookup not bounded")
	}
}

func TestSemanticCacheInvalidEmbeddings(t *testing.T) {
	for name, values := range map[string][]float32{"empty": {}, "dimension": {1}, "nan": {float32(math.NaN()), 0}, "infinity": {1, float32(math.Inf(1))}, "zero": {0, 0}} {
		t.Run(name, func(t *testing.T) {
			adapter := &boundedEmbed{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: values}}}}
			repo := &mockRepo{}
			svc := NewSemanticCacheService(boundsConfig(), repo, nil, adapter)
			t.Cleanup(svc.Close)
			if hit, err := svc.Lookup(context.Background(), "scope", "model", "text"); hit != nil || err == nil {
				t.Fatal("invalid embedding must report bypass")
			}
			if err := svc.Store(context.Background(), "id", "scope", "model", "text", "[]", nil); err == nil {
				t.Fatal("invalid store vector must be reported")
			}
			if repo.listCalls != 0 || len(repo.entries) != 0 {
				t.Fatal("invalid vector reached repository")
			}
		})
	}
}

type failingVector struct{ storage.VectorAdapter }

func (v failingVector) Search(context.Context, string, []float32, int) ([]map[string]interface{}, error) {
	return nil, errors.New("vector unavailable")
}
func (v failingVector) Insert(context.Context, string, [][]float32, []map[string]interface{}) error {
	return errors.New("vector unavailable")
}

func TestSemanticCacheVectorFaultReportedWithoutScan(t *testing.T) {
	adapter := &boundedEmbed{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}}
	repo := &mockRepo{}
	svc := NewSemanticCacheService(boundsConfig(), repo, failingVector{}, adapter)
	t.Cleanup(svc.Close)
	if _, err := svc.Lookup(context.Background(), "scope", "model", "text"); err == nil {
		t.Fatal("vector fault swallowed")
	}
	if repo.listCalls != 0 {
		t.Fatal("vector fault triggered repository scan")
	}
	if err := svc.Store(context.Background(), "id", "scope", "model", "text", "[]", nil); err == nil {
		t.Fatal("insert failure swallowed")
	}
}

func TestSemanticCacheQueueAndShutdown(t *testing.T) {
	adapter := &boundedEmbed{block: true, started: make(chan struct{}, 2)}
	svc := NewSemanticCacheService(boundsConfig(), &mockRepo{}, nil, adapter)
	write := CacheWrite{ID: "id", Scope: "faq-v1", Model: "model", Text: "text", Response: `[{"message":{"role":"assistant","content":"answer"}}]`}
	if !svc.Enqueue(write) {
		t.Fatal("first write not accepted")
	}
	<-adapter.started
	if !svc.Enqueue(write) {
		t.Fatal("queued write not accepted")
	}
	started := time.Now()
	if svc.Enqueue(write) {
		t.Fatal("full queue accepted write")
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("full queue blocked caller")
	}
	svc.Close()
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("shutdown not bounded")
	}
	if svc.Enqueue(write) {
		t.Fatal("closed queue accepted write")
	}
	svc.Close()
}

type capturedRepo struct {
	mockRepo
	stored chan *controlstate.SemanticCacheEntry
}

func (r *capturedRepo) Store(_ context.Context, entry *controlstate.SemanticCacheEntry) error {
	r.stored <- entry
	return nil
}

func TestSemanticCacheWriteSnapshotAndDisabled(t *testing.T) {
	adapter := &boundedEmbed{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}}
	repo := &capturedRepo{stored: make(chan *controlstate.SemanticCacheEntry, 1)}
	svc := NewSemanticCacheService(boundsConfig(), repo, nil, adapter)
	t.Cleanup(svc.Close)
	usage := "usage-v1"
	write := CacheWrite{ID: "id", Scope: "faq-v1", Model: "model-v1", Text: "text", Response: "[]", UsageID: &usage}
	if !svc.Enqueue(write) {
		t.Fatal("enqueue failed")
	}
	usage = "usage-v2"
	write.Scope = "faq-v2"
	entry := <-repo.stored
	if entry.Scope != "faq-v1" || *entry.UsageID != "usage-v1" || entry.Model != "model-v1" {
		t.Fatal("queued identity was mutable")
	}
	cfg := boundsConfig()
	cfg.Enabled = false
	disabled := NewSemanticCacheService(cfg, repo, nil, adapter)
	before := adapter.calls.Load()
	disabled.Lookup(context.Background(), "scope", "model", "text")
	disabled.Enqueue(write)
	disabled.Close()
	if adapter.calls.Load() != before {
		t.Fatal("disabled cache performed I/O")
	}
}

func TestSemanticCacheCombinedReadWriteCapacity(t *testing.T) {
	adapter := &boundedEmbed{block: true, started: make(chan struct{}, 6), release: make(chan struct{})}
	t.Cleanup(func() { close(adapter.release) })
	cfg := boundsConfig()
	cfg.ReadConcurrency, cfg.WriteWorkers, cfg.QueueCapacity = 4, 2, 32
	svc := NewSemanticCacheService(cfg, &mockRepo{}, nil, adapter)
	t.Cleanup(svc.Close)
	write := CacheWrite{ID: "id", Scope: "v1", Model: "model", Text: "text", Response: "[]"}
	svc.Enqueue(write)
	svc.Enqueue(write)
	<-adapter.started
	<-adapter.started
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		go func() { svc.Lookup(context.Background(), "scope", "model", "text"); done <- struct{}{} }()
	}
	for i := 0; i < 4; i++ {
		<-adapter.started
	}
	for i := 0; i < 32; i++ {
		if !svc.Enqueue(write) {
			t.Fatal("queue filled before configured capacity")
		}
	}
	if svc.Enqueue(write) {
		t.Fatal("burst exceeded queue capacity")
	}
	if _, err := svc.Lookup(context.Background(), "scope", "model", "text"); err == nil {
		t.Fatal("fifth read accepted")
	}
	if adapter.calls.Load() != 6 {
		t.Fatal("combined embedding concurrency exceeded four reads plus two writes")
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	svc.Close()
}
