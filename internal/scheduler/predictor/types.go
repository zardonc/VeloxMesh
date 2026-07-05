package predictor

import (
	"context"

	"veloxmesh/internal/scheduler"
)

type Prediction struct {
	Quantiles    map[int]float64
	ModelVersion string
	Signals      map[string]float64
	Err          error
}

type OutputTokenPredictor interface {
	Predict(ctx context.Context, tasks []scheduler.TaskFeature) ([]Prediction, error)
}
