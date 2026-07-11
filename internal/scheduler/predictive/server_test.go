package predictive

import (
	"testing"

	"veloxmesh/internal/scheduler"
)

func TestScoreToProtoMapsMetadata(t *testing.T) {
	got := scoreToProto(scheduler.ScoreResult{
		TaskID:               "t1",
		ClassificationSource: "structured",
		AnomalyStatus:        scheduler.AnomalyStatusDegraded,
	})

	if got.GetClassificationSource() != "structured" || got.GetAnomalyStatus() != scheduler.AnomalyStatusDegraded {
		t.Fatalf("score metadata not mapped: %#v", got)
	}
}
