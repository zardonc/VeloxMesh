package pipeline

import (
	"os"
	"testing"
)

func TestSemanticRuleConfig(t *testing.T) {
	t.Run("default config has all rules disabled", func(t *testing.T) {
		cfg := DefaultSemanticPipelineConfig()
		if len(cfg.Rules) != 7 {
			t.Errorf("expected 7 rules, got %d", len(cfg.Rules))
		}
		for name, rule := range cfg.Rules {
			if rule.Enabled {
				t.Errorf("expected rule %s to be disabled", name)
			}
		}
	})

	t.Run("user config overrides global defaults", func(t *testing.T) {
		global := DefaultSemanticPipelineConfig()
		global.Rules[RuleRTK] = RuleConfig{Enabled: true}
		global.Rules[RuleHeadroom] = RuleConfig{Enabled: true}

		user := &SemanticPipelineConfig{Rules: make(map[RuleName]RuleConfig)}
		user.Rules[RuleHeadroom] = RuleConfig{Enabled: false} // Override to false
		user.Rules[RulePII] = RuleConfig{Enabled: true}

		resolved := ResolveSemanticRuleConfig(global, user)

		if !resolved.Rules[RuleRTK].Enabled {
			t.Error("expected RTK to be enabled from global")
		}
		if resolved.Rules[RuleHeadroom].Enabled {
			t.Error("expected Headroom to be disabled from user override")
		}
		if !resolved.Rules[RulePII].Enabled {
			t.Error("expected PII to be enabled from user")
		}
		if resolved.Rules[RuleFilter].Enabled {
			t.Error("expected Filter to be disabled by default")
		}
	})

	t.Run("caveman and ponytail cannot be enabled together", func(t *testing.T) {
		cfg := DefaultSemanticPipelineConfig()
		cfg.Rules[RuleCaveman] = RuleConfig{Enabled: true}
		cfg.Rules[RulePonytail] = RuleConfig{Enabled: true}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error when both caveman and ponytail are enabled")
		}
	})

	t.Run("rewrite_request_text is false unless explicitly true", func(t *testing.T) {
		cfg := DefaultSemanticPipelineConfig()
		
		// Rule is enabled, but no rewrite option
		cfg.Rules[RuleCaveman] = RuleConfig{Enabled: true}
		if cfg.CanRewriteRequestText(RuleCaveman) {
			t.Error("expected CanRewriteRequestText to be false when option is missing")
		}

		// Option is explicitly true
		cfg.Rules[RuleCaveman] = RuleConfig{
			Enabled: true,
			Options: map[string]interface{}{"rewrite_request_text": true},
		}
		if !cfg.CanRewriteRequestText(RuleCaveman) {
			t.Error("expected CanRewriteRequestText to be true")
		}

		// Validation should pass
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected validation to pass, got %v", err)
		}

		// Option is string instead of bool
		cfg.Rules[RuleCaveman] = RuleConfig{
			Enabled: true,
			Options: map[string]interface{}{"rewrite_request_text": "yes"},
		}
		if cfg.CanRewriteRequestText(RuleCaveman) {
			t.Error("expected CanRewriteRequestText to be false when option is not a bool")
		}
		
		if err := cfg.Validate(); err == nil {
			t.Error("expected validation to fail because rewrite_request_text is not a boolean")
		}
	})

	t.Run("validation rejects unknown rules", func(t *testing.T) {
		cfg := DefaultSemanticPipelineConfig()
		cfg.Rules[RuleName("invalid_rule")] = RuleConfig{Enabled: true}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected validation to fail on unknown rule")
		}
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("empty path returns defaults", func(t *testing.T) {
		cfg, err := LoadSemanticPipelineConfigFile("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.Rules) != 7 {
			t.Errorf("expected defaults, got %d rules", len(cfg.Rules))
		}
	})

	t.Run("loads valid yaml", func(t *testing.T) {
		yamlData := []byte(`
rules:
  caveman:
    enabled: true
    options:
      rewrite_request_text: true
  rtk:
    enabled: true
`)
		tmpfile, err := os.CreateTemp("", "semantic-config-*.yaml")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		if _, err := tmpfile.Write(yamlData); err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()

		cfg, err := LoadSemanticPipelineConfigFile(tmpfile.Name())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cfg.Rules[RuleCaveman].Enabled {
			t.Error("expected caveman to be enabled")
		}
		if !cfg.CanRewriteRequestText(RuleCaveman) {
			t.Error("expected caveman to allow request rewrite")
		}
		if !cfg.Rules[RuleRTK].Enabled {
			t.Error("expected rtk to be enabled")
		}
		if cfg.Rules[RulePonytail].Enabled {
			t.Error("expected ponytail to be disabled")
		}
	})
}
