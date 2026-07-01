package controlstate

import (
	"context"
	"fmt"
	"strings"
	"time"

	gwErr "veloxmesh/internal/errors"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/providers"
)

type ComboResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Strategy  string    `json:"strategy"`
	Members   []string  `json:"members"`
	Judge     *string   `json:"judge,omitempty"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ComboListResponse struct {
	Data []*ComboResponse `json:"data"`
}

type ComboCreateRequest struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Enabled  *bool    `json:"enabled,omitempty"`
	Strategy string   `json:"strategy"`
	Members  []string `json:"members"`
	Judge    *string  `json:"judge,omitempty"`
}

type ComboUpdateRequest struct {
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Strategy string   `json:"strategy"`
	Members  []string `json:"members"`
	Judge    *string  `json:"judge,omitempty"`
	Revision int64    `json:"revision"`
}

type AdminComboService struct {
	repo      Repository
	manager   *RuntimeProviderManager
	cipher    SecretCipher
	publisher hotstate.ConfigChangePublisher
}

func NewAdminComboService(repo Repository, manager *RuntimeProviderManager, cipher SecretCipher, publisher hotstate.ConfigChangePublisher) *AdminComboService {
	return &AdminComboService{
		repo:      repo,
		manager:   manager,
		cipher:    cipher,
		publisher: publisher,
	}
}

// Helper to record audit
func (s *AdminComboService) RecordAudit(ctx context.Context, action string, targetID string, outcome string, meta map[string]interface{}) {
	// For now we duplicate a simple audit call, or we could require an AuditRepository directly
	auditRepo := s.repo.Audit()
	if auditRepo == nil {
		return
	}
	// simplistic metadata encoding
	var b []byte
	if meta != nil {
		b = []byte(fmt.Sprintf("%v", meta)) // ideally json
	}
	_ = auditRepo.Log(ctx, &AuditEvent{
		Actor:     "admin", // TODO: extract from context if auth is added
		Action:    action,
		TargetID:  targetID,
		Outcome:   outcome,
		Metadata:  b,
		Timestamp: time.Now().UTC(),
	})
}

func (s *AdminComboService) mapToResponse(r *ComboRecord) *ComboResponse {
	if r == nil {
		return nil
	}
	var judge *string
	if r.Judge != "" {
		judge = &r.Judge
	}
	return &ComboResponse{
		ID:        r.ID,
		Name:      r.Name,
		Enabled:   r.Enabled,
		Strategy:  r.Strategy,
		Members:   r.Members,
		Judge:     judge,
		Revision:  r.Revision,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

func (s *AdminComboService) Create(ctx context.Context, req *ComboCreateRequest) (res *ComboResponse, err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			if IsValidationError(err) {
				outcome = "validation_failed"
			} else {
				outcome = "combo_failed"
			}
		}
		s.RecordAudit(ctx, "combo.create", req.ID, outcome, meta)
	}()

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	mutation := &ComboMutation{
		ID:       req.ID,
		Name:     req.Name,
		Enabled:  enabled,
		Strategy: req.Strategy,
		Members:  req.Members,
		Judge:    req.Judge,
	}

	activeModels, err := s.getActiveModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load active models: %w", err)
	}

	fieldErrs := ValidateComboMutation(mutation, activeModels)
	if len(fieldErrs) > 0 {
		return nil, newValidationError(fieldErrs)
	}

	created, err := s.repo.Combos().Create(ctx, mutation)
	if err != nil {
		return nil, fmt.Errorf("failed to create combo: %w", err)
	}

	if err := s.reloadRuntime(ctx); err != nil {
		return nil, err
	}

	if s.publisher != nil {
		_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
			Type:       hotstate.EventCombo,
			TargetID:   created.ID,
			ProviderID: "combo-" + created.ID,
			Action:     "create",
			Revision:   created.Revision,
			Timestamp:  time.Now().UTC(),
		})
	}

	return s.mapToResponse(created), nil
}

func (s *AdminComboService) Update(ctx context.Context, id string, req *ComboUpdateRequest) (res *ComboResponse, err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			if IsValidationError(err) {
				outcome = "validation_failed"
			} else if strings.Contains(err.Error(), "optimistic concurrency conflict") {
				outcome = "conflict"
			} else {
				outcome = "combo_failed"
			}
		}
		s.RecordAudit(ctx, "combo.update", id, outcome, meta)
	}()

	mutation := &ComboMutation{
		ID:       id,
		Name:     req.Name,
		Enabled:  req.Enabled,
		Strategy: req.Strategy,
		Members:  req.Members,
		Judge:    req.Judge,
		Revision: &req.Revision,
	}

	activeModels, err := s.getActiveModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load active models: %w", err)
	}

	fieldErrs := ValidateComboMutation(mutation, activeModels)
	if len(fieldErrs) > 0 {
		return nil, newValidationError(fieldErrs)
	}

	updated, err := s.repo.Combos().Update(ctx, mutation)
	if err != nil {
		if strings.Contains(err.Error(), "optimistic concurrency conflict") {
			return nil, gwErr.NewGatewayError("combo_conflict", "optimistic concurrency conflict: record modified or not found", 409)
		}
		return nil, fmt.Errorf("failed to update combo: %w", err)
	}

	if err := s.reloadRuntime(ctx); err != nil {
		return nil, err
	}

	if s.publisher != nil {
		_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
			Type:       hotstate.EventCombo,
			TargetID:   updated.ID,
			ProviderID: "combo-" + updated.ID,
			Action:     "update",
			Revision:   updated.Revision,
			Timestamp:  time.Now().UTC(),
		})
	}

	return s.mapToResponse(updated), nil
}

func (s *AdminComboService) Get(ctx context.Context, id string) (*ComboResponse, error) {
	rec, err := s.repo.Combos().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, gwErr.NewGatewayError("combo_not_found", "combo not found", 404)
	}
	return s.mapToResponse(rec), nil
}

func (s *AdminComboService) List(ctx context.Context, filter ComboFilter) (*ComboListResponse, error) {
	records, err := s.repo.Combos().List(ctx, filter)
	if err != nil {
		return nil, err
	}
	var data []*ComboResponse
	for _, r := range records {
		data = append(data, s.mapToResponse(r))
	}
	if data == nil {
		data = []*ComboResponse{}
	}
	return &ComboListResponse{Data: data}, nil
}

func (s *AdminComboService) Delete(ctx context.Context, id string) (err error) {
	outcome := "success"
	var meta map[string]interface{}
	defer func() {
		if err != nil {
			if gwE, ok := err.(*gwErr.GatewayError); ok && gwE.Code == "combo_delete_not_safe" {
				outcome = "conflict"
			} else {
				outcome = "combo_failed"
			}
		}
		s.RecordAudit(ctx, "combo.delete", id, outcome, meta)
	}()

	rec, err := s.repo.Combos().Get(ctx, id)
	if err != nil {
		return err
	}
	if rec == nil {
		return gwErr.NewGatewayError("combo_not_found", "combo not found", 404)
	}

	if rec.Enabled {
		return gwErr.NewGatewayError("combo_delete_not_safe", "cannot delete an enabled combo; disable it first", 409)
	}

	if err := s.repo.Combos().Delete(ctx, id); err != nil {
		return err
	}

	_ = s.reloadRuntime(ctx)

	if s.publisher != nil {
		_ = s.publisher.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
			Type:       hotstate.EventCombo,
			TargetID:   id,
			ProviderID: "combo-" + id,
			Action:     "delete",
			Revision:   0,
			Timestamp:  time.Now().UTC(),
		})
	}

	return nil
}

func (s *AdminComboService) reloadRuntime(ctx context.Context) error {
	records, err := LoadActiveProviderRecords(ctx, s.repo.Providers())
	if err != nil {
		return fmt.Errorf("failed to load active provider records: %w", err)
	}

	var rCfg *RoutingConfig
	if s.repo.Routing() != nil {
		rCfg, err = s.repo.Routing().Get(ctx)
		if err != nil && err != ErrRoutingConfigNotFound {
			return fmt.Errorf("failed to load routing config: %w", err)
		}
	}

	secrets := make(map[string]string)
	for _, rec := range records {
		ciphertext, nonce, keyID, err := s.repo.Providers().GetEncryptedSecret(ctx, rec.ID)
		if err != nil {
			return fmt.Errorf("failed to get encrypted secret for %s: %w", rec.ID, err)
		}
		if len(ciphertext) == 0 {
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

	var combos []providers.Combo
	if s.repo.Combos() != nil {
		enabled := true
		comboRecords, err := s.repo.Combos().List(ctx, ComboFilter{Enabled: &enabled})
		if err != nil {
			return fmt.Errorf("failed to load combos: %w", err)
		}
		for _, rec := range comboRecords {
			combos = append(combos, providers.Combo{
				ID:       rec.ID,
				Name:     rec.Name,
				Strategy: rec.Strategy,
				Members:  rec.Members,
				Judge:    rec.Judge,
			})
		}
	}

	var semRules *SemanticRuleSnapshot
	snap := s.manager.Snapshot()
	if snap != nil {
		semRules = snap.SemanticRules
	}

	err = s.manager.ActivateDurable(ctx, records, secrets, rCfg, combos, semRules, nil, nil)
	if err != nil {
		return gwErr.NewGatewayError("activation_failed", fmt.Sprintf("activation failed: %v", err), 500)
	}

	return nil
}

func (s *AdminComboService) getActiveModels(ctx context.Context) ([]string, error) {
	// load providers and get their models
	providers, err := s.repo.Providers().List(ctx, ProviderFilter{Enabled: func() *bool { b := true; return &b }()})
	if err != nil {
		return nil, err
	}
	var activeModels []string
	for _, p := range providers {
		activeModels = append(activeModels, p.Models...)
	}
	return activeModels, nil
}
