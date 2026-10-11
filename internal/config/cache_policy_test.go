package config

// Failure cases: implicit semantic reuse, unknown policy, duplicate trusted
// profiles, exact-only requiring a live embedding/vector dependency.
import "testing"

func TestCacheReusePolicyValidation(t *testing.T) {
	for _, mode := range []string{"", "disabled", "exact", "semantic"} {
		profile := CacheUseCaseConfig{UseCaseID: "FAQ", KnowledgeVersion: "v1", TargetModel: "model", SystemPrompt: "FAQ", APIKeyIDs: []string{"fixture"}, ReuseMode: mode}
		if err := validateCacheUseCases([]CacheUseCaseConfig{profile}); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	profile := CacheUseCaseConfig{UseCaseID: "FAQ", KnowledgeVersion: "v1", TargetModel: "model", SystemPrompt: "FAQ", APIKeyIDs: []string{"fixture"}, ReuseMode: "unknown"}
	if err := validateCacheUseCases([]CacheUseCaseConfig{profile}); err == nil {
		t.Fatal("unknown cache policy accepted")
	}
	profile.ReuseMode = "exact"
	if err := validateCacheUseCases([]CacheUseCaseConfig{profile, profile}); err == nil {
		t.Fatal("ambiguous overlapping trusted cache profiles accepted")
	}
}

func TestExactOnlyCacheNeedsNoEmbeddingDependency(t *testing.T) {
	cache := CacheConfig{Enabled: true, TTL: "1h", ReadTimeout: "100ms", ReadConcurrency: 1,
		WriteTimeout: "1s", WriteWorkers: 1, QueueCapacity: 1, ShutdownGrace: "1s",
		UseCases: []CacheUseCaseConfig{{ReuseMode: "exact", UseCaseID: "FAQ", KnowledgeVersion: "v1",
			TargetModel: "model", SystemPrompt: "FAQ", APIKeyIDs: []string{"fixture"}}}}
	if err := validateSemanticCacheConfig(&Config{Cache: cache}); err != nil {
		t.Fatalf("exact-only cache required unused vector/embedding configuration: %v", err)
	}
}
