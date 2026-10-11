package config

import (
	"fmt"
	"time"
)

const (
	CacheReuseDisabled = "disabled"
	CacheReuseExact    = "exact"
	CacheReuseSemantic = "semantic"
)

// Omitted or unrecognized policies never implicitly authorize answer reuse.
func EffectiveCacheReuseMode(mode string) string {
	switch mode {
	case CacheReuseExact, CacheReuseSemantic:
		return mode
	default:
		return CacheReuseDisabled
	}
}

func (c CacheConfig) HasSemanticReuse() bool {
	for _, profile := range c.UseCases {
		if profile.ReuseMode == CacheReuseSemantic {
			return true
		}
	}
	return false
}

func (c CacheConfig) HasAnswerReuse() bool {
	for _, profile := range c.UseCases {
		if EffectiveCacheReuseMode(profile.ReuseMode) != CacheReuseDisabled {
			return true
		}
	}
	return false
}

func validateNonSemanticCache(cache CacheConfig) error {
	if err := validateCacheUseCases(cache.UseCases); err != nil {
		return err
	}
	if !cache.HasAnswerReuse() {
		return nil
	}
	ttl, err := time.ParseDuration(cache.TTL)
	if err != nil || ttl <= 0 {
		return fmt.Errorf("cache ttl must be a positive duration")
	}
	return validateCacheBounds(cache)
}
