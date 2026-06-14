package handlers

import (
	"encoding/json"
	"net/http"
	"time"
	"veloxmesh/internal/gateway"
)

type ModelsHandler struct {
	service *gateway.Service
}

func NewModelsHandler(svc *gateway.Service) *ModelsHandler {
	return &ModelsHandler{service: svc}
}

type ModelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelItem `json:"data"`
}

func (h *ModelsHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	// For Phase 1, we can return the configured models from the registry.
	// Since we don't have a direct method to list all models across all providers yet,
	// we will construct a static list based on the router configuration.
	
	// A simple approach is to return the default provider's models.
	models := h.service.GetAvailableModels()

	var data []ModelItem
	for _, m := range models {
		data = append(data, ModelItem{
			ID:      m,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "veloxmesh",
		})
	}

	resp := ModelsResponse{
		Object: "list",
		Data:   data,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
