package config

import (
	"os"
	"strconv"
)

const (
	defaultSemanticCacheVectorDimension = 1536
	defaultPGVectorIndexType            = "hnsw"
	defaultPGVectorHNSWM                = 16
	defaultPGVectorHNSWEFConstruction   = 64
	defaultPGVectorSearchEF             = 40
	defaultSLAPromotionCandidateWindow  = 32
)

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}

func getEnvFloat(key string, fallback float64) float64 {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
