package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"veloxmesh/internal/controlstate"
	gwErr "veloxmesh/internal/errors"
	"veloxmesh/internal/pipeline"
)

type AdminSemanticRulesHandler struct {
	svc *controlstate.AdminSemanticRulesService
}

func NewAdminSemanticRulesHandler(svc *controlstate.AdminSemanticRulesService) *AdminSemanticRulesHandler {
	return &AdminSemanticRulesHandler{svc: svc}
}

func (h *AdminSemanticRulesHandler) GetGlobalDefaults(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.GetGlobalDefaults(r.Context())
	if err != nil {
		sendAdminError(w, gwErr.NewGatewayError("internal_error", err.Error(), http.StatusInternalServerError))
		return
	}
	if cfg == nil {
		cfg = pipeline.DefaultSemanticPipelineConfig()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func (h *AdminSemanticRulesHandler) SaveGlobalDefaults(w http.ResponseWriter, r *http.Request) {
	var cfg pipeline.SemanticPipelineConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest))
		return
	}
	if err := h.svc.SaveGlobalDefaults(r.Context(), &cfg); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("internal_error", err.Error(), http.StatusInternalServerError))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminSemanticRulesHandler) GetUserConfig(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "userId path parameter is required", http.StatusBadRequest))
		return
	}

	cfg, err := h.svc.GetUserConfig(r.Context(), userID)
	if err != nil {
		sendAdminError(w, gwErr.NewGatewayError("internal_error", err.Error(), http.StatusInternalServerError))
		return
	}
	if cfg == nil {
		sendAdminError(w, gwErr.NewGatewayError("not_found", "user config not found", http.StatusNotFound))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func (h *AdminSemanticRulesHandler) SaveUserConfig(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "userId path parameter is required", http.StatusBadRequest))
		return
	}

	var cfg pipeline.SemanticPipelineConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest))
		return
	}

	if err := h.svc.SaveUserConfig(r.Context(), userID, &cfg); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("internal_error", err.Error(), http.StatusInternalServerError))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
