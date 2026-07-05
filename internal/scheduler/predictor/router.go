package predictor

import (
	"context"

	"veloxmesh/internal/scheduler"
)

type Router struct {
	Champion          OutputTokenPredictor
	Challenger        OutputTokenPredictor
	ChallengerPercent int
	Shadow            OutputTokenPredictor
	RecordShadow      func([]Prediction, error)
}

func (r Router) Predict(ctx context.Context, tasks []scheduler.TaskFeature) ([]Prediction, error) {
	if r.Shadow != nil {
		predictions, err := r.Shadow.Predict(ctx, tasks)
		if r.RecordShadow != nil {
			r.RecordShadow(predictions, err)
		}
	}
	selected := r.Champion
	if r.Challenger != nil && r.ChallengerPercent >= 100 {
		selected = r.Challenger
	}
	if selected == nil {
		selected = NoopPredictor{}
	}
	return selected.Predict(ctx, tasks)
}
