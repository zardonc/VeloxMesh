package config

import (
	"fmt"
	"strings"
	"time"
)

const maxProviderInflight = 4096
const maxProviderProtectionDuration = time.Hour
const maxProviderResourceGroupBytes = 128

type ProviderProtectionConfig struct {
	FirstByteTimeout    string `json:"first_byte_timeout"`
	FirstContentTimeout string `json:"first_content_timeout"`
	StreamIdleTimeout   string `json:"stream_idle_timeout"`
	TotalTimeout        string `json:"total_timeout"`
	MaxInflight         int    `json:"max_inflight"`
	ResourceGroup       string `json:"resource_group"`
}

func ValidateProviderProtection(policies map[string]ProviderProtectionConfig) error {
	return validateProviderProtection(policies)
}

func validateProviderProtection(policies map[string]ProviderProtectionConfig) error {
	groups := make(map[string]int)
	for id, policy := range policies {
		if id == "" || strings.ContainsRune(id, 0) {
			return fmt.Errorf("provider_protection requires valid provider IDs")
		}
		if err := validateProtectionPolicy(policy); err != nil {
			return fmt.Errorf("provider_protection %s: %w", id, err)
		}
		group := policy.ResourceGroup
		if group == "" {
			group = id
		}
		if limit, exists := groups[group]; exists && limit != policy.MaxInflight {
			return fmt.Errorf("provider_protection shared resource group must use the same max_inflight")
		}
		groups[group] = policy.MaxInflight
	}
	return nil
}

func validateProtectionPolicy(policy ProviderProtectionConfig) error {
	if policy.MaxInflight < 0 || policy.MaxInflight > maxProviderInflight {
		return fmt.Errorf("max_inflight must be between 0 and %d", maxProviderInflight)
	}
	if len(policy.ResourceGroup) > maxProviderResourceGroupBytes || strings.ContainsRune(policy.ResourceGroup, 0) {
		return fmt.Errorf("resource_group must be at most %d bytes and contain no NUL", maxProviderResourceGroupBytes)
	}
	for name, value := range map[string]string{"first_byte_timeout": policy.FirstByteTimeout, "first_content_timeout": policy.FirstContentTimeout,
		"stream_idle_timeout": policy.StreamIdleTimeout, "total_timeout": policy.TotalTimeout} {
		if value == "" {
			continue
		}
		duration, err := time.ParseDuration(value)
		if err != nil || duration <= 0 || duration > maxProviderProtectionDuration {
			return fmt.Errorf("%s must be a positive duration no greater than 1h", name)
		}
	}
	if policy.TotalTimeout == "" && (policy.FirstByteTimeout != "" || policy.FirstContentTimeout != "" || policy.StreamIdleTimeout != "") {
		return fmt.Errorf("phase deadlines require total_timeout")
	}
	return nil
}
