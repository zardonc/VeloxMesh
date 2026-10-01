package config

import (
	"fmt"
	"time"
)

func mergeCacheBounds(dst *CacheConfig, src *cacheFileConfig) {
	if src.ReadTimeout != "" {
		dst.ReadTimeout = src.ReadTimeout
	}
	if src.ReadConcurrency != nil {
		dst.ReadConcurrency = *src.ReadConcurrency
	}
	if src.WriteTimeout != "" {
		dst.WriteTimeout = src.WriteTimeout
	}
	if src.WriteWorkers != nil {
		dst.WriteWorkers = *src.WriteWorkers
	}
	if src.QueueCapacity != nil {
		dst.QueueCapacity = *src.QueueCapacity
	}
	if src.ShutdownGrace != "" {
		dst.ShutdownGrace = src.ShutdownGrace
	}
}

func validateCacheBounds(cache CacheConfig) error {
	for name, value := range map[string]string{"read_timeout": cache.ReadTimeout, "write_timeout": cache.WriteTimeout, "shutdown_grace": cache.ShutdownGrace} {
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 {
			return fmt.Errorf("cache.%s must be a positive duration", name)
		}
	}
	if cache.ReadConcurrency < 1 || cache.WriteWorkers < 1 || cache.QueueCapacity < 1 {
		return fmt.Errorf("cache read_concurrency, write_workers and queue_capacity must be positive")
	}
	return nil
}
