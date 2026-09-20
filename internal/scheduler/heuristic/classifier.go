package heuristic

import "veloxmesh/internal/scheduler"

type Classification struct {
	Kind       scheduler.RequestKind
	Source     string
	Confidence float64
}

type Classifier struct{}

func (Classifier) Classify(feature scheduler.TaskFeature) Classification {
	if feature.RequestKind != "" {
		return Classification{Kind: feature.RequestKind, Source: "structured", Confidence: confidence(feature)}
	}
	if feature.HasToolCalls {
		return Classification{Kind: scheduler.RequestKindToolCall, Source: "rule", Confidence: 0.8}
	}
	if feature.CodeBlockCount > 0 {
		return Classification{Kind: scheduler.RequestKindCodeGen, Source: "rule", Confidence: 0.75}
	}
	return Classification{Kind: scheduler.RequestKindSimpleQA, Source: "fallback", Confidence: 0.6}
}

func confidence(feature scheduler.TaskFeature) float64 {
	if feature.ConfidenceHint > 0 {
		return feature.ConfidenceHint
	}
	return 1
}
