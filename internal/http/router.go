package http

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/http/handlers"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/coordination"
)

func NewRouter(cfg *config.Config, svc *gateway.Service, adminProvHandler *handlers.AdminProvidersHandler, adminCombosHandler *handlers.AdminCombosHandler, adminSemanticRulesHandler *handlers.AdminSemanticRulesHandler, hotStateClient hotstate.Client, repo controlstate.Repository, coord coordination.Coordinator, lagReporter handlers.LagReporter) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recover)
	r.Use(middleware.Logging)

	chatHandler := handlers.NewChatHandler(svc)
	modelsHandler := handlers.NewModelsHandler(svc)

	// API routes that need auth will use a sub-router or group
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(cfg, hotStateClient, repo))
		r.Post("/v1/chat/completions", chatHandler.ChatCompletions)
		r.Get("/v1/models", modelsHandler.ListModels)
	})

	if adminProvHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(middleware.AdminAuth(cfg))
			r.Use(middleware.RequireWritable(coord))
			r.Get("/admin/v1/providers", adminProvHandler.List)
			r.Post("/admin/v1/providers", adminProvHandler.Create)
			r.Get("/admin/v1/providers/{id}", adminProvHandler.Get)
			r.Put("/admin/v1/providers/{id}", adminProvHandler.Update)
			r.Post("/admin/v1/providers/{id}/disable", adminProvHandler.Disable)
			r.Post("/admin/v1/providers/{id}/test-connection", adminProvHandler.TestConnection)
			r.Delete("/admin/v1/providers/{id}", adminProvHandler.Delete)
			r.Put("/admin/v1/providers/{id}/models/{model}/rate", adminProvHandler.SetRate)
			r.Get("/admin/v1/providers/{id}/models/{model}/rate", adminProvHandler.GetRate)
			r.Delete("/admin/v1/providers/{id}/models/{model}/rate", adminProvHandler.DeleteRate)
		})
	}

	if adminCombosHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(middleware.AdminAuth(cfg))
			r.Use(middleware.RequireWritable(coord))
			r.Get("/admin/v1/combos", adminCombosHandler.List)
			r.Post("/admin/v1/combos", adminCombosHandler.Create)
			r.Get("/admin/v1/combos/{id}", adminCombosHandler.Get)
			r.Put("/admin/v1/combos/{id}", adminCombosHandler.Update)
			r.Delete("/admin/v1/combos/{id}", adminCombosHandler.Delete)
		})
	}

	if adminSemanticRulesHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(middleware.AdminAuth(cfg))
			r.Use(middleware.RequireWritable(coord))
			r.Get("/admin/v1/semantic-rules", adminSemanticRulesHandler.GetGlobalDefaults)
			r.Put("/admin/v1/semantic-rules", adminSemanticRulesHandler.SaveGlobalDefaults)
			r.Get("/admin/v1/semantic-rules/users/{userId}", adminSemanticRulesHandler.GetUserConfig)
			r.Put("/admin/v1/semantic-rules/users/{userId}", adminSemanticRulesHandler.SaveUserConfig)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(middleware.AdminAuth(cfg))
		r.Get("/admin/v1/topology", handlers.Topology(coord, lagReporter))
	})

	r.Get("/healthz", handlers.Healthz)
	r.Get("/readyz", handlers.Readyz(cfg, svc, coord, lagReporter))
	r.Handle("/metrics", promhttp.Handler())

	return r
}
