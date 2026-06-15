package app

import (
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
	Config *config.Config
	Logger *slog.Logger
	Router http.Handler
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
	healthStore := health.NewInMemoryStore(3)
	for _, p := range cfg.Providers {
		healthStore.EnsureProvider(p.ID)
	}

	routingSvc := routing.NewHealthAwareRouter(registry, healthStore, cfg.RoutingStrategy)
	admissionCtrl := admission.NewPassThroughController()

	gatewaySvc := gateway.NewService(routingSvc, admissionCtrl, healthStore)

	r := router.NewRouter(cfg, gatewaySvc)

	return &App{
		Config: cfg,
		Logger: logger,
		Router: r,
	}, nil
}

func (a *App) Run() error {
	a.Logger.Info("starting gateway", "addr", a.Config.GatewayDataAddr)
	return http.ListenAndServe(a.Config.GatewayDataAddr, a.Router)
}
