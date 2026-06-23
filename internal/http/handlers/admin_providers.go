package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	key := controlstate.IdempotencyKeyFromRequest(r)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "failed to read body", http.StatusBadRequest))
		return
	}

	res, err := h.service.WithIdempotency(r.Context(), key, "provider.create", r.Method, r.URL.Path, body, func(ctx context.Context) (interface{}, error) {
		var req controlstate.ProviderCreateRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest)
		}
		return h.service.Create(ctx, &req)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
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

	key := controlstate.IdempotencyKeyFromRequest(r)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "failed to read body", http.StatusBadRequest))
		return
	}

	res, err := h.service.WithIdempotency(r.Context(), key, "provider.update", r.Method, r.URL.Path, body, func(ctx context.Context) (interface{}, error) {
		var req controlstate.ProviderUpdateRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest)
		}
		return h.service.Update(ctx, id, &req)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AdminProvidersHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id", http.StatusBadRequest))
		return
	}

	key := controlstate.IdempotencyKeyFromRequest(r)
	res, err := h.service.WithIdempotency(r.Context(), key, "provider.test_connection", r.Method, r.URL.Path, nil, func(ctx context.Context) (interface{}, error) {
		return h.service.TestConnection(ctx, id)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AdminProvidersHandler) Disable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id", http.StatusBadRequest))
		return
	}

	key := controlstate.IdempotencyKeyFromRequest(r)
	res, err := h.service.WithIdempotency(r.Context(), key, "provider.disable", r.Method, r.URL.Path, nil, func(ctx context.Context) (interface{}, error) {
		return nil, h.service.Disable(ctx, id)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		if idemRes.Status != http.StatusNoContent {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
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

	key := controlstate.IdempotencyKeyFromRequest(r)
	res, err := h.service.WithIdempotency(r.Context(), key, "provider.delete", r.Method, r.URL.Path, nil, func(ctx context.Context) (interface{}, error) {
		return nil, h.service.Delete(ctx, id)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		if idemRes.Status != http.StatusNoContent {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminProvidersHandler) SetRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	model := chi.URLParam(r, "model")
	if id == "" || model == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id or model", http.StatusBadRequest))
		return
	}

	key := controlstate.IdempotencyKeyFromRequest(r)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "failed to read body", http.StatusBadRequest))
		return
	}

	res, err := h.service.WithIdempotency(r.Context(), key, "rate.set", r.Method, r.URL.Path, body, func(ctx context.Context) (interface{}, error) {
		var req controlstate.RateRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, gwErr.NewGatewayError("invalid_request", "invalid JSON body", http.StatusBadRequest)
		}
		return h.service.SetRate(ctx, id, model, &req)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AdminProvidersHandler) GetRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	model := chi.URLParam(r, "model")
	if id == "" || model == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id or model", http.StatusBadRequest))
		return
	}

	res, err := h.service.GetRate(r.Context(), id, model)
	if err != nil {
		sendAdminError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AdminProvidersHandler) DeleteRate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	model := chi.URLParam(r, "model")
	if id == "" || model == "" {
		sendAdminError(w, gwErr.NewGatewayError("invalid_request", "missing id or model", http.StatusBadRequest))
		return
	}

	key := controlstate.IdempotencyKeyFromRequest(r)
	res, err := h.service.WithIdempotency(r.Context(), key, "rate.delete", r.Method, r.URL.Path, nil, func(ctx context.Context) (interface{}, error) {
		return nil, h.service.DeleteRate(ctx, id, model)
	})

	if err != nil {
		sendAdminError(w, err)
		return
	}

	if idemRes, ok := res.(*controlstate.IdempotencyResult); ok {
		if idemRes.Status != http.StatusNoContent {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(idemRes.Status)
		w.Write(idemRes.Response)
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
