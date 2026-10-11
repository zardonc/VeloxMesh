package config

// Failure cases precede implementation: malformed/negative/unbounded deadlines,
// invalid concurrency, ambiguous shared-group capacities, unknown provider keys.
import "testing"

func TestProviderProtectionValidation(t *testing.T) {
	valid := ProviderProtectionConfig{FirstByteTimeout: "1s", FirstContentTimeout: "2s", StreamIdleTimeout: "1s", TotalTimeout: "10s", MaxInflight: 4, ResourceGroup: "shared-local"}
	if err := validateProviderProtection(map[string]ProviderProtectionConfig{"chat": valid}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(ProviderProtectionConfig) ProviderProtectionConfig{
		func(value ProviderProtectionConfig) ProviderProtectionConfig {
			value.FirstByteTimeout = "bad"
			return value
		},
		func(value ProviderProtectionConfig) ProviderProtectionConfig {
			value.FirstContentTimeout = "-1s"
			return value
		},
		func(value ProviderProtectionConfig) ProviderProtectionConfig { value.MaxInflight = -1; return value },
		func(value ProviderProtectionConfig) ProviderProtectionConfig {
			value.ResourceGroup = "bad\x00group"
			return value
		},
	} {
		if err := validateProviderProtection(map[string]ProviderProtectionConfig{"chat": mutate(valid)}); err == nil {
			t.Fatal("invalid protection policy accepted")
		}
	}
	other := valid
	other.MaxInflight = 8
	if err := validateProviderProtection(map[string]ProviderProtectionConfig{"chat": valid, "embedding": other}); err == nil {
		t.Fatal("inconsistent shared-resource capacity accepted")
	}
}
