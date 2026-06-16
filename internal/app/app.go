package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/anthropic"
	"veloxmesh/internal/providers/gemini"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/routing"
)

type App struct {
	Config      *config.Config
	Logger      *slog.Logger
	Router      http.Handler
	Prober      *health.Prober
	healthStore health.Store
}

func (a *App) HealthStore() health.Store {
	return a.healthStore
}

func New() (*App, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	logger := observability.SetupLogger(cfg.LogLevel)

	var adapters []providers.ProviderAdapter
	for _, p := range cfg.Providers {
		switch p.Type {
		case "openai-compatible":
			adapter := openai.NewAdapter(
				p.ID,
				p.BaseURL,
				p.APIKey,
				strings.Join(p.Models, ","),
			)
			adapters = append(adapters, adapter)
		case "anthropic":
			adapter := anthropic.NewAdapter(
				p.ID,
				p.BaseURL,
				p.APIKey,
				strings.Join(p.Models, ","),
			)
			adapters = append(adapters, adapter)
		case "gemini":
			adapter := gemini.NewAdapter(
				p.ID,
				p.BaseURL,
				p.APIKey,
				strings.Join(p.Models, ","),
			)
			adapters = append(adapters, adapter)
		}
	}

	registry := providers.NewRegistry(cfg, adapters...)

	// Create health store
	healthStore := health.NewInMemoryStore()
	for _, p := range cfg.Providers {
		failureThreshold := cfg.HealthCheck.FailureThreshold
		successThreshold := cfg.HealthCheck.SuccessThreshold
		if p.HealthCheck != nil {
			if p.HealthCheck.FailureThreshold > 0 {
				failureThreshold = p.HealthCheck.FailureThreshold
			}
			if p.HealthCheck.SuccessThreshold > 0 {
				successThreshold = p.HealthCheck.SuccessThreshold
			}
		}
		healthStore.EnsureProvider(p.ID, failureThreshold, successThreshold)
	}

	prober := health.NewProber(registry, healthStore, cfg, logger)

	routingSvc := routing.NewHealthAwareRouter(registry, healthStore, cfg.RoutingStrategy)
	admissionCtrl := admission.NewPassThroughController()

	gatewaySvc := gateway.NewService(routingSvc, admissionCtrl, healthStore, cfg.FallbackEnabled, cfg.MaxAttempts)

	r := router.NewRouter(cfg, gatewaySvc)

	return &App{
		Config:      cfg,
		Logger:      logger,
		Router:      r,
		Prober:      prober,
		healthStore: healthStore,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("starting gateway", "addr", a.Config.GatewayDataAddr)

	go a.Prober.Start(ctx)

	errChan := make(chan error, 1)
	go func() {
		errChan <- http.ListenAndServe(a.Config.GatewayDataAddr, a.Router)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
