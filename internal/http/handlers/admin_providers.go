package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"veloxmesh/internal/controlstate"
	gwErr "veloxmesh/internal/errors"
)

type AdminProvidersHandler struct {
	service *controlstate.AdminProviderService
}

func NewAdminProvidersHandler(service *controlstate.AdminProviderService) *AdminProvidersHandler {
	return &AdminProvidersHandler{service: service}
}

func (h *AdminProvidersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req controlstate.ProviderCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest))
		return
	}

	resp, err := h.service.Create(r.Context(), &req)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	var filter controlstate.ProviderFilter
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == "true"
		filter.Enabled = &enabled
	}
	if typeStr := r.URL.Query().Get("type"); typeStr != "" {
		filter.Type = typeStr
	}
	if searchStr := r.URL.Query().Get("search"); searchStr != "" {
		filter.Search = searchStr
	}

	resp, err := h.service.List(r.Context(), filter)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminProvidersHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id", http.StatusBadRequest))
		return
	}

	resp, err := h.service.Get(r.Context(), id)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminProvidersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id", http.StatusBadRequest))
		return
	}

	var req controlstate.ProviderUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest))
		return
	}

	resp, err := h.service.Update(r.Context(), id, &req)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminProvidersHandler) Disable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id", http.StatusBadRequest))
		return
	}

	err := h.service.Disable(r.Context(), id)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminProvidersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id", http.StatusBadRequest))
		return
	}

	err := h.service.Delete(r.Context(), id)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func sendAdminError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")

	var valErr *controlstate.ValidationErrorResponse
	if errors.As(err, &valErr) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(valErr)
		return
	}

	var gwError *gwErr.GatewayError
	if errors.As(err, &gwError) {
		w.WriteHeader(gwError.HTTPStatus)
		_ = json.NewEncoder(w).Encode(gwError)
		return
	}

	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":    "internal_error",
		"message": err.Error(),
	})
}
