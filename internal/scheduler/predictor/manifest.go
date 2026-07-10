package predictor

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

const ProtocolVersion = "predictor-v1"

type FeatureSpec struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Dimensions []int  `json:"dimensions,omitempty"`
}

type Manifest struct {
	ProtocolVersion            string        `json:"protocol_version"`
	ModelVersion               string        `json:"model_version"`
	TaskType                   string        `json:"task_type"`
	Quantiles                  []int         `json:"quantiles"`
	FeatureSchema              []FeatureSpec `json:"feature_schema"`
	TrainingDataHash           string        `json:"training_data_hash"`
	CompatibleSchedulerVersion string        `json:"compatible_scheduler_version"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read predictor manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse predictor manifest: %w", err)
	}
	return manifest, ValidateManifest(manifest)
}

func ValidateManifest(manifest Manifest) error {
	if manifest.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("unsupported predictor protocol: %s", manifest.ProtocolVersion)
	}
	if manifest.ModelVersion == "" || manifest.TrainingDataHash == "" {
		return fmt.Errorf("predictor manifest missing model_version or training_data_hash")
	}
	if !slices.Equal(manifest.Quantiles, []int{50, 70, 90}) {
		return fmt.Errorf("unsupported predictor quantiles: %v", manifest.Quantiles)
	}
	return ValidateFeatureSchema(manifest.FeatureSchema)
}

func ValidateFeatureSchema(schema []FeatureSpec) error {
	expected := SupportedFeatureSchema()
	if len(schema) != len(expected) {
		return fmt.Errorf("predictor feature schema length mismatch: got %d want %d", len(schema), len(expected))
	}
	for i, feature := range schema {
		if feature.Name != expected[i].Name || feature.Type != expected[i].Type {
			return fmt.Errorf("predictor feature %d mismatch: got %s/%s want %s/%s", i, feature.Name, feature.Type, expected[i].Name, expected[i].Type)
		}
		if !slices.Equal(feature.Dimensions, expected[i].Dimensions) {
			return fmt.Errorf("predictor feature %s dimensions mismatch: got %v want %v", feature.Name, feature.Dimensions, expected[i].Dimensions)
		}
	}
	return nil
}

func SupportedFeatureSchema() []FeatureSpec {
	return []FeatureSpec{
		floatFeature("estimated_input_tokens"),
		floatFeature("estimated_output_tokens"),
		floatFeature("neighbor_count"),
		floatFeature("latency_p50_ms"),
		floatFeature("latency_p90_ms"),
		floatFeature("latency_stddev_ms"),
		floatFeature("output_tokens_p70"),
		floatFeature("success_rate"),
		floatFeature("timeout_rate"),
		{Name: "coverage_level", Type: "enum", Dimensions: []int{1}},
		floatFeature("coverage_ratio"),
	}
}

func floatFeature(name string) FeatureSpec {
	return FeatureSpec{Name: name, Type: "float32", Dimensions: []int{1}}
}
