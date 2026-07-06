package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

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

func (h *AdminSchedulerHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	limit, err := optionalPositiveInt(r, "limit")
	if err != nil {
		sendAdminError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.RuntimeStatus(r.Context(), limit))
}

func (h *AdminSchedulerHandler) GetSLARules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.service.SLARules())
}

func (h *AdminSchedulerHandler) PutSLARules(w http.ResponseWriter, r *http.Request) {
	var req scheduler.SLARulesReplaceRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest))
		return
	}
	resp, err := h.service.ReplaceSLARules(r.Context(), &req)
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

func optionalPositiveInt(r *http.Request, name string) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, gwErr.NewGatewayError("invalid_request", name+" must be a positive integer", http.StatusBadRequest)
	}
	return value, nil
}
