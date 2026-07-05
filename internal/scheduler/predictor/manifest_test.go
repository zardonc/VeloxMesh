package predictor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestAcceptsPredictorV1Schema(t *testing.T) {
	path := writeManifest(t, validManifest())
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if manifest.ModelVersion != "scheduler-predictor-v1" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestLoadManifestFailsFastOnSchemaDrift(t *testing.T) {
	manifest := validManifest()
	manifest.FeatureSchema[2].Type = "int64"
	_, err := LoadManifest(writeManifest(t, manifest))
	if err == nil || !strings.Contains(err.Error(), "feature 2 mismatch") {
		t.Fatalf("expected actionable feature drift error, got %v", err)
	}
}

func TestLoadManifestRejectsUnsupportedQuantiles(t *testing.T) {
	manifest := validManifest()
	manifest.Quantiles = []int{70}
	_, err := LoadManifest(writeManifest(t, manifest))
	if err == nil || !strings.Contains(err.Error(), "unsupported predictor quantiles") {
		t.Fatalf("expected quantile error, got %v", err)
	}
}

func validManifest() Manifest {
	return Manifest{
		ProtocolVersion:            ProtocolVersion,
		ModelVersion:               "scheduler-predictor-v1",
		TaskType:                   "quantile_regression",
		Quantiles:                  []int{50, 70, 90},
		FeatureSchema:              SupportedFeatureSchema(),
		TrainingDataHash:           strings.Repeat("a", 64),
		CompatibleSchedulerVersion: ">=0.9.0",
	}
}

func writeManifest(t *testing.T, manifest Manifest) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.json")
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}
