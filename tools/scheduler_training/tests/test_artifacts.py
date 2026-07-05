import json

from scheduler_training.artifacts import build_manifest, sha256_file, write_constant_onnx


def test_manifest_contains_runtime_contract(tmp_path):
    model_path = tmp_path / "model.onnx"
    model = {
        "target": "p70_output_tokens",
        "p70_output_tokens": 42,
        "features": ["estimated_input_tokens", "neighbor_count"],
        "semantic_aggregate_features": ["neighbor_count"],
        "semantic_aggregates_supported": True,
    }
    metrics = {"mae": 0, "sample_count": 1}
    write_constant_onnx(model, model_path)

    manifest = build_manifest(model, metrics, model_path, "v1", {"start": "s", "end": "e"})
    assert manifest["model_sha256"] == sha256_file(model_path)
    assert manifest["feature_schema_version"] == "scheduler-training-v1"
    assert manifest["semantic_aggregates_supported"] is True
    assert manifest["semantic_aggregate_features"] == ["neighbor_count"]
    assert manifest["training_window"]["start"] == "s"
    assert json.dumps(manifest)
