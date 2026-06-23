package controlstate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	gwErr "veloxmesh/internal/errors"
	"veloxmesh/internal/hotstate"
)

// DTOs

type ProviderSecretResponse struct {
	Configured bool       `json:"configured"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type ProviderResponse struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	BaseURL      string                 `json:"base_url"`
	Enabled      bool                   `json:"enabled"`
	Models       []string               `json:"models,omitempty"`
	DefaultModel string                 `json:"default_model,omitempty"`
	Timeout      string                 `json:"timeout,omitempty"`
	Weight       int                    `json:"weight,omitempty"`
	Revision     int64                  `json:"revision"`
	Secret       ProviderSecretResponse `json:"secret"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type ProviderTestConnectionResponse struct {
	ProviderID   string    `json:"provider_id"`
	ProviderType string    `json:"provider_type"`
	OK           bool      `json:"ok"`
	Code         string    `json:"code"`
	Message      string    `json:"message"`
	LatencyMs    int64     `json:"latency_ms"`
	CheckedAt    time.Time `json:"checked_at"`
}

type ProviderListResponse struct {
	Data []*ProviderResponse `json:"data"`
}

type ProviderCreateRequest struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	BaseURL      string   `json:"base_url"`
	Enabled      *bool    `json:"enabled,omitempty"` // defaults to true
	APIKey       string   `json:"api_key"`           // required on create
	Models       []string `json:"models"`
	DefaultModel *string  `json:"default_model,omitempty"`
	Timeout      *string  `json:"timeout,omitempty"`
	Weight       *int     `json:"weight,omitempty"`
}

type RateRequest struct {
	InputCreditRate  int64 `json:"input_credit_rate"`
	OutputCreditRate int64 `json:"output_credit_rate"`
}

type ProviderUpdateRequest struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	BaseURL      string   `json:"base_url"`
	Enabled      bool     `json:"enabled"`
	APIKey       *string  `json:"api_key,omitempty"` // optional on update
	Models       []string `json:"models"`
	DefaultModel *string  `json:"default_model,omitempty"`
	Timeout      *string  `json:"timeout,omitempty"`
	Weight       *int     `json:"weight,omitempty"`
	Revision     int64    `json:"revision"` // required for optimistic concurrency
}

type AdminProviderService struct {
	repo      Repository
	cipher    SecretCipher
	manager   *RuntimeProviderManager
	publisher hotstate.ConfigChangePublisher
}

func NewAdminProviderService(repo Repository, cipher SecretCipher, manager *RuntimeProviderManager, publisher hotstate.ConfigChangePublisher) *AdminProviderService {
	return &AdminProviderService{
		repo:      repo,
		cipher:    cipher,
		manager:   manager,
		publisher: publisher,
	}
}

func (s *AdminProviderService) mapToResponse(r *ProviderRecord) *ProviderResponse {
	if r == nil {
		return nil
	}
	return &ProviderResponse{
		ID:           r.ID,
		Name:         r.Name,
		Type:         r.Type,
		BaseURL:      r.BaseURL,
		Enabled:      r.Enabled,
		Models:       r.Models,
		DefaultModel: r.DefaultModel,
		Timeout:      r.Timeout,
		Weight:       r.Weight,
		Revision:     r.Revision,
		Secret: ProviderSecretResponse{
			Configured: r.Secret.SecretConfigured,
			UpdatedAt:  r.Secret.UpdatedAt,
		},
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func (s *AdminProviderService) Create(ctx context.Context, req *ProviderCreateRequest) (res *ProviderResponse, err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			if IsValidationError(err) {
				outcome = "validation_failed"
			} else if gwE, ok := err.(*gwErr.GatewayError); ok && gwE.Code == "provider_conflict" {
				outcome = "conflict"
			} else if err.Error() == "activation failed" || strings.Contains(err.Error(), "activation failed") {
				outcome = "activation_failed"
			} else {
				outcome = "provider_failed"
			}
		}
		s.RecordAudit(ctx, "provider.create", req.ID, outcome, meta)
	}()

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	mutation := &ProviderMutation{
		ID:           req.ID,
		Name:         req.Name,
		Type:         req.Type,
		BaseURL:      req.BaseURL,
		Enabled:      enabled,
		APIKey:       &req.APIKey,
		Models:       req.Models,
		DefaultModel: req.DefaultModel,
		Timeout:      req.Timeout,
		Weight:       req.Weight,
	}

	fieldErrs := ValidateProviderMutation(mutation, true)
	if len(fieldErrs) > 0 {
		return nil, newValidationError(fieldErrs)
	}

	// Encrypt secret
	encSecret, err := s.cipher.EncryptProviderSecret([]byte(req.APIKey))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// We would use a transaction here, but since the interface doesn't let us pass it to the sub-repos natively without context wrapping
	// we will simulate the behavior by activating the runtime after success and reverting if it fails.
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	provRepo := s.repo.Providers()
	created, err := provRepo.Create(ctx, mutation)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	err = provRepo.PutEncryptedSecret(ctx, created.ID, encSecret.Ciphertext, encSecret.Nonce, encSecret.KeyID)
	if err != nil {
		return nil, fmt.Errorf("failed to save secret: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Trigger runtime activation
	if err := s.reloadRuntime(ctx); err != nil {
		// If runtime reload fails, we should ideally rollback the DB transaction.
		// However, since we committed already, the state is persisted.
		// For a full fix, we could do `reloadRuntime` before commit by reading uncommitted state.
		// For now we just return the activation error.
		return nil, err
	}

	if s.publisher != nil {
		_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
			ProviderID: created.ID,
			Action:     "create",
			Revision:   created.Revision,
			Timestamp:  time.Now().UTC(),
		})
	}

	return s.mapToResponse(created), nil
}

