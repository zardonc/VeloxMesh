package controlstate

import (
	"context"
	"fmt"

	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/pipeline"
)

type AdminSemanticRulesService struct {
	repo           Repository
	hotStateClient hotstate.Client
}

func NewAdminSemanticRulesService(repo Repository, hotStateClient hotstate.Client) *AdminSemanticRulesService {
	return &AdminSemanticRulesService{
		repo:           repo,
		hotStateClient: hotStateClient,
	}
}

func (s *AdminSemanticRulesService) GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error) {
	if s.repo.SemanticRules() == nil {
		return nil, fmt.Errorf("semantic rules store not configured")
	}
	return s.repo.SemanticRules().GetGlobalDefaults(ctx)
}

func (s *AdminSemanticRulesService) GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error) {
	if s.repo.SemanticRules() == nil {
		return nil, fmt.Errorf("semantic rules store not configured")
	}
	return s.repo.SemanticRules().GetUserConfig(ctx, userID)
}

func (s *AdminSemanticRulesService) SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error {
	if s.repo.SemanticRules() == nil {
		return fmt.Errorf("semantic rules store not configured")
	}
	if err := s.repo.SemanticRules().SaveGlobalDefaults(ctx, cfg); err != nil {
		return err
	}
	// Publish change
	s.hotStateClient.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
		Type:       hotstate.EventSemanticRules,
		TargetID:   "global",
		ProviderID: "semantic_rules",
		Action:     "updated",
		Revision:   0,
	})
	return nil
}

func (s *AdminSemanticRulesService) SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error {
	if s.repo.SemanticRules() == nil {
		return fmt.Errorf("semantic rules store not configured")
	}
	if err := s.repo.SemanticRules().SaveUserConfig(ctx, userID, cfg); err != nil {
		return err
	}
	// Publish change
	s.hotStateClient.PublishConfigChange(ctx, &hotstate.ConfigChangeMessage{
		Type:       hotstate.EventSemanticRules,
		TargetID:   userID,
		ProviderID: "semantic_rules",
		Action:     "updated",
		Revision:   0,
	})
	return nil
}
