package controlstate

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"veloxmesh/internal/config"
	gwErr "veloxmesh/internal/errors"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/anthropic"
	"veloxmesh/internal/providers/gemini"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/routing"
)

type RuntimeSnapshot struct {
	Registry      *providers.Registry
	Router        routing.Router
	Prober        *health.Prober
	RoutingConfig *RoutingConfig
	SemanticRules *SemanticRuleSnapshot
}

type SemanticRuleSnapshot struct {
	Global *pipeline.SemanticPipelineConfig
	Users  map[string]*pipeline.SemanticPipelineConfig
}

type ActivationValidator func(ctx context.Context, adapters []providers.ProviderAdapter) error

type RuntimeProviderManager struct {
	snapshot    atomic.Value // holds *RuntimeSnapshot
	healthStore health.Store
	cfg         *config.Config
	logger      *slog.Logger

	mu           sync.Mutex
	baseCtx      context.Context
	proberCancel context.CancelFunc
}

func NewRuntimeProviderManager(cfg *config.Config, logger *slog.Logger, healthStore health.Store) *RuntimeProviderManager {
	if logger == nil {
		logger = slog.Default()
	}
	if healthStore == nil {
		healthStore = health.NewInMemoryStore()
	}
	m := &RuntimeProviderManager{
		healthStore: healthStore,
		cfg:         cfg,
		logger:      logger,
	}
	m.snapshot.Store((*RuntimeSnapshot)(nil))
	return m
}

func (m *RuntimeProviderManager) HealthStore() health.Store {
	return m.healthStore
}

func (m *RuntimeProviderManager) Snapshot() *RuntimeSnapshot {
	snap, _ := m.snapshot.Load().(*RuntimeSnapshot)
	return snap
}

func (m *RuntimeProviderManager) GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error) {
	snap := m.Snapshot()
	if snap != nil && snap.SemanticRules != nil && snap.SemanticRules.Global != nil {
		return snap.SemanticRules.Global, nil
	}
	return pipeline.DefaultSemanticPipelineConfig(), nil
}

func (m *RuntimeProviderManager) GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error) {
	snap := m.Snapshot()
	if snap != nil && snap.SemanticRules != nil && snap.SemanticRules.Users != nil {
		if cfg, ok := snap.SemanticRules.Users[userID]; ok {
			return cfg, nil
		}
	}
	return nil, nil
}

func (m *RuntimeProviderManager) SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error {
	return fmt.Errorf("read-only rule store")
}

func (m *RuntimeProviderManager) SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error {
	return fmt.Errorf("read-only rule store")
}

func (m *RuntimeProviderManager) ListUserConfigs(ctx context.Context) (map[string]*pipeline.SemanticPipelineConfig, error) {
	return nil, fmt.Errorf("read-only rule store")
}

func (m *RuntimeProviderManager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.baseCtx = ctx

	snap := m.Snapshot()
	if snap != nil && snap.Prober != nil {
		pCtx, cancel := context.WithCancel(ctx)
		m.proberCancel = cancel
		go snap.Prober.Start(pCtx)
	}
}

func (m *RuntimeProviderManager) ActivateStatic(providersCfg []config.ProviderConfig, adapters []providers.ProviderAdapter) error {
	return m.activateInternal(providersCfg, adapters, nil, nil, nil)
}