func (s *AdminProviderService) Update(ctx context.Context, id string, req *ProviderUpdateRequest) (res *ProviderResponse, err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			if IsValidationError(err) {
				outcome = "validation_failed"
			} else if gwE, ok := err.(*gwErr.GatewayError); ok && gwE.Code == "provider_conflict" {
				outcome = "conflict"
			} else if strings.Contains(err.Error(), "activation failed") {
				outcome = "activation_failed"
			} else {
				outcome = "provider_failed"
			}
		}
		s.RecordAudit(ctx, "provider.update", id, outcome, meta)
	}()

	mutation := &ProviderMutation{
		ID:           id,
		Name:         req.Name,
		Type:         req.Type,
		BaseURL:      req.BaseURL,
		Enabled:      req.Enabled,
		APIKey:       req.APIKey,
		Models:       req.Models,
		DefaultModel: req.DefaultModel,
		Timeout:      req.Timeout,
		Weight:       req.Weight,
		Revision:     &req.Revision,
	}

	fieldErrs := ValidateProviderMutation(mutation, false)
	if len(fieldErrs) > 0 {
		return nil, newValidationError(fieldErrs)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	provRepo := s.repo.Providers()
	updated, err := provRepo.Update(ctx, mutation)
	if err != nil {
		if strings.Contains(err.Error(), "optimistic concurrency conflict") {
			return nil, gwErr.NewGatewayError("provider_conflict", "optimistic concurrency conflict: record modified or not found", 409)
		}
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	if req.APIKey != nil && *req.APIKey != "" {
		encSecret, err := s.cipher.EncryptProviderSecret([]byte(*req.APIKey))
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt secret: %w", err)
		}
		err = provRepo.PutEncryptedSecret(ctx, updated.ID, encSecret.Ciphertext, encSecret.Nonce, encSecret.KeyID)
		if err != nil {
			return nil, fmt.Errorf("failed to update secret: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if err := s.reloadRuntime(ctx); err != nil {
		return nil, err
	}

	// Secret updated at might have changed, refetch
	updated, _ = provRepo.Get(ctx, id)

	if s.publisher != nil && updated != nil {
		_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
			ProviderID: updated.ID,
			Action:     "update",
			Revision:   updated.Revision,
			Timestamp:  time.Now().UTC(),
		})
	}

	return s.mapToResponse(updated), nil
}

func (s *AdminProviderService) TestConnection(ctx context.Context, id string) (res *ProviderTestConnectionResponse, err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			outcome = "provider_failed"
		} else if res != nil && !res.OK {
			outcome = res.Code
			meta = map[string]interface{}{"message": res.Message}
		}
		s.RecordAudit(ctx, "provider.test_connection", id, outcome, meta)
	}()

	start := time.Now()

	rec, err := s.repo.Providers().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, gwErr.NewGatewayError("provider_not_found", "provider not found", 404)
	}

	if !rec.Enabled {
		return &ProviderTestConnectionResponse{
			ProviderID:   rec.ID,
			ProviderType: rec.Type,
			OK:           false,
			Code:         "provider_disabled",
			Message:      "provider is disabled",
			LatencyMs:    time.Since(start).Milliseconds(),
			CheckedAt:    time.Now().UTC(),
		}, nil
	}

	ciphertext, nonce, keyID, err := s.repo.Providers().GetEncryptedSecret(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get encrypted secret: %w", err)
	}

	var apiKey string
	if len(ciphertext) > 0 {
		cleartext, err := s.cipher.DecryptProviderSecret(&EncryptedSecret{
			Ciphertext: ciphertext,
			Nonce:      nonce,
			KeyID:      keyID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret: %w", err)
		}
		apiKey = string(cleartext)
	}

	adapters, err := BuildProviderAdapters([]*ProviderRecord{rec}, map[string]string{rec.ID: apiKey})
	if err != nil {
		return &ProviderTestConnectionResponse{
			ProviderID:   rec.ID,
			ProviderType: rec.Type,
			OK:           false,
			Code:         "provider_configuration_invalid",
			Message:      err.Error(),
			LatencyMs:    time.Since(start).Milliseconds(),
			CheckedAt:    time.Now().UTC(),
		}, nil
	}

	adapter := adapters[0]
	hs := adapter.HealthCheck(ctx)

	code := "success"
	if !hs.Available {
		code = "provider_unavailable"
	}

	return &ProviderTestConnectionResponse{
		ProviderID:   rec.ID,
		ProviderType: rec.Type,
		OK:           hs.Available,
		Code:         code,
		Message:      hs.Message,
		LatencyMs:    time.Since(start).Milliseconds(),
		CheckedAt:    time.Now().UTC(),
	}, nil
}

