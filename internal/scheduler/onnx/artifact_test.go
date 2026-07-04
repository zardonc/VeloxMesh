package onnx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadArtifactValidatesModelAndManifest(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "scheduler-training-v1")
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if artifact.Manifest.ModelParameters.P70OutputTokens != 42 {
		t.Fatalf("unexpected artifact: %#v", artifact)
	}
}

func TestLoadArtifactRejectsMissingModel(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "scheduler-training-v1")
	if err := os.Remove(filepath.Join(dir, "model.onnx")); err != nil {
		t.Fatalf("remove model: %v", err)
	}
	if _, err := LoadArtifact(dir); err == nil {
		t.Fatalf("expected missing model error")
	}
}

func TestLoadArtifactRejectsChecksumMismatch(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "scheduler-training-v1")
	if err := os.WriteFile(filepath.Join(dir, "model.onnx"), []byte("changed"), 0o600); err != nil {
		t.Fatalf("change model: %v", err)
	}
	if _, err := LoadArtifact(dir); err == nil {
		t.Fatalf("expected checksum mismatch")
	}
}

func TestLoadArtifactRejectsUnsupportedSchema(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "old-schema")
	if _, err := LoadArtifact(dir); err == nil {
		t.Fatalf("expected unsupported schema")
	}
}

func writeTestArtifact(t *testing.T, version string, p70 float64, schema string) string {
	t.Helper()
	dir := t.TempDir()
	model := []byte("onnx-test-model")
	if err := os.WriteFile(filepath.Join(dir, "model.onnx"), model, 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	sum := sha256.Sum256(model)
	manifest := Manifest{
		SchedulerVersion: version, ModelVersion: version, Target: "p70_output_tokens",
		FeatureSchema: schema, TrainingWindow: map[string]string{"start": "a", "end": "b"},
		Metrics: map[string]float64{"mae": 5}, ONNXParity: Parity{Passed: true},
		ModelSHA256: hex.EncodeToString(sum[:]), ModelParameters: ModelParameters{P70OutputTokens: p70},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}