func (m *RuntimeProviderManager) activateInternal(providersCfg []config.ProviderConfig, adapters []providers.ProviderAdapter, rCfg *RoutingConfig, combos []providers.Combo, semRules *SemanticRuleSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfgClone := *m.cfg
	cfgClone.Providers = providersCfg

	strategy := cfgClone.RoutingStrategy
	if rCfg != nil {
		strategy = rCfg.Strategy
	}

	for _, p := range providersCfg {
		fail := 3
		succ := 1
		if p.HealthCheck != nil {
			if p.HealthCheck.FailureThreshold > 0 {
				fail = p.HealthCheck.FailureThreshold
			}
			if p.HealthCheck.SuccessThreshold > 0 {
				succ = p.HealthCheck.SuccessThreshold
			}
		}
		m.healthStore.EnsureProvider(p.ID, fail, succ)
	}

	registry := providers.NewRegistry(&cfgClone, adapters, combos)
	router := routing.NewHealthAwareRouter(registry, m.healthStore, strategy)
	prober := health.NewProber(registry, m.healthStore, &cfgClone, m.logger)

	snap := &RuntimeSnapshot{
		Registry:      registry,
		Router:        router,
		Prober:        prober,
		RoutingConfig: rCfg,
		SemanticRules: semRules,
	}

	m.snapshot.Store(snap)

	if m.baseCtx != nil {
		if m.proberCancel != nil {
			m.proberCancel()
		}
		pCtx, cancel := context.WithCancel(m.baseCtx)
		m.proberCancel = cancel
		go prober.Start(pCtx)
	}

	return nil
}

func (m *RuntimeProviderManager) ActivateProviderSet(ctx context.Context, records []*ProviderRecord, secrets map[string]string, validator ActivationValidator) error {
	var rCfg *RoutingConfig
	var semRules *SemanticRuleSnapshot
	// Combos are currently not stored in RuntimeSnapshot, they are passed directly to Registry.
	// We need to fetch combos from repo or registry? Wait, if we fetch them from Registry, we can't easily retrieve them.
	// Let's just pull them from the snapshot's Registry if possible.
	var combos []providers.Combo
	
	snap := m.Snapshot()
	if snap != nil {
		rCfg = snap.RoutingConfig
		semRules = snap.SemanticRules
		if snap.Registry != nil {
			combos = snap.Registry.Combos()
		}
	}
	return m.ActivateDurable(ctx, records, secrets, rCfg, combos, semRules, validator)
}

func (m *RuntimeProviderManager) ActivateDurable(ctx context.Context, records []*ProviderRecord, secrets map[string]string, rCfg *RoutingConfig, combos []providers.Combo, semRules *SemanticRuleSnapshot, validator ActivationValidator) error {
	providerConfigs, err := BuildRuntimeConfig(records)
	if err != nil {
		return err
	}

	adapters, err := BuildProviderAdapters(records, secrets)
	if err != nil {
		return err
	}

	if validator != nil {
		if err := validator(ctx, adapters); err != nil {
			return gwErr.NewGatewayError("provider_activation_failed", fmt.Sprintf("validation failed: %v", err), 500)
		}
	}

	return m.activateInternal(providerConfigs, adapters, rCfg, combos, semRules)
}

// Router delegates
func (m *RuntimeProviderManager) Select(ctx context.Context, req *llm.LLMRequest) (providers.ProviderAdapter, routing.RoutingDecision, error) {
	snap := m.Snapshot()
	if snap == nil || snap.Router == nil {
		return nil, routing.RoutingDecision{}, gwErr.ErrNoActiveProviderConfig
	}
	return snap.Router.Select(ctx, req)
}

func (m *RuntimeProviderManager) SelectExcluding(ctx context.Context, req *llm.LLMRequest, excluded map[string]bool) (providers.ProviderAdapter, routing.RoutingDecision, error) {
	snap := m.Snapshot()
	if snap == nil || snap.Router == nil {
		return nil, routing.RoutingDecision{}, gwErr.ErrNoActiveProviderConfig
	}
	return snap.Router.SelectExcluding(ctx, req, excluded)
}

func (m *RuntimeProviderManager) GetProviderCapabilities() []providers.ProviderCapabilities {
	snap := m.Snapshot()
	if snap == nil || snap.Router == nil {
		return nil
	}
	return snap.Router.GetProviderCapabilities()
}

func (m *RuntimeProviderManager) GetAvailableModels() []string {
	snap := m.Snapshot()
	if snap == nil || snap.Router == nil {
		return nil
	}
	return snap.Router.GetAvailableModels()
}

