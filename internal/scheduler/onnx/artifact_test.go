package onnx

import (
	"crypto/sha256"
	"encoding/base64"
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

func TestLoadArtifactUsesONNXModelOutput(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 999, "scheduler-training-v1")
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if artifact.Runner.P70OutputTokens() != 42 {
		t.Fatalf("expected ONNX model prediction, got %f", artifact.Runner.P70OutputTokens())
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

func TestLoadArtifactRejectsUnsupportedONNXModel(t *testing.T) {
	dir := writeTestArtifactManifest(t, "scheduler-p70-v1", 42, "scheduler-training-v1", nil)
	model := []byte("not-a-supported-onnx-model")
	if err := os.WriteFile(filepath.Join(dir, "model.onnx"), model, 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	sum := sha256.Sum256(model)
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	manifest.ModelSHA256 = hex.EncodeToString(sum[:])
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, err := LoadArtifact(dir); err == nil {
		t.Fatalf("expected unsupported ONNX model error")
	}
}

func TestLoadArtifactRejectsUnsupportedSchema(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "old-schema")
	if _, err := LoadArtifact(dir); err == nil {
		t.Fatalf("expected unsupported schema")
	}
}

func TestLoadArtifactReadsSemanticAggregateSupport(t *testing.T) {
	dir := writeTestArtifactWithSemanticSupport(t, "scheduler-p70-v1", true)
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if !artifact.Manifest.SupportsSemanticAggregates() {
		t.Fatalf("expected semantic aggregate support")
	}
}

func TestLoadArtifactReadsAnomalyMetadata(t *testing.T) {
	dir := writeTestArtifactManifest(t, "scheduler-p70-v1", 42, "scheduler-training-v1", func(manifest *Manifest) {
		manifest.AnomalyThresholds = map[string]map[string]AnomalyThreshold{
			"simple_qa": {
				"tenant": {Threshold: 1.5, SampleCount: 20, Mean: 1.1, Stddev: 0.2},
			},
		}
		manifest.AnomalyEvidence = map[string]AnomalyEvidence{
			"simple_qa": {Success: 20, Failure: 1, Timeout: 2, UnavailableThreshold: 0},
		}
	})
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	threshold := artifact.Manifest.AnomalyThresholds["simple_qa"]["tenant"]
	if threshold.Threshold != 1.5 || threshold.SampleCount != 20 || threshold.Mean != 1.1 || threshold.Stddev != 0.2 {
		t.Fatalf("unexpected anomaly threshold: %#v", threshold)
	}
	if artifact.Manifest.AnomalyEvidence["simple_qa"].Timeout != 2 {
		t.Fatalf("unexpected anomaly evidence: %#v", artifact.Manifest.AnomalyEvidence)
	}
	if artifact.AnomalyStatus != AnomalyStatusAvailable || artifact.AnomalyReason != AnomalyReasonOK {
		t.Fatalf("unexpected anomaly state: %#v", artifact)
	}
}

func TestLoadArtifactAllowsMissingAnomalyMetadata(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "scheduler-training-v1")
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if artifact.AnomalyStatus != AnomalyStatusUnavailable || artifact.AnomalyReason != AnomalyReasonMissingMetadata {
		t.Fatalf("unexpected anomaly state: %#v", artifact)
	}
}

func TestLoadArtifactDegradesInvalidAnomalyMetadata(t *testing.T) {
	dir := writeTestArtifactManifest(t, "scheduler-p70-v1", 42, "scheduler-training-v1", func(manifest *Manifest) {
		manifest.AnomalyThresholds = map[string]map[string]AnomalyThreshold{
			"simple_qa": {"tenant": {Threshold: 0, SampleCount: 20, Mean: 1, Stddev: 0}},
		}
	})
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if artifact.AnomalyStatus != AnomalyStatusDegraded || artifact.AnomalyReason != AnomalyReasonInvalidMetadata {
		t.Fatalf("unexpected anomaly state: %#v", artifact)
	}
	if len(artifact.AnomalyErrors) == 0 {
		t.Fatalf("expected detailed validation errors for logs")
	}
}

func TestLoadArtifactDefaultsWithoutSemanticAggregateSupport(t *testing.T) {
	dir := writeTestArtifact(t, "scheduler-p70-v1", 42, "scheduler-training-v1")
	artifact, err := LoadArtifact(dir)
	if err != nil {
		t.Fatalf("LoadArtifact: %v", err)
	}
	if artifact.Manifest.SupportsSemanticAggregates() {
		t.Fatalf("expected legacy artifact without semantic aggregate support")
	}
}

func TestLoadArtifactRejectsUnknownSemanticFeature(t *testing.T) {
	dir := writeTestArtifactWithSemanticSupport(t, "scheduler-p70-v1", false)
	if _, err := LoadArtifact(dir); err == nil {
		t.Fatalf("expected unsupported semantic feature")
	}
}

func writeTestArtifact(t *testing.T, version string, p70 float64, schema string) string {
	return writeTestArtifactManifest(t, version, p70, schema, nil)
}

func writeTestArtifactWithSemanticSupport(t *testing.T, version string, valid bool) string {
	features := append([]string{}, semanticAggregateFeatureNames...)
	if !valid {
		features = []string{"tenant_id"}
	}
	return writeTestArtifactManifest(t, version, 42, "scheduler-training-v1", func(manifest *Manifest) {
		manifest.SemanticSupport = true
		manifest.SemanticFeatures = features
		manifest.Features = append([]string{"estimated_input_tokens"}, features...)
	})
}

func writeTestArtifactManifest(t *testing.T, version string, p70 float64, schema string, mutate func(*Manifest)) string {
	t.Helper()
	dir := t.TempDir()
	model := testConstantONNXModel(t)
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
	if mutate != nil {
		mutate(&manifest)
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

func testConstantONNXModel(t *testing.T) []byte {
	t.Helper()
	model, err := base64.StdEncoding.DecodeString("CA0SHHZlbG94bWVzaC1zY2hlZHVsZXItdHJhaW5pbmc6fwpDEhFwNzBfb3V0cHV0X3Rva2VucyIIQ29uc3RhbnQqJAoFdmFsdWUqGAgBEAEiBAAAKEJCDGNvbnN0YW50X3A3MKABBBIXc2NoZWR1bGVyX3A3MF9wcmVkaWN0b3JiHwoRcDcwX291dHB1dF90b2tlbnMSCgoICAESBAoCCAFCAhAb")
	if err != nil {
		t.Fatalf("decode test ONNX model: %v", err)
	}
	return model
}
