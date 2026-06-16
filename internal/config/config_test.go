package config

import (
	"testing"
)

func TestConfigFallbackDefaults(t *testing.T) {
	// One provider: fallback should be disabled
	c1 := &Config{
		Providers: []ProviderConfig{{ID: "p1"}},
	}
	c1.FallbackEnabled = len(c1.Providers) > 1
	if c1.FallbackEnabled {
		t.Errorf("expected fallback to be disabled for 1 provider")
	}

	// Two providers: fallback should be enabled
	c2 := &Config{
		Providers: []ProviderConfig{{ID: "p1"}, {ID: "p2"}},
	}
	c2.FallbackEnabled = len(c2.Providers) > 1
	if !c2.FallbackEnabled {
		t.Errorf("expected fallback to be enabled for 2 providers")
	}
}

func TestConfigMaxAttemptsValidation(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 2},
		{-1, 1},
		{1, 1},
		{3, 3},
		{6, 5},
	}

	for _, tt := range tests {
		c := &Config{
			RoutingStrategy: "round-robin",
			Providers: []ProviderConfig{
				{ID: "p1", Type: "openai-compatible", BaseURL: "http", Models: []string{"m1"}},
			},
			MaxAttempts: tt.input,
		}

		err := c.Validate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if c.MaxAttempts != tt.expected {
			t.Errorf("for input %d, expected max attempts %d, got %d", tt.input, tt.expected, c.MaxAttempts)
		}
	}
}
