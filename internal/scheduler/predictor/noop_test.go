package predictor

import (
	"context"
	"errors"
	"testing"

	"veloxmesh/internal/scheduler"
)

func TestNoopPredictorReturnsPerTaskUnavailableErrors(t *testing.T) {
	predictor := NoopPredictor{ModelVersion: "noop"}
	predictions, err := predictor.Predict(context.Background(), []scheduler.TaskFeature{
		{TaskID: "a"},
		{TaskID: "b"},
	})
	if err != nil {
		t.Fatalf("Predict returned batch error: %v", err)
	}
	if len(predictions) != 2 {
		t.Fatalf("expected 2 predictions, got %d", len(predictions))
	}
	for _, prediction := range predictions {
		if !errors.Is(prediction.Err, ErrUnavailable) {
			t.Fatalf("expected per-task unavailable error, got %v", prediction.Err)
		}
	}
}
