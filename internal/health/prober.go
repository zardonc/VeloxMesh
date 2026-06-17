package health

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"veloxmesh/internal/config"
	"veloxmesh/internal/providers"
)

type ProbeResult struct {
	ProviderID string
	Available  bool
	Message    string
	Duration   time.Duration
	Timestamp  time.Time
}

type Prober struct {
	registry *providers.Registry
	store    Store
	cfg      *config.Config
	logger   *slog.Logger
}

func NewProber(registry *providers.Registry, store Store, cfg *config.Config, logger *slog.Logger) *Prober {
	if logger == nil {
		logger = slog.Default()
	}
	return &Prober{
		registry: registry,
		store:    store,
		cfg:      cfg,
		logger:   logger,
	}
}

func (p *Prober) getProviderConfig(id string) *config.ProviderConfig {
	for i := range p.cfg.Providers {
		if p.cfg.Providers[i].ID == id {
			return &p.cfg.Providers[i]
		}
	}
	return nil
}

func (p *Prober) getTimeout(providerID string) time.Duration {
	timeoutStr := p.cfg.HealthCheck.Timeout

	provCfg := p.getProviderConfig(providerID)
	if provCfg != nil && provCfg.HealthCheck != nil && provCfg.HealthCheck.Timeout != "" {
		timeoutStr = provCfg.HealthCheck.Timeout
	}

	d, err := time.ParseDuration(timeoutStr)
	if err != nil || d <= 0 {
		return 2 * time.Second
	}
	return d
}

func (p *Prober) ProbeProvider(ctx context.Context, providerID string) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		ProviderID: providerID,
		Timestamp:  start,
	}

	adapter, err := p.registry.Get(providerID)
	if err != nil {
		result.Available = false
		result.Message = err.Error()
		result.Duration = time.Since(start)
		p.store.RecordProbe(providerID, false, result.Duration, result.Message)
		return result
	}

	timeoutDur := p.getTimeout(providerID)
	ctx, cancel := context.WithTimeout(ctx, timeoutDur)
	defer cancel()

	status := adapter.HealthCheck(ctx)
	result.Duration = time.Since(start)
	result.Available = status.Available
	result.Message = status.Message

	p.store.RecordProbe(providerID, result.Available, result.Duration, result.Message)
	return result
}

func (p *Prober) ProbeOnce(ctx context.Context) {
	adapters := p.registry.List()

	concurrency := p.cfg.HealthCheck.MaxConcurrency
	if concurrency < 1 {
		concurrency = 4
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, adapter := range adapters {
		wg.Add(1)
		id := adapter.ID()

		go func(providerID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := p.ProbeProvider(ctx, providerID)
			if !res.Available {
				p.logger.Debug("probe failed", "provider", providerID, "error", res.Message, "duration", res.Duration)
			}
		}(id)
	}

	wg.Wait()
}

func (p *Prober) Start(ctx context.Context) {
	if p.cfg.HealthCheck.Enabled != nil && !*p.cfg.HealthCheck.Enabled {
		return
	}

	concurrency := p.cfg.HealthCheck.MaxConcurrency
	if concurrency < 1 {
		concurrency = 4
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	adapters := p.registry.List()
	for _, adapter := range adapters {
		provCfg := p.getProviderConfig(adapter.ID())
		enabled := true
		if provCfg != nil && provCfg.HealthCheck != nil && provCfg.HealthCheck.Enabled != nil {
			enabled = *provCfg.HealthCheck.Enabled
		}
		if !enabled {
			continue
		}

		intervalStr := p.cfg.HealthCheck.Interval
		if provCfg != nil && provCfg.HealthCheck != nil && provCfg.HealthCheck.Interval != "" {
			intervalStr = provCfg.HealthCheck.Interval
		}
		interval, err := time.ParseDuration(intervalStr)
		if err != nil || interval <= 0 {
			interval = 30 * time.Second
		}

		initialDelayStr := p.cfg.HealthCheck.InitialDelay
		if provCfg != nil && provCfg.HealthCheck != nil && provCfg.HealthCheck.InitialDelay != "" {
			initialDelayStr = provCfg.HealthCheck.InitialDelay
		}
		initialDelay, err := time.ParseDuration(initialDelayStr)

		wg.Add(1)
		go func(id string, intv, del time.Duration) {
			defer wg.Done()
			if err == nil && del > 0 {
				select {
				case <-time.After(del):
				case <-ctx.Done():
					return
				}
			}

			ticker := time.NewTicker(intv)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					sem <- struct{}{}
					res := p.ProbeProvider(ctx, id)
					if !res.Available {
						p.logger.Debug("probe failed", "provider", id, "error", res.Message, "duration", res.Duration)
					}
					<-sem
				}
			}
		}(adapter.ID(), interval, initialDelay)
	}

	// This blocks until ctx is canceled and all loops exit
	wg.Wait()
}