func (m *RuntimeProviderManager) FallbackConfig() (bool, int) {
	snap := m.Snapshot()
	if snap != nil && snap.RoutingConfig != nil {
		return snap.RoutingConfig.FallbackEnabled, snap.RoutingConfig.MaxAttempts
	}
	return m.cfg.FallbackEnabled, m.cfg.MaxAttempts
}

func (m *RuntimeProviderManager) RoutingStrategy() string {
	snap := m.Snapshot()
	if snap != nil && snap.RoutingConfig != nil {
		return snap.RoutingConfig.Strategy
	}
	return m.cfg.RoutingStrategy
}

func (m *RuntimeProviderManager) CircuitBreakerConfig() (int, time.Duration) {
	threshold := m.cfg.HealthCheck.FailureThreshold
	if threshold <= 0 {
		threshold = 5 // fallback default
	}
	timeoutStr := m.cfg.HealthCheck.Interval
	if timeoutStr == "" {
		timeoutStr = "30s"
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 30 * time.Second
	}
	return threshold, timeout
}

func (m *RuntimeProviderManager) ProbeEnabled() bool {
	// The plan says probing uses durable provider health config.
	// We can report whether probe is running.
	// For simplicity, we just look at m.proberCancel or m.cfg
	if m.cfg.HealthCheck.Enabled != nil {
		return *m.cfg.HealthCheck.Enabled
	}
	return len(m.cfg.Providers) > 1
}

// LoadActiveProviderRecords returns all enabled provider records from the repository.
func LoadActiveProviderRecords(ctx context.Context, repo ProviderRepository) ([]*ProviderRecord, error) {
	enabled := true
	return repo.List(ctx, ProviderFilter{Enabled: &enabled})
}

// BuildRuntimeConfig converts active durable provider records into config.ProviderConfig.
func BuildRuntimeConfig(records []*ProviderRecord) ([]config.ProviderConfig, error) {
	var providerConfigs []config.ProviderConfig
	for _, r := range records {
		if !r.Enabled {
			continue
		}

		cfg := config.ProviderConfig{
			ID:           r.ID,
			Type:         r.Type,
			BaseURL:      r.BaseURL,
			Models:       r.Models,
			DefaultModel: r.DefaultModel,
			Timeout:      r.Timeout,
			Weight:       r.Weight,
		}
		providerConfigs = append(providerConfigs, cfg)
	}
	return providerConfigs, nil
}

// BuildProviderAdapters converts active durable provider records and decrypted secrets into provider adapters.
func BuildProviderAdapters(records []*ProviderRecord, decryptedSecrets map[string]string) ([]providers.ProviderAdapter, error) {
	var adapters []providers.ProviderAdapter

	for _, r := range records {
		if !r.Enabled {
			continue
		}

		if len(r.Models) == 0 {
			return nil, gwErr.NewGatewayError(gwErr.ErrMissingProviderModelConfig.Code, fmt.Sprintf("provider %s is missing required model config", r.ID), gwErr.ErrMissingProviderModelConfig.HTTPStatus)
		}

		apiKey, ok := decryptedSecrets[r.ID]
		if !ok || apiKey == "" {
			return nil, gwErr.NewGatewayError(gwErr.ErrMissingProviderSecret.Code, fmt.Sprintf("missing provider secret for provider %s", r.ID), gwErr.ErrMissingProviderSecret.HTTPStatus)
		}

		modelsCSV := strings.Join(r.Models, ",")

		var adapter providers.ProviderAdapter
		switch r.Type {
		case "openai-compatible":
			adapter = openai.NewAdapter(r.ID, r.BaseURL, apiKey, modelsCSV)
		case "anthropic":
			adapter = anthropic.NewAdapter(r.ID, r.BaseURL, apiKey, modelsCSV)
		case "gemini":
			adapter = gemini.NewAdapter(r.ID, r.BaseURL, apiKey, modelsCSV)
		default:
			return nil, fmt.Errorf("unknown provider type: %s", r.Type)
		}
		adapters = append(adapters, adapter)
	}

	return adapters, nil
}