func (s *AdminProviderService) Get(ctx context.Context, id string) (*ProviderResponse, error) {
	rec, err := s.repo.Providers().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, gwErr.NewGatewayError("provider_not_found", "provider not found", 404)
	}
	return s.mapToResponse(rec), nil
}

func (s *AdminProviderService) List(ctx context.Context, filter ProviderFilter) (*ProviderListResponse, error) {
	records, err := s.repo.Providers().List(ctx, filter)
	if err != nil {
		return nil, err
	}
	var data []*ProviderResponse
	for _, r := range records {
		data = append(data, s.mapToResponse(r))
	}
	if data == nil {
		data = []*ProviderResponse{}
	}
	return &ProviderListResponse{Data: data}, nil
}

func (s *AdminProviderService) Disable(ctx context.Context, id string) (err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			outcome = "provider_failed"
		}
		s.RecordAudit(ctx, "provider.disable", id, outcome, meta)
	}()

	rec, err := s.repo.Providers().Get(ctx, id)
	if err != nil {
		return err
	}
	if rec == nil {
		return gwErr.NewGatewayError("provider_not_found", "provider not found", 404)
	}

	if !rec.Enabled {
		return nil // already disabled
	}

	mutation := &ProviderMutation{
		ID:           rec.ID,
		Name:         rec.Name,
		Type:         rec.Type,
		BaseURL:      rec.BaseURL,
		Enabled:      false,
		Models:       rec.Models,
		DefaultModel: &rec.DefaultModel,
		Timeout:      &rec.Timeout,
		Weight:       &rec.Weight,
		Revision:     &rec.Revision,
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = s.repo.Providers().Update(ctx, mutation)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if err := s.reloadRuntime(ctx); err != nil {
		return err
	}

	if s.publisher != nil {
		// we need to get the updated revision
		if updatedRec, err := s.repo.Providers().Get(ctx, id); err == nil && updatedRec != nil {
			_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
				ProviderID: id,
				Action:     "disable",
				Revision:   updatedRec.Revision,
				Timestamp:  time.Now().UTC(),
			})
		}
	}

	return nil
}

func (s *AdminProviderService) Delete(ctx context.Context, id string) (err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			if gwE, ok := err.(*gwErr.GatewayError); ok && gwE.Code == "provider_delete_not_safe" {
				outcome = "conflict"
			} else {
				outcome = "provider_failed"
			}
		}
		s.RecordAudit(ctx, "provider.delete", id, outcome, meta)
	}()

	// First check if it exists
	rec, err := s.repo.Providers().Get(ctx, id)
	if err != nil {
		return err
	}
	if rec == nil {
		return gwErr.NewGatewayError("provider_not_found", "provider not found", 404)
	}

	if rec.Enabled {
		return gwErr.NewGatewayError("provider_delete_not_safe", "cannot delete an enabled provider; disable it first", 409)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := s.repo.Providers().Delete(ctx, id); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Just in case it was somehow active
	_ = s.reloadRuntime(ctx)

	if s.publisher != nil {
		_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
			ProviderID: id,
			Action:     "delete",
			Revision:   0, // Revision doesn't matter for delete
			Timestamp:  time.Now().UTC(),
		})
	}

	return nil
}

func (s *AdminProviderService) reloadRuntime(ctx context.Context) error {
	records, err := LoadActiveProviderRecords(ctx, s.repo.Providers())
	if err != nil {
		return fmt.Errorf("failed to load active provider records: %w", err)
	}

	secrets := make(map[string]string)
	for _, rec := range records {
		ciphertext, nonce, keyID, err := s.repo.Providers().GetEncryptedSecret(ctx, rec.ID)
		if err != nil {
			return fmt.Errorf("failed to get encrypted secret for %s: %w", rec.ID, err)
		}
		if len(ciphertext) == 0 {
			// skip missing secrets
			continue
		}
		cleartext, err := s.cipher.DecryptProviderSecret(&EncryptedSecret{
			Ciphertext: ciphertext,
			Nonce:      nonce,
			KeyID:      keyID,
		})
		if err != nil {
			return fmt.Errorf("failed to decrypt secret for %s: %w", rec.ID, err)
		}
		secrets[rec.ID] = string(cleartext)
	}

	err = s.manager.ActivateProviderSet(ctx, records, secrets, nil)
	if err != nil {
		// Log the error but maybe return it wrapped.
		return gwErr.NewGatewayError("provider_activation_failed", fmt.Sprintf("activation failed: %v", err), 500)
	}

	return nil
}

