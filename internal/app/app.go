package app

import (
	"log/slog"
	"net/http"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/routing"
)

type App struct {
	Config *config.Config
	Logger *slog.Logger
	Router http.Handler
}

func New() *App {
	cfg := config.LoadConfig()
	logger := observability.SetupLogger(cfg.LogLevel)
	
	openaiAdapter := openai.NewAdapter(
		cfg.DefaultProvider,
		cfg.PrimaryBaseURL,
		cfg.PrimaryAPIKey,
		cfg.PrimaryModels,
	)
	
	registry := providers.NewRegistry(cfg, openaiAdapter)
	routingSvc := routing.NewStaticRouter(registry)
	admissionCtrl := admission.NewPassThroughController()
	
	gatewaySvc := gateway.NewService(routingSvc, admissionCtrl)
	
	r := router.NewRouter(cfg, gatewaySvc)

	return &App{
		Config: cfg,
		Logger: logger,
		Router: r,
	}
}

func (a *App) Run() error {
	a.Logger.Info("starting gateway", "addr", a.Config.GatewayDataAddr)
	return http.ListenAndServe(a.Config.GatewayDataAddr, a.Router)
}
