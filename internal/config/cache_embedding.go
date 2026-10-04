package config

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxEmbeddingPrefixBytes = 64
const maxEmbeddingMemoEntries = 4096
const maxEmbeddingMemoTTL = time.Hour

func mergeCacheEmbedding(dst *CacheConfig, src *cacheFileConfig) {
	if src.EmbeddingInputPrefix != nil {
		dst.EmbeddingInputPrefix = *src.EmbeddingInputPrefix
	}
	if src.EmbeddingMemoCapacity != nil {
		dst.EmbeddingMemoCapacity = *src.EmbeddingMemoCapacity
	}
	if src.EmbeddingMemoTTL != nil {
		dst.EmbeddingMemoTTL = *src.EmbeddingMemoTTL
	}
}

func validateCacheEmbedding(cache CacheConfig) error {
	if err := validateEmbeddingPrefix(cache.EmbeddingInputPrefix); err != nil {
		return err
	}
	if cache.EmbeddingMemoCapacity < 0 || cache.EmbeddingMemoCapacity > maxEmbeddingMemoEntries {
		return fmt.Errorf("cache.embedding_memo_capacity must be between 0 and %d", maxEmbeddingMemoEntries)
	}
	if cache.EmbeddingMemoCapacity == 0 && cache.EmbeddingMemoTTL == "" {
		return nil
	}
	ttl, err := time.ParseDuration(cache.EmbeddingMemoTTL)
	if err != nil || ttl <= 0 || ttl > maxEmbeddingMemoTTL || cache.EmbeddingMemoCapacity == 0 {
		return fmt.Errorf("cache.embedding_memo_ttl requires positive capacity and a duration in (0, 1h]")
	}
	return nil
}

func validateEmbeddingPrefix(prefix string) error {
	if len(prefix) > maxEmbeddingPrefixBytes || strings.ContainsRune(prefix, 0) || !utf8.ValidString(prefix) {
		return fmt.Errorf("cache.embedding_input_prefix must be valid UTF-8, at most %d bytes and contain no NUL", maxEmbeddingPrefixBytes)
	}
	return nil
}
