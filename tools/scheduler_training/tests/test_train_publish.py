import json

from scheduler_training.evaluate import evaluate_file
from scheduler_training.export import SEMANTIC_AGGREGATE_FIELDS, read_jsonl
from scheduler_training.publish import publish_artifact
from scheduler_training.train import FEATURE_FIELDS, TARGET, prepare_features, train_file, train_p70


def write_samples(path):
    rows = [
        {"task_id": "t1", "output_tokens": 10, "outcome": "success", "coverage_level": "tenant"},
        {"task_id": "t2", "output_tokens": 20, "outcome": "success", "neighbor_count": 3},
        {"task_id": "t3", "output_tokens": 30, "outcome": "success"},
    ]
    path.write_text("\n".join(json.dumps(row) for row in rows), encoding="utf-8")


def anomaly_row(task_type="simple_qa", coverage_level="tenant", outcome="success", latency_p90_ms=200):
    return {
        "task_id": f"{task_type}-{coverage_level}-{outcome}-{latency_p90_ms}",
        "request_kind": task_type,
        "coverage_level": coverage_level,
        "outcome": outcome,
        "output_tokens": 10,
        "latency_p50_ms": 100,
        "latency_p90_ms": latency_p90_ms,
        "success_rate": 1.0,
        "timeout_rate": 0.0,
        "coverage_ratio": 1.0,
    }


def test_train_computes_anomaly_thresholds_from_successful_samples():
    rows = [anomaly_row(latency_p90_ms=200 + i) for i in range(20)]

    trained = train_p70(rows)
    threshold = trained["anomaly_thresholds"]["simple_qa"]["tenant"]

    assert threshold["sample_count"] == 20
    assert threshold["threshold"] >= threshold["mean"]
    assert threshold["mean"] > 0
    assert threshold["stddev"] >= 0


def test_train_keeps_failure_timeout_as_evidence_only():
    successes = [anomaly_row(latency_p90_ms=200 + i) for i in range(20)]
    noisy = successes + [
        anomaly_row(outcome="failure", latency_p90_ms=10000),
        anomaly_row(outcome="timeout", latency_p90_ms=10000),
    ]

    clean = train_p70(successes)
    trained = train_p70(noisy)

    assert trained["anomaly_thresholds"] == clean["anomaly_thresholds"]
    assert trained["anomaly_evidence"]["simple_qa"]["success"] == 20
    assert trained["anomaly_evidence"]["simple_qa"]["failure"] == 1
    assert trained["anomaly_evidence"]["simple_qa"]["timeout"] == 1


def test_train_omits_sparse_coverage_but_keeps_task_type_fallback():
    rows = [anomaly_row(coverage_level="tenant", latency_p90_ms=200 + i) for i in range(19)]
    rows.append(anomaly_row(coverage_level="fallback", latency_p90_ms=240))

    trained = train_p70(rows)

    assert "tenant" not in trained["anomaly_thresholds"]["simple_qa"]
    assert "fallback" not in trained["anomaly_thresholds"]["simple_qa"]
    assert trained["anomaly_thresholds"]["simple_qa"]["all"]["sample_count"] == 20


def test_train_marks_sparse_task_type_unavailable_without_threshold():
    trained = train_p70([anomaly_row(latency_p90_ms=200 + i) for i in range(19)])

    assert "simple_qa" not in trained["anomaly_thresholds"]
    assert trained["anomaly_evidence"]["simple_qa"]["unavailable_threshold"] == 1


def test_train_evaluate_and_publish_runtime_artifact(tmp_path):
    samples = tmp_path / "samples.jsonl"
    model = tmp_path / "build" / "model.json"
    metrics = tmp_path / "build" / "metrics.json"
    write_samples(samples)

    trained = train_file(samples, model)
    assert trained["target"] == TARGET
    assert trained["p70_output_tokens"] == 20
    assert trained["training_data_hash"]
    assert trained["semantic_aggregates_supported"] is True
    assert trained["semantic_aggregate_features"] == SEMANTIC_AGGREGATE_FIELDS
    assert trained["features"] == FEATURE_FIELDS

    prepared = prepare_features(read_jsonl(samples))
    assert len(prepared) == 3
    assert len(prepared[0]) == len(FEATURE_FIELDS)

    evaluated = evaluate_file(model, samples, metrics)
    assert evaluated["sample_count"] == 3

    artifact = publish_artifact(model, metrics, tmp_path / "artifacts", "scheduler-p70-v1", {"start": "a", "end": "b"})
    assert (artifact / "model.onnx").exists()
    manifest = json.loads((artifact / "manifest.json").read_text(encoding="utf-8"))
    assert manifest["protocol_version"] == "predictor-v1"
    assert manifest["task_type"] == "quantile_regression"
    assert manifest["quantiles"] == [50, 70, 90]
    assert manifest["training_data_hash"] == trained["training_data_hash"]
    assert manifest["compatible_scheduler_version"] == ">=0.9.0"
    assert manifest["feature_schema"][0] == {
        "name": "estimated_input_tokens",
        "type": "float32",
        "dimensions": [1],
    }
    assert manifest["feature_schema"][-2] == {"name": "coverage_level", "type": "enum", "dimensions": [1]}
    assert manifest["onnx_parity"]["passed"] is True
    assert manifest["model_sha256"]
    assert manifest["semantic_aggregates_supported"] is True
    assert manifest["semantic_aggregate_features"] == SEMANTIC_AGGREGATE_FIELDS
    assert "anomaly_thresholds" in manifest
    assert "anomaly_evidence" in manifest
    assert sorted(path.name for path in artifact.iterdir()) == ["manifest.json", "model.onnx"]


def test_publish_includes_anomaly_metadata_without_raw_exports(tmp_path):
    samples = tmp_path / "samples.jsonl"
    model = tmp_path / "build" / "model.json"
    metrics = tmp_path / "build" / "metrics.json"
    rows = [anomaly_row(latency_p90_ms=200 + i) for i in range(20)]
    samples.write_text("\n".join(json.dumps(row) for row in rows), encoding="utf-8")
    metrics.parent.mkdir(parents=True, exist_ok=True)
    metrics.write_text(json.dumps({"sample_count": 20}), encoding="utf-8")

    train_file(samples, model)
    artifact = publish_artifact(model, metrics, tmp_path / "artifacts", "scheduler-p70-v1", {"start": "a", "end": "b"})
    manifest = json.loads((artifact / "manifest.json").read_text(encoding="utf-8"))

    assert manifest["anomaly_thresholds"]["simple_qa"]["tenant"]["sample_count"] == 20
    assert manifest["anomaly_thresholds"]["simple_qa"]["all"]["threshold"] > 0
    assert manifest["anomaly_evidence"]["simple_qa"]["success"] == 20
    assert sorted(path.name for path in artifact.iterdir()) == ["manifest.json", "model.onnx"]
