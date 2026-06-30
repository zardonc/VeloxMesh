package sqlite

import (
	"context"
	"testing"

	"veloxmesh/internal/pipeline"
)

func TestSemanticRule(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Run migrations
	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	store := repo.SemanticRules()

	t.Run("empty store resolves every rule disabled", func(t *testing.T) {
		cfg, err := store.GetGlobalDefaults(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Rules) != 7 {
			t.Errorf("expected 7 default rules, got %d", len(cfg.Rules))
		}
		for name, rule := range cfg.Rules {
			if rule.Enabled {
				t.Errorf("expected rule %s to be disabled", name)
			}
		}
	})

	t.Run("global defaults apply", func(t *testing.T) {
		cfg := pipeline.DefaultSemanticPipelineConfig()
		cfg.Rules[pipeline.RuleRTK] = pipeline.RuleConfig{Enabled: true}
		
		if err := store.SaveGlobalDefaults(ctx, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		loaded, err := store.GetGlobalDefaults(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !loaded.Rules[pipeline.RuleRTK].Enabled {
			t.Error("expected RTK to be enabled globally")
		}
	})

	t.Run("user isolation and precedence", func(t *testing.T) {
		userA := &pipeline.SemanticPipelineConfig{
			Rules: map[pipeline.RuleName]pipeline.RuleConfig{
				pipeline.RuleHeadroom: {Enabled: true},
			},
		}
		if err := store.SaveUserConfig(ctx, "user-A", userA); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		userB := &pipeline.SemanticPipelineConfig{
			Rules: map[pipeline.RuleName]pipeline.RuleConfig{
				pipeline.RulePII: {Enabled: true},
			},
		}
		if err := store.SaveUserConfig(ctx, "user-B", userB); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cfgA, err := store.GetUserConfig(ctx, "user-A")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfgA.Rules[pipeline.RuleHeadroom].Enabled {
			t.Error("expected Headroom enabled for user A")
		}
		if cfgA.Rules[pipeline.RulePII].Enabled {
			t.Error("expected PII disabled for user A")
		}

		cfgB, err := store.GetUserConfig(ctx, "user-B")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfgB.Rules[pipeline.RuleHeadroom].Enabled {
			t.Error("expected Headroom disabled for user B")
		}
		if !cfgB.Rules[pipeline.RulePII].Enabled {
			t.Error("expected PII enabled for user B")
		}
	})

	t.Run("caveman and ponytail exclusivity", func(t *testing.T) {
		cfg := pipeline.DefaultSemanticPipelineConfig()
		cfg.Rules[pipeline.RuleCaveman] = pipeline.RuleConfig{Enabled: true}
		cfg.Rules[pipeline.RulePonytail] = pipeline.RuleConfig{Enabled: true}

		err := store.SaveGlobalDefaults(ctx, cfg)
		if err == nil {
			t.Error("expected error saving global defaults with both caveman and ponytail enabled")
		}

		userCfg := &pipeline.SemanticPipelineConfig{
			Rules: map[pipeline.RuleName]pipeline.RuleConfig{
				pipeline.RuleCaveman: {Enabled: true},
				pipeline.RulePonytail: {Enabled: true},
			},
		}
		err = store.SaveUserConfig(ctx, "user-C", userCfg)
		if err == nil {
			t.Error("expected error saving user config with both caveman and ponytail enabled")
		}
	})
	
	t.Run("rewrite_request_text", func(t *testing.T) {
		cfg := pipeline.DefaultSemanticPipelineConfig()
		cfg.Rules[pipeline.RuleCaveman] = pipeline.RuleConfig{
			Enabled: true,
			Options: map[string]interface{}{"rewrite_request_text": "yes"}, // string instead of bool
		}

		err := store.SaveGlobalDefaults(ctx, cfg)
		if err == nil {
			t.Error("expected error saving invalid rewrite_request_text type")
		}

		cfg.Rules[pipeline.RuleCaveman] = pipeline.RuleConfig{
			Enabled: true,
			Options: map[string]interface{}{"rewrite_request_text": true},
		}

		err = store.SaveGlobalDefaults(ctx, cfg)
		if err != nil {
			t.Errorf("expected success saving valid rewrite_request_text, got %v", err)
		}
	})
}
