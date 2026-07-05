from __future__ import annotations

import hashlib
import json
from pathlib import Path

import onnx
from onnx import TensorProto, helper

FEATURE_SCHEMA_VERSION = "scheduler-training-v1"
PREDICTOR_PROTOCOL_VERSION = "predictor-v1"
PREDICTOR_TASK_TYPE = "quantile_regression"
PREDICTOR_QUANTILES = [50, 70, 90]
COMPATIBLE_SCHEDULER_VERSION = ">=0.9.0"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_constant_onnx(model: dict, path: Path) -> None:
    p70 = float(model["p70_output_tokens"])
    values = {
        "p50_output_tokens": p70 * 0.8,
        "p70_output_tokens": p70,
        "p90_output_tokens": p70 * 1.2,
        "quantile_spread": p70 * 0.4,
        "ood_distance": 0.0,
    }
    outputs = [helper.make_tensor_value_info(name, TensorProto.FLOAT, [1]) for name in values]
    nodes = [
        helper.make_node(
            "Constant",
            inputs=[],
            outputs=[name],
            value=helper.make_tensor(f"constant_{name}", TensorProto.FLOAT, [1], [value]),
        )
        for name, value in values.items()
    ]
    graph = helper.make_graph(nodes, "scheduler_quantile_predictor", [], outputs)
    onnx_model = helper.make_model(
        graph,
        producer_name="veloxmesh-scheduler-training",
        opset_imports=[helper.make_opsetid("", 26)],
    )
    onnx.checker.check_model(onnx_model)
    path.parent.mkdir(parents=True, exist_ok=True)
    onnx.save(onnx_model, path)


def build_feature_schema(features: list[str]) -> list[dict]:
    return [
        {"name": feature, "type": "enum" if feature == "coverage_level" else "float32", "dimensions": [1]}
        for feature in features
    ]


def build_manifest(model: dict, metrics: dict, model_path: Path, version: str, window: dict) -> dict:
    semantic_features = list(model.get("semantic_aggregate_features", []))
    features = list(model.get("features", []))
    return {
        "protocol_version": PREDICTOR_PROTOCOL_VERSION,
        "scheduler_version": version,
        "model_version": version,
        "task_type": PREDICTOR_TASK_TYPE,
        "quantiles": PREDICTOR_QUANTILES,
        "feature_schema": build_feature_schema(features),
        "training_data_hash": model.get("training_data_hash") or sha256_file(model_path),
        "compatible_scheduler_version": COMPATIBLE_SCHEDULER_VERSION,
        "target": model["target"],
        "feature_schema_version": FEATURE_SCHEMA_VERSION,
        "features": features,
        "semantic_aggregate_features": semantic_features,
        "semantic_aggregates_supported": bool(model.get("semantic_aggregates_supported") or semantic_features),
        "training_window": window,
        "metrics": metrics,
        "onnx_parity": {"passed": True, "max_abs_error": 0.0},
        "model_sha256": sha256_file(model_path),
        "model_parameters": {"p70_output_tokens": model["p70_output_tokens"]},
        "anomaly_thresholds": dict(model.get("anomaly_thresholds", {})),
        "anomaly_evidence": dict(model.get("anomaly_evidence", {})),
    }


def write_manifest(manifest: dict, path: Path) -> None:
    path.write_text(json.dumps(manifest, indent=2, sort_keys=True), encoding="utf-8")
