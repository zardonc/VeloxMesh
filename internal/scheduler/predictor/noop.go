package predictor

import (
	"context"
	"errors"

	"veloxmesh/internal/scheduler"
)

var ErrUnavailable = errors.New("predictor unavailable")

type NoopPredictor struct {
	ModelVersion string
	Reason       error
}

func (p NoopPredictor) Predict(_ context.Context, tasks []scheduler.TaskFeature) ([]Prediction, error) {
	reason := p.Reason
	if reason == nil {
		reason = ErrUnavailable
	}
	predictions := make([]Prediction, len(tasks))
	for i := range tasks {
		predictions[i] = Prediction{
			ModelVersion: p.ModelVersion,
			Err:          reason,
		}
	}
	return predictions, nil
}
