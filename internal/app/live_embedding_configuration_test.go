//go:build phase29preflight

package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const liveInvalidPrefixBytes = 65
const liveInvalidMemoCapacity = 4097

// Exercise the actual application configuration boundary after real startup.
func TestLiveEmbeddingConfigurationBoundaries(t *testing.T) {
	chain := newLiveChain(t)
	chain.app.Close()
	data, err := os.ReadFile(os.Getenv("CONFIG_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name   string
		fields map[string]any
	}{
		{"long_prefix", map[string]any{"embedding_input_prefix": strings.Repeat("x", liveInvalidPrefixBytes)}},
		{"nul_prefix", map[string]any{"embedding_input_prefix": "query\x00"}},
		{"negative_capacity", map[string]any{"embedding_memo_capacity": -1}},
		{"oversize_capacity", map[string]any{"embedding_memo_capacity": liveInvalidMemoCapacity}},
		{"missing_ttl", map[string]any{"embedding_memo_capacity": 1, "embedding_memo_ttl": ""}},
		{"zero_ttl", map[string]any{"embedding_memo_capacity": 1, "embedding_memo_ttl": "0s"}},
		{"oversize_ttl", map[string]any{"embedding_memo_capacity": 1, "embedding_memo_ttl": "2h"}},
		{"ttl_without_capacity", map[string]any{"embedding_memo_capacity": 0, "embedding_memo_ttl": "1m"}},
	} {
		t.Run(entry.name, func(t *testing.T) {
			var original map[string]any
			if err := json.Unmarshal(data, &original); err != nil {
				t.Fatal(err)
			}
			cacheConfig := original["cache"].(map[string]any)
			candidate := make(map[string]any, len(cacheConfig)+len(entry.fields))
			for key, value := range cacheConfig {
				candidate[key] = value
			}
			for key, value := range entry.fields {
				candidate[key] = value
			}
			t.Setenv("CONFIG_FILE", liveJSON(t, map[string]any{"providers": original["providers"],
				"default_provider": original["default_provider"], "cache": candidate}))
			application, err := New()
			if err == nil {
				application.Close()
				t.Fatal("invalid embedding configuration accepted")
			}
			shipLogJSON(t, map[string]any{"type": "embedding_configuration_rejected", "case": entry.name, "error": err.Error()})
		})
	}
}
