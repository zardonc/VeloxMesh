package cache

import (
	"strings"
	"testing"
)

func TestSemanticCacheCollectionNames(t *testing.T) {
	first := vectorCollection("tenant-version-one", "model-one")
	if strings.ContainsAny(first, ":/\\\n\r") {
		t.Fatalf("invalid Qdrant collection name: %q", first)
	}
	if first == vectorCollection("tenant-version-two", "model-one") || first == vectorCollection("tenant-version-one", "model-two") {
		t.Fatal("collection names must preserve scope and model isolation")
	}
}
