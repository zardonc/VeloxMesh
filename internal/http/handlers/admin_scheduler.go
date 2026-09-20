package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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

func (h *AdminSchedulerHandler) ExportTrainingSamples(w http.ResponseWriter, r *http.Request) {
	req, err := trainingExportRequest(r)
	if err != nil {
		sendAdminError(w, err)
		return
	}
	resp, err := h.service.ExportTrainingSamples(r.Context(), req)
	if err != nil {
		sendAdminError(w, err)
		return
	}
	if wantsNDJSON(r) {
		writeTrainingNDJSON(w, resp.Samples)
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

func trainingExportRequest(r *http.Request) (scheduler.TrainingExportRequest, error) {
	limit, err := optionalPositiveInt(r, "limit")
	if err != nil {
		return scheduler.TrainingExportRequest{}, err
	}
	start, err := optionalRFC3339(r, "start")
	if err != nil {
		return scheduler.TrainingExportRequest{}, err
	}
	end, err := optionalRFC3339(r, "end")
	if err != nil {
		return scheduler.TrainingExportRequest{}, err
	}
	return scheduler.TrainingExportRequest{Start: start, End: end, TaskType: r.URL.Query().Get("task_type"), Limit: limit}, nil
}

func optionalRFC3339(r *http.Request, name string) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, gwErr.NewGatewayError("invalid_request", name+" must be RFC3339", http.StatusBadRequest)
	}
	return value, nil
}

func wantsNDJSON(r *http.Request) bool {
	return r.URL.Query().Get("format") == "ndjson" || strings.Contains(r.Header.Get("Accept"), "application/x-ndjson")
}

func writeTrainingNDJSON(w http.ResponseWriter, samples []scheduler.TrainingExportSample) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	enc := json.NewEncoder(w)
	for _, sample := range samples {
		_ = enc.Encode(sample)
	}
}
