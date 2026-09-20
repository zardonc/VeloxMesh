package predictor

import "testing"

func TestPredictionContractIsQuantileAware(t *testing.T) {
	prediction := Prediction{Quantiles: map[int]float64{50: 10, 70: 20, 90: 30}}
	if prediction.Quantiles[70] != 20 {
		t.Fatalf("expected P70 value to live under generic quantiles map")
	}
}
