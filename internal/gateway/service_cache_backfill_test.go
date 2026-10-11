package gateway_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/cache"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/storage"
)

const (
	backfillHealthy int32 = iota
	backfillTimeout
	backfillUnavailable
	backfillInvalidVector
	backfillReadTimeout  = 100 * time.Millisecond
	backfillSeedQuestion = "seed question"
	backfillMissQuestion = "different question"
)

// A ready-scope lookup without a completed embedding must not immediately retry
// that dependency through asynchronous storage. Empty scopes and completed
// embeddings remain writable, including when only vector search has failed.
func TestServiceCacheBackfillSkipsFailedEmbedding(t *testing.T) {
	for _, test := range []struct {
		name  string
		fault int32
	}{{"timeout", backfillTimeout}, {"http503", backfillUnavailable}, {"invalid_vector", backfillInvalidVector}} {
		t.Run(test.name, func(t *testing.T) {
			endpoint, adapter := newBackfillEmbeddingHTTP(t)
			f := newIdentityFixture(t, identityFixtureOptions{cache: true, embedding: adapter, readTimeout: backfillReadTimeout})
			seedBackfillScope(t, f)
			before := len(f.stages.snapshot())
			endpoint.fault.Store(test.fault)

			f.call(t, identityUserA, backfillMissQuestion)
			f.cache.Close() // Drain accepted work before asserting no hidden retry.
			if test.fault == backfillTimeout {
				endpoint.waitForCancellation(t)
			}
			assertBackfillAccounting(t, f, 1)
			assertBackfillCounts(t, f, backfillCounts{rows: 1, httpCalls: 1, endpoint: endpoint})
			assertBackfillStages(t, f.stages.snapshot()[before:], map[string]int{"write_enqueue": 0, "store_embedding": 0, "store_total": 0})
		})
	}
}

func TestServiceCacheBackfillRecoversAfterEmbeddingTimeout(t *testing.T) {
	endpoint, adapter := newBackfillEmbeddingHTTP(t)
	f := newIdentityFixture(t, identityFixtureOptions{cache: true, embedding: adapter, readTimeout: backfillReadTimeout})
	seedBackfillScope(t, f)
	before := len(f.stages.snapshot())
	endpoint.fault.Store(backfillTimeout)
	f.call(t, identityUserA, backfillMissQuestion)
	endpoint.waitForCancellation(t)
	assertBackfillStages(t, f.stages.snapshot()[before:], map[string]int{"write_enqueue": 0})

	f.call(t, identityUserA, backfillMissQuestion)
	f.stages.waitForWrite(t)
	f.call(t, identityUserA, backfillMissQuestion)
	f.cache.Close()
	assertBackfillAccounting(t, f, 2)
	assertBackfillCounts(t, f, backfillCounts{rows: 2, httpCalls: 3, endpoint: endpoint})
	assertBackfillStages(t, f.stages.snapshot()[before:], map[string]int{"write_enqueue": 1, "store_embedding": 0, "store_total": 1})
}

func TestServiceCacheBackfillSeedsEmptyScopeAndOrdinaryMiss(t *testing.T) {
	endpoint, adapter := newBackfillEmbeddingHTTP(t)
	f := newIdentityFixture(t, identityFixtureOptions{cache: true, embedding: adapter, readTimeout: backfillReadTimeout})
	f.call(t, identityUserA, backfillSeedQuestion)
	f.stages.waitForWrite(t)
	f.call(t, identityUserA, backfillMissQuestion)
	f.stages.waitForWrite(t)
	f.call(t, identityUserA, backfillMissQuestion)
	f.cache.Close()
	assertBackfillAccounting(t, f, 2)
	assertBackfillCounts(t, f, backfillCounts{rows: 2, httpCalls: 3, endpoint: endpoint})
	assertBackfillStages(t, f.stages.snapshot(), map[string]int{"write_enqueue": 2, "store_embedding": 1, "store_total": 2})
}

func TestServiceCacheBackfillReusesVectorAfterSearchFailure(t *testing.T) {
	endpoint, adapter := newBackfillEmbeddingHTTP(t)
	vector := &backfillSearchFailure{NoopVectorAdapter: storage.NewNoopVectorAdapter()}
	f := newIdentityFixture(t, identityFixtureOptions{cache: true, embedding: adapter, vector: vector, readTimeout: backfillReadTimeout})
	seedBackfillScope(t, f)
	before := len(f.stages.snapshot())
	f.call(t, identityUserA, backfillMissQuestion)
	f.cache.Close()
	assertBackfillAccounting(t, f, 1)
	assertBackfillCounts(t, f, backfillCounts{rows: 2, httpCalls: 1, endpoint: endpoint})
	assertBackfillStages(t, f.stages.snapshot()[before:], map[string]int{"write_enqueue": 1, "store_embedding": 0, "store_total": 1, "vector_search": 1})
	if vector.searches.Load() != 1 || vector.inserts.Load() != 2 {
		t.Errorf("vector calls: searches=%d inserts=%d; want one search and seed plus backfill insert", vector.searches.Load(), vector.inserts.Load())
	}
}

