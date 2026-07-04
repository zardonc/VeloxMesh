import json

from scheduler_training.evaluate import evaluate_file
from scheduler_training.publish import publish_artifact
from scheduler_training.train import TARGET, train_file


def write_samples(path):
    rows = [
        {"task_id": "t1", "output_tokens": 10, "outcome": "success"},
        {"task_id": "t2", "output_tokens": 20, "outcome": "success"},
        {"task_id": "t3", "output_tokens": 30, "outcome": "success"},
    ]
    path.write_text("\n".join(json.dumps(row) for row in rows), encoding="utf-8")


def test_train_evaluate_and_publish_runtime_artifact(tmp_path):
    samples = tmp_path / "samples.jsonl"
    model = tmp_path / "build" / "model.json"
    metrics = tmp_path / "build" / "metrics.json"
    write_samples(samples)

    trained = train_file(samples, model)
    assert trained["target"] == TARGET
    assert trained["p70_output_tokens"] == 20

    evaluated = evaluate_file(model, samples, metrics)
    assert evaluated["sample_count"] == 3

    artifact = publish_artifact(model, metrics, tmp_path / "artifacts", "scheduler-p70-v1", {"start": "a", "end": "b"})
    assert (artifact / "model.onnx").exists()
    manifest = json.loads((artifact / "manifest.json").read_text(encoding="utf-8"))
    assert manifest["onnx_parity"]["passed"] is True
    assert manifest["model_sha256"]
    assert sorted(path.name for path in artifact.iterdir()) == ["manifest.json", "model.onnx"]
