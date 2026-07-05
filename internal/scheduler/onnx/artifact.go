package onnx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const SupportedFeatureSchemaVersion = "scheduler-training-v1"

const (
	AnomalyStatusAvailable       = "available"
	AnomalyStatusUnavailable     = "unavailable"
	AnomalyStatusDegraded        = "degraded"
	AnomalyReasonOK              = "ok"
	AnomalyReasonMissingMetadata = "missing_metadata"
	AnomalyReasonInvalidMetadata = "invalid_metadata"
)

type Manifest struct {
	SchedulerVersion  string                                 `json:"scheduler_version"`
	ModelVersion      string                                 `json:"model_version"`
	Target            string                                 `json:"target"`
	FeatureSchema     string                                 `json:"feature_schema_version"`
	TrainingWindow    map[string]string                      `json:"training_window"`
	Metrics           map[string]float64                     `json:"metrics"`
	ONNXParity        Parity                                 `json:"onnx_parity"`
	Features          []string                               `json:"features"`
	SemanticFeatures  []string                               `json:"semantic_aggregate_features"`
	SemanticSupport   bool                                   `json:"semantic_aggregates_supported"`
	ModelSHA256       string                                 `json:"model_sha256"`
	ModelParameters   ModelParameters                        `json:"model_parameters"`
	AnomalyThresholds map[string]map[string]AnomalyThreshold `json:"anomaly_thresholds"`
	AnomalyEvidence   map[string]AnomalyEvidence             `json:"anomaly_evidence"`
}

type Parity struct {
	Passed      bool    `json:"passed"`
	MaxAbsError float64 `json:"max_abs_error"`
}

type ModelParameters struct {
	P70OutputTokens float64 `json:"p70_output_tokens"`
}

type AnomalyThreshold struct {
	Threshold   float64 `json:"threshold"`
	SampleCount int     `json:"sample_count"`
	Mean        float64 `json:"mean"`
	Stddev      float64 `json:"stddev"`
}

type AnomalyEvidence struct {
	Success              int `json:"success"`
	Failure              int `json:"failure"`
	Timeout              int `json:"timeout"`
	UnavailableThreshold int `json:"unavailable_threshold"`
}

type Artifact struct {
	Dir           string
	ModelPath     string
	Manifest      Manifest
	Runner        modelRunner
	AnomalyStatus string
	AnomalyReason string
	AnomalyErrors []string
}

func LoadArtifact(dir string) (*Artifact, error) {
	manifest, err := readManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	modelPath := filepath.Join(dir, "model.onnx")
	if err := validateArtifactModel(modelPath, manifest); err != nil {
		return nil, err
	}
	runner, err := loadModelRunner(modelPath)
	if err != nil {
		return nil, err
	}
	status, reason, errors := anomalyState(manifest)
	return &Artifact{Dir: dir, ModelPath: modelPath, Manifest: manifest, Runner: runner, AnomalyStatus: status, AnomalyReason: reason, AnomalyErrors: errors}, nil
}

func readManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read ONNX manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse ONNX manifest: %w", err)
	}
	if manifest.FeatureSchema != SupportedFeatureSchemaVersion {
		return Manifest{}, fmt.Errorf("unsupported ONNX feature schema: %s", manifest.FeatureSchema)
	}
	if manifest.Target != "p70_output_tokens" {
		return Manifest{}, fmt.Errorf("unsupported ONNX target: %s", manifest.Target)
	}
	if !manifest.ONNXParity.Passed {
		return Manifest{}, fmt.Errorf("ONNX parity check did not pass")
	}
	if err := validateSemanticFeatures(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) SupportsSemanticAggregates() bool {
	return m.SemanticSupport || len(m.SemanticFeatures) > 0
}

func anomalyState(manifest Manifest) (string, string, []string) {
	if len(manifest.AnomalyThresholds) == 0 {
		return AnomalyStatusUnavailable, AnomalyReasonMissingMetadata, nil
	}
	errs := validateAnomalyThresholds(manifest.AnomalyThresholds)
	if len(errs) > 0 {
		return AnomalyStatusDegraded, AnomalyReasonInvalidMetadata, errs
	}
	return AnomalyStatusAvailable, AnomalyReasonOK, nil
}

func validateAnomalyThresholds(thresholds map[string]map[string]AnomalyThreshold) []string {
	var errs []string
	for taskType, byCoverage := range thresholds {
		for coverage, threshold := range byCoverage {
			if !supportedCoverageLevel(coverage) {
				errs = append(errs, fmt.Sprintf("%s.%s unsupported coverage", taskType, coverage))
			}
			if threshold.Threshold <= 0 || threshold.SampleCount <= 0 || threshold.Mean < 0 || threshold.Stddev < 0 {
				errs = append(errs, fmt.Sprintf("%s.%s invalid threshold", taskType, coverage))
			}
		}
	}
	return errs
}

func supportedCoverageLevel(value string) bool {
	switch value {
	case "none", "fallback", "tenant", "all":
		return true
	default:
		return false
	}
}

func validateSemanticFeatures(manifest Manifest) error {
	for _, feature := range manifest.SemanticFeatures {
		if !supportedSemanticFeature(feature) {
			return fmt.Errorf("unsupported semantic aggregate feature: %s", feature)
		}
	}
	return nil
}

func supportedSemanticFeature(feature string) bool {
	for _, supported := range semanticAggregateFeatureNames {
		if feature == supported {
			return true
		}
	}
	return false
}

var semanticAggregateFeatureNames = []string{
	"neighbor_count",
	"latency_p50_ms",
	"latency_p90_ms",
	"latency_stddev_ms",
	"output_tokens_p70",
	"success_rate",
	"timeout_rate",
	"coverage_level",
	"coverage_ratio",
}

func validateArtifactModel(path string, manifest Manifest) error {
	sum, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("read ONNX model: %w", err)
	}
	if sum != manifest.ModelSHA256 {
		return fmt.Errorf("ONNX model checksum mismatch")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