type backfillEmbeddingHTTP struct {
	fault     atomic.Int32
	calls     atomic.Int32
	cancelled chan struct{}
}

func newBackfillEmbeddingHTTP(t *testing.T) (*backfillEmbeddingHTTP, *openai.Adapter) {
	t.Helper()
	endpoint := &backfillEmbeddingHTTP{cancelled: make(chan struct{}, 1)}
	server := httptest.NewServer(http.HandlerFunc(endpoint.serveHTTP))
	t.Cleanup(server.Close)
	return endpoint, openai.NewAdapter("backfill-embedding", server.URL, "", "text-embedding-3-small")
}

func (endpoint *backfillEmbeddingHTTP) serveHTTP(w http.ResponseWriter, r *http.Request) {
	var request llm.EmbeddingRequest
	if r.Method != http.MethodPost || r.URL.Path != "/embeddings" || json.NewDecoder(r.Body).Decode(&request) != nil || len(request.Input) != 1 {
		http.Error(w, "invalid embedding request", http.StatusBadRequest)
		return
	}
	endpoint.calls.Add(1)
	fault := endpoint.fault.Swap(backfillHealthy)
	if fault == backfillTimeout {
		<-r.Context().Done()
		endpoint.cancelled <- struct{}{}
		return
	}
	if fault == backfillUnavailable {
		http.Error(w, "embedding service unavailable", http.StatusServiceUnavailable)
		return
	}
	vector := []float32{-1}
	if request.Input[0] == backfillSeedQuestion {
		vector = []float32{1}
	}
	if fault == backfillInvalidVector {
		vector = []float32{0}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(llm.EmbeddingResponse{Model: request.Model, Data: []llm.Embedding{{Index: 0, Embedding: vector}}})
}

func (endpoint *backfillEmbeddingHTTP) waitForCancellation(t *testing.T) {
	t.Helper()
	timer := time.NewTimer(identityWriteWait)
	defer timer.Stop()
	select {
	case <-endpoint.cancelled:
	case <-timer.C:
		t.Fatal("embedding HTTP request did not observe deadline cancellation")
	}
}

func seedBackfillScope(t *testing.T, f *identityFixture) {
	t.Helper()
	scope, eligible := f.cache.Eligible(identityUserA, "user", identityRequest(backfillSeedQuestion))
	if !eligible {
		t.Fatal("seed must be eligible")
	}
	response, err := json.Marshal(textResponse("answer for " + identityUserA).Choices)
	if err != nil {
		t.Fatal(err)
	}
	write := cache.CacheWrite{ID: "backfill-seed", Scope: scope, Model: identityModel, Text: backfillSeedQuestion, Response: string(response), Vector: []float32{1}}
	if err := f.cache.StoreWrite(context.Background(), write); err != nil {
		t.Fatal(err)
	}
	f.stages.waitForWrite(t)
}

func assertBackfillAccounting(t *testing.T, f *identityFixture, calls int) {
	t.Helper()
	records := f.usages(t)
	if len(records) != calls || len(f.adapter.traceIDs) != calls {
		t.Errorf("usage records=%d primary calls=%d, want %d each", len(records), len(f.adapter.traceIDs), calls)
	}
	for _, record := range records {
		assertIdentityUUID(t, record.ID)
		if record.Key != identityUserA || record.Status != string(controlstate.SettlementStatusSettled) || !record.Credits.Valid || record.Credits.Int64 != identityRequestCost {
			t.Errorf("incorrect fallback settlement: %+v", record)
		}
	}
	f.assertBalance(t, identityUserA, identityInitialBalance-int64(calls)*identityRequestCost)
}

type backfillCounts struct {
	rows      int
	httpCalls int32
	endpoint  *backfillEmbeddingHTTP
}

func assertBackfillCounts(t *testing.T, f *identityFixture, expected backfillCounts) {
	t.Helper()
	if entries := f.candidates(t, identityUserA); len(entries) != expected.rows {
		t.Errorf("cache rows=%d, want %d", len(entries), expected.rows)
	}
	if calls := expected.endpoint.calls.Load(); calls != expected.httpCalls {
		t.Errorf("embedding HTTP calls=%d, want %d", calls, expected.httpCalls)
	}
}

func assertBackfillStages(t *testing.T, stages []observability.StageMeasurement, expected map[string]int) {
	t.Helper()
	counts := make(map[string]int)
	for _, stage := range stages {
		counts[stage.Name]++
	}
	for stage, want := range expected {
		if counts[stage] != want {
			t.Errorf("%s stages=%d, want %d", stage, counts[stage], want)
		}
	}
}

type backfillSearchFailure struct {
	*storage.NoopVectorAdapter
	searches atomic.Int32
	inserts  atomic.Int32
}

func (vector *backfillSearchFailure) Search(context.Context, string, []float32, int) ([]map[string]interface{}, error) {
	vector.searches.Add(1)
	return nil, errors.New("vector search unavailable")
}

func (vector *backfillSearchFailure) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	vector.inserts.Add(1)
	return vector.NoopVectorAdapter.Insert(ctx, collection, vectors, metadata)
}
