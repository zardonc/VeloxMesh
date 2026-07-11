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
	Rules  map[RuleName]RuleConfig `yaml:"rules" json:"rules"`
	Input  PipelineStageConfig     `yaml:"input" json:"input"`
	Output PipelineStageConfig     `yaml:"output" json:"output"`
}

type PipelineStageConfig struct {
	Rules map[RuleName]RuleConfig `yaml:"rules" json:"rules"`
}

func DefaultSemanticPipelineConfig() *SemanticPipelineConfig {
	cfg := &SemanticPipelineConfig{
		Rules:  defaultRuleMap(),
		Input:  PipelineStageConfig{Rules: map[RuleName]RuleConfig{}},
		Output: PipelineStageConfig{Rules: map[RuleName]RuleConfig{}},
	}
	return cfg
}

func defaultRuleMap() map[RuleName]RuleConfig {
	rules := make(map[RuleName]RuleConfig)
	for _, name := range AllRuleNames {
		rules[name] = RuleConfig{Enabled: false}
	}
	return rules
}

func (c *SemanticPipelineConfig) Validate() error {
	c.applyDefaults()
	for _, rules := range []map[RuleName]RuleConfig{c.Rules, c.Input.Rules, c.Output.Rules} {
		if err := validateRules(rules); err != nil {
			return err
		}
	}
	return nil
}

func validateRules(rules map[RuleName]RuleConfig) error {
	for name, rule := range rules {
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

	caveman := rules[RuleCaveman]
	ponytail := rules[RulePonytail]
	if caveman.Enabled && ponytail.Enabled {
		return fmt.Errorf("caveman and ponytail cannot be enabled simultaneously")
	}

	return nil
}

func (c *SemanticPipelineConfig) applyDefaults() {
	if c.Rules == nil {
		c.Rules = defaultRuleMap()
	}
}

func (c *SemanticPipelineConfig) CanRewriteRequestText(name RuleName) bool {
	if name != RuleCaveman && name != RulePonytail {
		return false
	}
	rule := c.RequestRule(name)
	if !rule.Enabled {
		return false
	}
	opt, exists := rule.Options["rewrite_request_text"]
	if !exists {
		return false
	}
	b, isBool := opt.(bool)
	return isBool && b
}

func (c *SemanticPipelineConfig) RequestRule(name RuleName) RuleConfig {
	c.applyDefaults()
	rule := c.Rules[name]
	stage, ok := c.Input.Rules[name]
	if ok {
		rule = stage
	}
	return rule
}

func (c *SemanticPipelineConfig) ResponseRule(name RuleName) RuleConfig {
	c.applyDefaults()
	rule := c.Rules[name]
	stage, ok := c.Output.Rules[name]
	if ok {
		rule = stage
	}
	return rule
}

func ResolveSemanticRuleConfig(global, user *SemanticPipelineConfig) *SemanticPipelineConfig {
	resolved := DefaultSemanticPipelineConfig()

	if global != nil {
		mergePipelineConfig(resolved, global)
	}
	if user != nil {
		mergePipelineConfig(resolved, user)
	}

	return resolved
}

func mergePipelineConfig(dst, src *SemanticPipelineConfig) {
	src.applyDefaults()
	for name, rule := range src.Rules {
		dst.Rules[name] = rule
		dst.Input.Rules[name] = rule
		dst.Output.Rules[name] = rule
	}
	for name, rule := range src.Input.Rules {
		dst.Input.Rules[name] = rule
	}
	for name, rule := range src.Output.Rules {
		dst.Output.Rules[name] = rule
	}
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