func (s *AdminProviderService) SetRate(ctx context.Context, providerID, model string, req *RateRequest) (*ProviderModelRate, error) {
	outcome := "success"
	var meta map[string]interface{}
	var err error
	defer func() {
		if err != nil {
			if IsValidationError(err) {
				outcome = "validation_failed"
			} else {
				outcome = "rate_failed"
			}
		}
		s.RecordAudit(ctx, "rate.set", providerID+":"+model, outcome, meta)
	}()

	if s.repo == nil || s.repo.Rates() == nil {
		err = gwErr.NewGatewayError("rate_management_unavailable", "rate management unavailable", 400)
		return nil, err
	}

	if providerID == "" || model == "" {
		err = newValidationError([]FieldError{{Field: "model", Code: "invalid", Message: "provider ID and model are required"}})
		return nil, err
	}

	if req.InputCreditRate < 0 || req.OutputCreditRate < 0 {
		err = newValidationError([]FieldError{{Field: "rate", Code: "invalid", Message: "rates cannot be negative"}})
		return nil, err
	}

	rec, getErr := s.repo.Providers().Get(ctx, providerID)
	if getErr != nil {
		err = getErr
		return nil, err
	}
	if rec == nil {
		err = newValidationError([]FieldError{{Field: "provider_id", Code: "invalid", Message: "provider not found"}})
		return nil, err
	}

	foundModel := false
	for _, m := range rec.Models {
		if m == model {
			foundModel = true
			break
		}
	}
	if !foundModel {
		err = newValidationError([]FieldError{{Field: "model", Code: "invalid", Message: "model does not belong to provider"}})
		return nil, err
	}

	rate := &ProviderModelRate{
		ProviderID:       providerID,
		Model:            model,
		InputCreditRate:  req.InputCreditRate,
		OutputCreditRate: req.OutputCreditRate,
	}

	err = s.repo.Rates().Save(ctx, rate)
	if err != nil {
		err = fmt.Errorf("failed to save rate: %w", err)
		return nil, err
	}

	return s.repo.Rates().Get(ctx, providerID, model)
}

func (s *AdminProviderService) GetRate(ctx context.Context, providerID, model string) (*ProviderModelRate, error) {
	if s.repo == nil || s.repo.Rates() == nil {
		return nil, gwErr.NewGatewayError("rate_management_unavailable", "rate management unavailable", 400)
	}

	rate, err := s.repo.Rates().Get(ctx, providerID, model)
	if err != nil {
		return nil, err
	}
	if rate == nil {
		return nil, gwErr.NewGatewayError("rate_not_found", "rate not found", 404)
	}

	return rate, nil
}

func (s *AdminProviderService) DeleteRate(ctx context.Context, providerID, model string) error {
	outcome := "success"
	var meta map[string]interface{}
	var err error
	defer func() {
		if err != nil && err.Error() != "rate management unavailable" {
			outcome = "rate_failed"
			s.RecordAudit(ctx, "rate.delete", providerID+":"+model, outcome, meta)
		}
	}()

	if s.repo == nil || s.repo.Rates() == nil {
		err = gwErr.NewGatewayError("rate_management_unavailable", "rate management unavailable", 400)
		return err
	}

	err = s.repo.Rates().Delete(ctx, providerID, model)
	if err != nil {
		err = fmt.Errorf("failed to delete rate: %w", err)
		return err
	}
	return nil
}

type ProviderFieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationErrorResponse struct {
	Code        string               `json:"code"`
	Message     string               `json:"message"`
	FieldErrors []ProviderFieldError `json:"field_errors"`
}

func (e *ValidationErrorResponse) Error() string {
	return e.Message
}

func newValidationError(errs []FieldError) *ValidationErrorResponse {
	var fieldErrs []ProviderFieldError
	for _, e := range errs {
		fieldErrs = append(fieldErrs, ProviderFieldError{
			Field:   e.Field,
			Code:    e.Code,
			Message: e.Message,
		})
	}
	return &ValidationErrorResponse{
		Code:        "validation_failed",
		Message:     "Validation failed",
		FieldErrors: fieldErrs,
	}
}

// IsValidationError checks if the error is a validation error type.
func IsValidationError(err error) bool {
	var valErr *ValidationErrorResponse
	return errors.As(err, &valErr)
}
