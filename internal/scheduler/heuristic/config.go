package heuristic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"veloxmesh/internal/scheduler"
)

type Config struct {
	Version               string
	BaseLatencyMs         map[scheduler.RequestKind]int64
	ModelMultiplier       map[string]float64
	PriorityMultiplier    map[scheduler.PriorityClass]float64
	UncertaintyPenaltyK   float64
	ToolCallPenaltyMs     int64
	StreamDiscountPercent int64
}

func DefaultConfig() Config {
	return Config{
		Version: "heuristic-v1",
		BaseLatencyMs: map[scheduler.RequestKind]int64{
			scheduler.RequestKindSimpleQA:         800,
			scheduler.RequestKindCodeGen:          4000,
			scheduler.RequestKindCodeReview:       2500,
			scheduler.RequestKindSummarization:    1800,
			scheduler.RequestKindTranslation:      1200,
			scheduler.RequestKindStructuredOutput: 2200,
			scheduler.RequestKindMultiStep:        3500,
			scheduler.RequestKindToolCall:         3000,
			scheduler.RequestKindRAG:              2800,
			scheduler.RequestKindCreative:         3200,
		},
		ModelMultiplier: map[string]float64{
			"small":    0.7,
			"standard": 1,
			"large":    1.4,
		},
		PriorityMultiplier: map[scheduler.PriorityClass]float64{
			scheduler.PriorityHigh:   2,
			scheduler.PriorityNormal: 1,
			scheduler.PriorityLow:    0.5,
		},
		UncertaintyPenaltyK:   0.2,
		ToolCallPenaltyMs:     500,
		StreamDiscountPercent: 10,
	}
}

type overrideConfig struct {
	BaseLatency     map[string]int64   `json:"base_latency"`
	ModelMultiplier map[string]float64 `json:"model_multipliers"`
}

func LoadConfigFile(path string, base Config) (Config, error) {
	if path == "" {
		return cloneConfig(base), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read heuristic_config_file: %w", err)
	}
	var override overrideConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&override); err != nil {
		return Config{}, fmt.Errorf("parse heuristic_config_file: %w", err)
	}
	cfg := cloneConfig(base)
	if err := applyBaseLatencyOverrides(cfg, override.BaseLatency); err != nil {
		return Config{}, err
	}
	applyModelMultiplierOverrides(cfg, override.ModelMultiplier)
	return cfg, nil
}

func cloneConfig(cfg Config) Config {
	if cfg.Version == "" {
		cfg = DefaultConfig()
	}
	cfg.BaseLatencyMs = cloneMap(cfg.BaseLatencyMs)
	cfg.ModelMultiplier = cloneMap(cfg.ModelMultiplier)
	cfg.PriorityMultiplier = cloneMap(cfg.PriorityMultiplier)
	return cfg
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func applyBaseLatencyOverrides(cfg Config, values map[string]int64) error {
	for key, value := range values {
		kind := scheduler.RequestKind(key)
		if _, ok := cfg.BaseLatencyMs[kind]; !ok {
			return fmt.Errorf("heuristic_config_file base_latency has unknown request kind %q", key)
		}
		cfg.BaseLatencyMs[kind] = value
	}
	return nil
}

func applyModelMultiplierOverrides(cfg Config, values map[string]float64) {
	for key, value := range values {
		cfg.ModelMultiplier[key] = value
	}
}
