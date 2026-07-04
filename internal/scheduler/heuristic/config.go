package heuristic

import "veloxmesh/internal/scheduler"

type Config struct {
	Version               string
	BaseLatencyMs         map[scheduler.RequestKind]int64
	ModelMultiplier       map[string]float64
	PriorityMultiplier    map[scheduler.PriorityClass]float64
	UncertaintyPenaltyK   float64
	ToolCallPenaltyMs     int64
	StreamDiscountPercent int64
}

func DefaultConfig() Config {
	return Config{
		Version: "heuristic-v1",
		BaseLatencyMs: map[scheduler.RequestKind]int64{
			scheduler.RequestKindSimpleQA:         800,
			scheduler.RequestKindCodeGen:          4000,
			scheduler.RequestKindCodeReview:       2500,
			scheduler.RequestKindSummarization:    1800,
			scheduler.RequestKindTranslation:      1200,
			scheduler.RequestKindStructuredOutput: 2200,
			scheduler.RequestKindMultiStep:        3500,
			scheduler.RequestKindToolCall:         3000,
			scheduler.RequestKindRAG:              2800,
			scheduler.RequestKindCreative:         3200,
		},
		ModelMultiplier: map[string]float64{
			"small":    0.7,
			"standard": 1,
			"large":    1.4,
		},
		PriorityMultiplier: map[scheduler.PriorityClass]float64{
			scheduler.PriorityHigh:   2,
			scheduler.PriorityNormal: 1,
			scheduler.PriorityLow:    0.5,
		},
		UncertaintyPenaltyK:   0.2,
		ToolCallPenaltyMs:     500,
		StreamDiscountPercent: 10,
	}
}
