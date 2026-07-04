package handlers

import (
	"encoding/json"
	"net/http"

	gwErr "veloxmesh/internal/errors"
	"veloxmesh/internal/scheduler"
)

type AdminSchedulerHandler struct {
	service *scheduler.AdminSchedulerService
}

func NewAdminSchedulerHandler(service *scheduler.AdminSchedulerService) *AdminSchedulerHandler {
	return &AdminSchedulerHandler{service: service}
}

func (h *AdminSchedulerHandler) GetRollout(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.Status(r.Context())
	if err != nil {
		sendAdminError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *AdminSchedulerHandler) PatchRollout(w http.ResponseWriter, r *http.Request) {
	var req scheduler.RolloutPatchRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest))
		return
	}
	resp, err := h.service.Update(r.Context(), &req)
	if err != nil {
		sendAdminError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
