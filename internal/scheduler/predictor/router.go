package predictor

import (
	"context"
	"hash/fnv"
	"strconv"

	"veloxmesh/internal/scheduler"
)

const (
	canaryBucketCount = 10000
	percentScale      = 100
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
	if r.Challenger != nil && assignedToChallenger(tasks, r.ChallengerPercent) {
		selected = r.Challenger
	}
	if selected == nil {
		selected = NoopPredictor{}
	}
	return selected.Predict(ctx, tasks)
}

func assignedToChallenger(tasks []scheduler.TaskFeature, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= percentScale {
		return true
	}
	if len(tasks) == 0 {
		return false
	}
	return canaryBucket(tasks[0]) < uint32(percent*percentScale)
}

func canaryBucket(task scheduler.TaskFeature) uint32 {
	key := task.TaskID
	if key == "" {
		key = strconv.FormatInt(task.EnqueueTimeMs, 10)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() % canaryBucketCount
}
