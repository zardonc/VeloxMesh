package config

import (
	"path/filepath"
	"testing"

	"veloxmesh/internal/pipeline"
)

func TestDeployExampleConfigsLoad(t *testing.T) {
	tests := []struct {
		name              string
		appConfig         string
		schedulerConfig   string
		cacheConfig       string
		pipelineConfig    string
		redisEnabled      bool
		cacheEnabled      bool
		heuristicEndpoint string
	}{
		{
			name:              "simple",
			appConfig:         "app.simple.example.json",
			schedulerConfig:   "scheduler.simple.example.json",
			cacheConfig:       "cache.simple.example.json",
			pipelineConfig:    "pipeline.simple.example.yaml",
			heuristicEndpoint: "scheduler-onnx:50051",
		},
		{
			name:              "full",
			appConfig:         "app.full.example.json",
			schedulerConfig:   "scheduler.full.example.json",
			cacheConfig:       "cache.full.example.json",
			pipelineConfig:    "pipeline.full.example.yaml",
			redisEnabled:      true,
			cacheEnabled:      true,
			heuristicEndpoint: "scheduler-onnx:50051",
		},
		{
			name:              "compare",
			appConfig:         "app.compare.example.json",
			schedulerConfig:   "scheduler.compare.example.json",
			cacheConfig:       "cache.compare.example.json",
			pipelineConfig:    "pipeline.compare.example.yaml",
			heuristicEndpoint: "scheduler-heuristic:50051",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CONFIG_FILE", deployConfigPath(tc.appConfig))
			t.Setenv("SCHEDULER_CONFIG_FILE", deployConfigPath(tc.schedulerConfig))
			t.Setenv("CACHE_CONFIG_FILE", deployConfigPath(tc.cacheConfig))
			t.Setenv("SEMANTIC_PIPELINE_CONFIG_FILE", deployConfigPath(tc.pipelineConfig))
			t.Setenv("OPENAI_PRIMARY_API_KEY", "test-provider-key")

			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.Redis.Enabled != tc.redisEnabled {
				t.Fatalf("redis enabled=%v, want %v", cfg.Redis.Enabled, tc.redisEnabled)
			}
			if cfg.Cache.Enabled != tc.cacheEnabled {
				t.Fatalf("cache enabled=%v, want %v", cfg.Cache.Enabled, tc.cacheEnabled)
			}
			if cfg.Scheduler.HeuristicEndpoint != tc.heuristicEndpoint {
				t.Fatalf("heuristic endpoint=%q, want %q", cfg.Scheduler.HeuristicEndpoint, tc.heuristicEndpoint)
			}
			if cfg.Scheduler.ONNXEndpoint != "scheduler-onnx:50051" {
				t.Fatalf("onnx endpoint=%q", cfg.Scheduler.ONNXEndpoint)
			}
			pipelineCfg, err := pipeline.LoadSemanticPipelineConfigFile(cfg.SemanticPipelineConfigFile)
			if err != nil {
				t.Fatalf("LoadSemanticPipelineConfigFile: %v", err)
			}
			for name, rule := range pipelineCfg.Input.Rules {
				if rule.Enabled {
					t.Fatalf("input rule %s enabled in example", name)
				}
			}
			for name, rule := range pipelineCfg.Output.Rules {
				if rule.Enabled {
					t.Fatalf("output rule %s enabled in example", name)
				}
			}
		})
	}
}

func deployConfigPath(name string) string {
	return filepath.Join("..", "..", "deploy", "config", name)
}
