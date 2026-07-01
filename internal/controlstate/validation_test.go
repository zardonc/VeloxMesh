package controlstate

import (
	"testing"
)

func TestValidateRoutingConfig(t *testing.T) {
	tests := []struct {
		name            string
		rc              *RoutingConfig
		providers       []*ProviderRecord
		backend         string
		redisConfigured bool
		wantErr         bool
	}{
		{
			name: "Valid sqlite round-robin",
			rc: &RoutingConfig{
				Strategy:        "round-robin",
				FallbackEnabled: false,
				MaxAttempts:     1,
			},
			providers: []*ProviderRecord{},
			backend:   "sqlite",
			wantErr:   false,
		},
		{
			name: "Valid postgres priority with fallback",
			rc: &RoutingConfig{
				Strategy:        "priority",
				DefaultProvider: "p1",
				FallbackEnabled: true,
				MaxAttempts:     2,
			},
			providers: []*ProviderRecord{
				{ID: "p1", Enabled: true},
				{ID: "p2", Enabled: true},
			},
			backend:         "postgres",
			redisConfigured: true,
			wantErr:         false,
		},
		{
			name: "Invalid strategy",
			rc: &RoutingConfig{
				Strategy: "random",
			},
			wantErr: true,
		},
		{
			name: "Missing active default provider",
			rc: &RoutingConfig{
				Strategy:        "priority",
				DefaultProvider: "p3",
			},
			providers: []*ProviderRecord{
				{ID: "p1", Enabled: true},
			},
			wantErr: true,
		},
		{
			name: "Fallback disabled forces one attempt",
			rc: &RoutingConfig{
				Strategy:        "round-robin",
				FallbackEnabled: false,
				MaxAttempts:     2,
			},
			wantErr: true,
		},
		{
			name: "Fallback enabled caps at active providers",
			rc: &RoutingConfig{
				Strategy:        "priority",
				FallbackEnabled: true,
				MaxAttempts:     3,
			},
			providers: []*ProviderRecord{
				{ID: "p1", Enabled: true},
			},
			wantErr: true,
		},
		{
			name: "Postgres without Redis is invalid",
			rc: &RoutingConfig{
				Strategy:        "round-robin",
				FallbackEnabled: false,
				MaxAttempts:     1,
			},
			backend:         "postgres",
			redisConfigured: false,
			wantErr:         true,
		},
		{
			name: "Valid composite-score",
			rc: &RoutingConfig{
				Strategy:        "composite-score",
				FallbackEnabled: false,
				MaxAttempts:     1,
				Composite: &CompositeRoutingConfig{
					PresetName:        "conservative",
					LatencyWeight:     0.5,
					LoadWeight:        0.5,
					HealthWeight:      0.0,
					ErrorRateWeight:   0.0,
					ScoreThreshold:    0.5,
					NearTieThreshold:  0.1,
					WarmUpSuccesses:   5,
					StaleMetricWindow: "2m",
				},
			},
			backend: "sqlite",
			wantErr: false,
		},
		{
			name: "Invalid composite weight",
			rc: &RoutingConfig{
				Strategy:        "composite-score",
				FallbackEnabled: false,
				MaxAttempts:     1,
				Composite: &CompositeRoutingConfig{
					LatencyWeight:     1.5,
					LoadWeight:        0.5,
					WarmUpSuccesses:   5,
					StaleMetricWindow: "2m",
				},
			},
			backend: "sqlite",
			wantErr: true,
		},
		{
			name: "Invalid composite warm up successes",
			rc: &RoutingConfig{
				Strategy:        "composite-score",
				FallbackEnabled: false,
				MaxAttempts:     1,
				Composite: &CompositeRoutingConfig{
					LatencyWeight:     0.5,
					LoadWeight:        0.5,
					WarmUpSuccesses:   0,
					StaleMetricWindow: "2m",
				},
			},
			backend: "sqlite",
			wantErr: true,
		},
		{
			name: "Invalid composite stale window",
			rc: &RoutingConfig{
				Strategy:        "composite-score",
				FallbackEnabled: false,
				MaxAttempts:     1,
				Composite: &CompositeRoutingConfig{
					LatencyWeight:     0.5,
					LoadWeight:        0.5,
					WarmUpSuccesses:   5,
					StaleMetricWindow: "-1m",
				},
			},
			backend: "sqlite",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateRoutingConfig(tt.rc, tt.providers, tt.backend, tt.redisConfigured)
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("ValidateRoutingConfig() errs = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}
