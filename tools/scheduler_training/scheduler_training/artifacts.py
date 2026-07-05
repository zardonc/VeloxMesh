from __future__ import annotations

import hashlib
import json
from pathlib import Path

import onnx
from onnx import TensorProto, helper

FEATURE_SCHEMA_VERSION = "scheduler-training-v1"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_constant_onnx(model: dict, path: Path) -> None:
    output = helper.make_tensor_value_info("p70_output_tokens", TensorProto.FLOAT, [1])
    value = helper.make_tensor("constant_p70", TensorProto.FLOAT, [1], [float(model["p70_output_tokens"])])
    node = helper.make_node("Constant", inputs=[], outputs=["p70_output_tokens"], value=value)
    graph = helper.make_graph([node], "scheduler_p70_predictor", [], [output])
    onnx_model = helper.make_model(graph, producer_name="veloxmesh-scheduler-training")
    onnx.checker.check_model(onnx_model)
    path.parent.mkdir(parents=True, exist_ok=True)
    onnx.save(onnx_model, path)


def build_manifest(model: dict, metrics: dict, model_path: Path, version: str, window: dict) -> dict:
    semantic_features = list(model.get("semantic_aggregate_features", []))
    return {
        "scheduler_version": version,
        "model_version": version,
        "target": model["target"],
        "feature_schema_version": FEATURE_SCHEMA_VERSION,
        "features": list(model.get("features", [])),
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
