package providers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"veloxmesh/internal/config"
	gwerr "veloxmesh/internal/errors"
)

type ProtectionPolicy struct {
	FirstByte, FirstContent, StreamIdle, Total time.Duration
	permits                                    chan struct{}
}

// One process-local set survives runtime registry replacements. Providers that
// share a resource group share one permit pool, regardless of model/account.
type ProtectionSet struct{ policies map[string]ProtectionPolicy }

func NewProtectionSet(configured map[string]config.ProviderProtectionConfig) (*ProtectionSet, error) {
	if err := config.ValidateProviderProtection(configured); err != nil {
		return nil, err
	}
	policies, groups := make(map[string]ProtectionPolicy), make(map[string]chan struct{})
	for id, value := range configured {
		policy, err := parseProtectionPolicy(value)
		if err != nil {
			return nil, err
		}
		group := value.ResourceGroup
		if group == "" {
			group = id
		}
		if value.MaxInflight > 0 {
			if groups[group] == nil {
				groups[group] = make(chan struct{}, value.MaxInflight)
			}
			policy.permits = groups[group]
		}
		policies[id] = policy
	}
	return &ProtectionSet{policies: policies}, nil
}

func parseProtectionPolicy(value config.ProviderProtectionConfig) (ProtectionPolicy, error) {
	var result ProtectionPolicy
	fields := []struct {
		value  string
		target *time.Duration
	}{
		{value.FirstByteTimeout, &result.FirstByte}, {value.FirstContentTimeout, &result.FirstContent},
		{value.StreamIdleTimeout, &result.StreamIdle}, {value.TotalTimeout, &result.Total},
	}
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		duration, err := time.ParseDuration(field.value)
		if err != nil {
			return ProtectionPolicy{}, err
		}
		*field.target = duration
	}
	return result, nil
}

func (set *ProtectionSet) Wrap(adapter ProviderAdapter) ProviderAdapter {
	if protected, ok := adapter.(interface{ unprotectedAdapter() ProviderAdapter }); ok {
		adapter = protected.unprotectedAdapter()
	}
	policy, exists := set.policies[adapter.ID()]
	if !exists {
		return adapter
	}
	base := &protectedAdapter{ProviderAdapter: adapter, policy: policy}
	stream, hasStream := adapter.(StreamAdapter)
	embed, hasEmbed := adapter.(EmbedAdapter)
	if hasStream && hasEmbed {
		return &protectedBoth{protectedAdapter: base, stream: stream, embed: embed}
	}
	if hasStream {
		return &protectedStream{protectedAdapter: base, stream: stream}
	}
	if hasEmbed {
		return &protectedEmbed{protectedAdapter: base, embed: embed}
	}
	return base
}

func acquireProtectionPermit(ctx context.Context, permits chan struct{}) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if permits == nil {
		return func() {}, nil
	}
	select {
	case permits <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-permits }) }, nil
	default:
		return nil, gwerr.NewGatewayError(gwerr.ProviderConcurrencyFull, "Provider resource concurrency limit reached", http.StatusTooManyRequests)
	}
}
