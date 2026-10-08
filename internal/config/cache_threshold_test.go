package config

import "testing"

func TestCacheThresholdEnvironmentValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{
		{"0.01", true}, {"0.92", true}, {"1", true},
		{"0", false}, {"-1", false}, {"1.01", false},
		{"NaN", false}, {"+Inf", false}, {"-Inf", false},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("SEMANTIC_CACHE_THRESHOLD", test.value)
			cache := CacheConfig{
				Enabled: true, Provider: "embedding", EmbeddingModel: "fixture", VectorDimension: 2,
				TTL: "1h", Threshold: cacheConfigFromEnv().Threshold, MaxCandidates: 10,
				PGVector: PGVectorConfig{IndexType: "hnsw", HNSWM: 1, HNSWEFConstruct: 1, SearchEF: 1},
				UseCases: []CacheUseCaseConfig{{ReuseMode: CacheReuseSemantic, UseCaseID: "FAQ",
					APIKeyIDs: []string{"fixture"}, KnowledgeVersion: "v1", TargetModel: "model", SystemPrompt: "FAQ"}},
				ReadTimeout: "100ms", ReadConcurrency: 4, WriteTimeout: "2s", WriteWorkers: 2,
				QueueCapacity: 32, ShutdownGrace: "1s",
			}
			if err := validateSemanticCacheConfig(&Config{Cache: cache}); (err == nil) != test.valid {
				t.Fatalf("threshold %q: valid=%t, error=%v", test.value, test.valid, err)
			}
		})
	}
}
