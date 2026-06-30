package controlstate

import (
	"context"

	"veloxmesh/internal/pipeline"
)

type SemanticRuleStore interface {
	GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error)
	GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error)
	SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error
	SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error
}
