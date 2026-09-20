package predictor

import (
	"context"
	"strconv"
	"testing"

	"veloxmesh/internal/scheduler"
)

func TestRouterRecordsShadowWithoutAdoptingIt(t *testing.T) {
	var shadow []Prediction
	router := Router{
		Champion:     staticPredictor{{Quantiles: map[int]float64{70: 20}}},
		Shadow:       staticPredictor{{Quantiles: map[int]float64{70: 999}}},
		RecordShadow: func(predictions []Prediction, _ error) { shadow = predictions },
	}
	got, err := router.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if got[0].Quantiles[70] != 20 || shadow[0].Quantiles[70] != 999 {
		t.Fatalf("expected champion adopted and shadow recorded: got=%#v shadow=%#v", got, shadow)
	}
}

func TestRouterCanaryUsesChallengerWhenFullyEnabled(t *testing.T) {
	router := Router{
		Champion:          staticPredictor{{Quantiles: map[int]float64{70: 20}}},
		Challenger:        staticPredictor{{Quantiles: map[int]float64{70: 30}}},
		ChallengerPercent: 100,
	}
	got, err := router.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if got[0].Quantiles[70] != 30 {
		t.Fatalf("expected challenger prediction, got %#v", got)
	}
}

func TestRouterCanarySplitsPartialTraffic(t *testing.T) {
	router := Router{
		Champion:          NoopPredictor{ModelVersion: "champion"},
		Challenger:        NoopPredictor{ModelVersion: "challenger"},
		ChallengerPercent: 50,
	}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		got, err := router.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "task-" + strconv.Itoa(i)}})
		if err != nil {
			t.Fatalf("Predict: %v", err)
		}
		seen[got[0].ModelVersion] = true
	}
	if !seen["champion"] || !seen["challenger"] {
		t.Fatalf("expected partial canary to use both predictors, saw %#v", seen)
	}
}

type staticPredictor []Prediction

func (p staticPredictor) Predict(context.Context, []scheduler.TaskFeature) ([]Prediction, error) {
	return []Prediction(p), nil
}
