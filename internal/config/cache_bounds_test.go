package config

import "testing"

func TestCacheConfigRejectsUnboundedWork(t *testing.T) {
	cache := CacheConfig{Enabled: true, Provider: "embedding", EmbeddingModel: "fixture", VectorDimension: 2, TTL: "1h", Threshold: .99, MaxCandidates: 10,
		PGVector:    PGVectorConfig{IndexType: "hnsw", HNSWM: 1, HNSWEFConstruct: 1, SearchEF: 1},
		UseCases:    []CacheUseCaseConfig{{ReuseMode: "semantic", UseCaseID: "FAQ", APIKeyIDs: []string{"fixture"}, KnowledgeVersion: "v1", TargetModel: "model", SystemPrompt: "FAQ"}},
		ReadTimeout: "100ms", ReadConcurrency: 4, WriteTimeout: "2s", WriteWorkers: 2, QueueCapacity: 32, ShutdownGrace: "1s"}
	if err := validateSemanticCacheConfig(&Config{Cache: cache}); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*CacheConfig){
		"read timeout":     func(c *CacheConfig) { c.ReadTimeout = "0s" },
		"read concurrency": func(c *CacheConfig) { c.ReadConcurrency = 0 },
		"write timeout":    func(c *CacheConfig) { c.WriteTimeout = "bad" },
		"workers":          func(c *CacheConfig) { c.WriteWorkers = -1 },
		"queue":            func(c *CacheConfig) { c.QueueCapacity = 0 },
		"shutdown":         func(c *CacheConfig) { c.ShutdownGrace = "0s" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := cache
			change(&candidate)
			if err := validateSemanticCacheConfig(&Config{Cache: candidate}); err == nil {
				t.Fatal("unbounded cache accepted")
			}
		})
	}
}
