package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/llm"
)

// Static answers must not replay calls, call results, or tool terminal markers,
// including a tool choice mixed with an otherwise ordinary answer.
func TestCacheRejectsToolProtocolResponses(t *testing.T) {
	plain := `{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}`
	tool := `{"message":{"role":"assistant","content":"Checking","tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},"finish_reason":"tool_calls"}`
	cases := map[string]string{
		"plain":    "[" + plain + "]",
		"calls":    "[" + tool + "]",
		"mixed":    "[" + plain + "," + tool + "]",
		"terminal": `[{"message":{"role":"assistant","content":"Checking"},"finish_reason":"tool_calls"}]`,
		"result":   `[{"message":{"role":"assistant","content":"answer","tool_call_id":"call-1"}}]`,
		"empty":    `[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]`,
	}
	for _, scope := range []string{"semantic-scope", exactScopePrefix + "scope"} {
		for name, response := range cases {
			t.Run(scope+"/"+name, func(t *testing.T) {
				service := responseProtocolService(t)
				query := CacheLookup{Scope: scope, Model: "model", Text: "question"}
				write := CacheWrite{ID: "entry", Scope: scope, Model: query.Model, Text: query.Text, Response: response, Vector: []float32{1, 0}}
				err := service.StoreWrite(context.Background(), write)
				if name == "plain" && err != nil {
					t.Fatal(err)
				}
				if name != "plain" && (err == nil || err.Error() != "invalid_entry") {
					t.Errorf("new %s response must be rejected, got %v", name, err)
				}
				seedProtocolResponse(t, service, write)
				result, err := service.LookupWithVector(context.Background(), query)
				if err != nil {
					t.Fatal(err)
				}
				if (result.Entry != nil) != (name == "plain") {
					t.Fatalf("static answer cache hit=%v for %s response", result.Entry != nil, name)
				}
			})
		}
	}
}

func seedProtocolResponse(t *testing.T, service *SemanticCacheService, write CacheWrite) {
	t.Helper()
	id := write.ID
	if isExactScope(write.Scope) {
		id = exactAnswerID(CacheLookup{Scope: write.Scope, Model: write.Model, Text: write.Text})
	}
	entry := &controlstate.SemanticCacheEntry{ID: id, Scope: write.Scope, Model: write.Model,
		Response: write.Response, Vector: floatsToBytes(write.Vector), Enabled: true, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err := service.repo.Store(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
}

func responseProtocolService(t *testing.T) *SemanticCacheService {
	t.Helper()
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "responses.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg := boundsConfig()
	cfg.WriteWorkers, cfg.ReadConcurrency = 0, 0
	adapter := &boundedEmbed{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}}
	service := NewSemanticCacheService(cfg, repo.SemanticCache(), nil, adapter)
	t.Cleanup(service.Close)
	return service
}
