package http

import (
	"github.com/go-chi/chi/v5"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/http/handlers"
	"veloxmesh/internal/http/middleware"
)

func NewRouter(cfg *config.Config, svc *gateway.Service) *chi.Mux {
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

	r.Get("/healthz", handlers.Healthz)
	r.Get("/readyz", handlers.Readyz(cfg, svc))

	return r
}
