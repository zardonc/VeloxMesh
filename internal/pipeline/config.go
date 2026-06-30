package pipeline

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v4"
)

type RuleName string

const (
	RuleRTK      RuleName = "rtk"
	RuleHeadroom RuleName = "headroom"
	RulePII      RuleName = "pii"
	RuleRewrite  RuleName = "rewrite"
	RuleCaveman  RuleName = "caveman"
	RulePonytail RuleName = "ponytail"
	RuleFilter   RuleName = "filter"
)

var AllRuleNames = []RuleName{
	RuleRTK, RuleHeadroom, RulePII, RuleRewrite, RuleCaveman, RulePonytail, RuleFilter,
}

type RuleConfig struct {
	Enabled bool                   `yaml:"enabled" json:"enabled"`
	Options map[string]interface{} `yaml:"options,omitempty" json:"options,omitempty"`
}

type SemanticPipelineConfig struct {
	Rules map[RuleName]RuleConfig `yaml:"rules" json:"rules"`
}

func DefaultSemanticPipelineConfig() *SemanticPipelineConfig {
	cfg := &SemanticPipelineConfig{
		Rules: make(map[RuleName]RuleConfig),
	}
	for _, name := range AllRuleNames {
		cfg.Rules[name] = RuleConfig{Enabled: false}
	}
	return cfg
}

func (c *SemanticPipelineConfig) Validate() error {
	for name, rule := range c.Rules {
		valid := false
		for _, validName := range AllRuleNames {
			if name == validName {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("unknown rule name: %s", name)
		}

		if rule.Enabled && (name == RuleCaveman || name == RulePonytail) {
			if opt, exists := rule.Options["rewrite_request_text"]; exists {
				if _, isBool := opt.(bool); !isBool {
					return fmt.Errorf("rewrite_request_text option must be a boolean for rule %s", name)
				}
			}
		}
	}

	caveman := c.Rules[RuleCaveman]
	ponytail := c.Rules[RulePonytail]
	if caveman.Enabled && ponytail.Enabled {
		return fmt.Errorf("caveman and ponytail cannot be enabled simultaneously")
	}

	return nil
}

func (c *SemanticPipelineConfig) CanRewriteRequestText(name RuleName) bool {
	if name != RuleCaveman && name != RulePonytail {
		return false
	}
	rule, ok := c.Rules[name]
	if !ok || !rule.Enabled {
		return false
	}
	opt, exists := rule.Options["rewrite_request_text"]
	if !exists {
		return false
	}
	b, isBool := opt.(bool)
	return isBool && b
}

func ResolveSemanticRuleConfig(global, user *SemanticPipelineConfig) *SemanticPipelineConfig {
	resolved := DefaultSemanticPipelineConfig()

	if global != nil {
		for name, rule := range global.Rules {
			resolved.Rules[name] = rule
		}
	}
	if user != nil {
		for name, rule := range user.Rules {
			resolved.Rules[name] = rule
		}
	}

	return resolved
}

func LoadSemanticPipelineConfigFile(path string) (*SemanticPipelineConfig, error) {
	if path == "" {
		return DefaultSemanticPipelineConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSemanticPipelineConfig(), nil
		}
		return nil, fmt.Errorf("failed to read semantic pipeline config file: %v", err)
	}

	cfg := DefaultSemanticPipelineConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse semantic pipeline config file: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid semantic pipeline config: %v", err)
	}

	return cfg, nil
}
