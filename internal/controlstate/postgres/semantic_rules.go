package postgres

import (
	"context"
	"errors"

	"veloxmesh/internal/pipeline"
)

var ErrUnsupportedRepository = errors.New("postgres repository capability unsupported")

type unsupportedSemanticRules struct{}

func (unsupportedSemanticRules) GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error) {
	return nil, ErrUnsupportedRepository
}

func (unsupportedSemanticRules) GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error) {
	return nil, ErrUnsupportedRepository
}

func (unsupportedSemanticRules) ListUserConfigs(ctx context.Context) (map[string]*pipeline.SemanticPipelineConfig, error) {
	return nil, ErrUnsupportedRepository
}

func (unsupportedSemanticRules) SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error {
	return ErrUnsupportedRepository
}

func (unsupportedSemanticRules) SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error {
	return ErrUnsupportedRepository
}
