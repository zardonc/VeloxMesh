package http

import (
	"github.com/go-chi/chi/v5"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/http/handlers"
	"veloxmesh/internal/http/middleware"
)

func NewRouter(cfg *config.Config, svc *gateway.Service, adminProvHandler *handlers.AdminProvidersHandler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recover)
	r.Use(middleware.Logging)

	chatHandler := handlers.NewChatHandler(svc)
	modelsHandler := handlers.NewModelsHandler(svc)

	// API routes that need auth will use a sub-router or group
	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(cfg))
		r.Post("/v1/chat/completions", chatHandler.ChatCompletions)
		r.Get("/v1/models", modelsHandler.ListModels)
	})

	if adminProvHandler != nil {
		r.Group(func(r chi.Router) {
			r.Use(middleware.AdminAuth(cfg))
			r.Get("/admin/v1/providers", adminProvHandler.List)
			r.Post("/admin/v1/providers", adminProvHandler.Create)
			r.Get("/admin/v1/providers/{id}", adminProvHandler.Get)
			r.Put("/admin/v1/providers/{id}", adminProvHandler.Update)
			r.Post("/admin/v1/providers/{id}/disable", adminProvHandler.Disable)
			r.Delete("/admin/v1/providers/{id}", adminProvHandler.Delete)
		})
	}

	r.Get("/healthz", handlers.Healthz)
	r.Get("/readyz", handlers.Readyz(cfg, svc))

	return r
}
