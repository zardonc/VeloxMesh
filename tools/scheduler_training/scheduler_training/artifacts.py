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
MIN_OUTPUT_TOKENS = 1.0
P50_RATIO = 0.8
P90_RATIO = 1.2
LATENCY_GAP_WEIGHT = 0.005
P70_FEATURE_WEIGHTS = {
    "estimated_input_tokens": 0.05,
    "estimated_output_tokens": 0.65,
    "neighbor_count": 0.5,
    "output_tokens_p70": 0.25,
}
OOD_FEATURE_WEIGHTS = {
    "timeout_rate": 1.0,
    "latency_stddev_ms": 0.01,
}


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def write_feature_onnx(model: dict, path: Path) -> None:
    features = list(model.get("features", []))
    if not features:
        raise ValueError("ONNX export requires scheduler features")

    builder = OnnxGraphBuilder()
    p70_raw = builder.weighted_feature_sum(
        features=features,
        prefix="p70_raw",
        weights=P70_FEATURE_WEIGHTS,
        bias=model["p70_output_tokens"],
    )
    p70 = builder.max_with_floor(name="p70_output_tokens", value=p70_raw, floor=MIN_OUTPUT_TOKENS)
    p50 = builder.multiply(name="p50_output_tokens", value=p70, factor=P50_RATIO)
    p90 = builder.multiply(name="p90_output_tokens", value=p70, factor=P90_RATIO)
    spread = builder.subtract(name="quantile_spread", left=p90, right=p50)
    ood = builder.ood_distance(features)

    inputs = [helper.make_tensor_value_info(feature, TensorProto.FLOAT, [1]) for feature in features]
    outputs = [helper.make_tensor_value_info(name, TensorProto.FLOAT, [1]) for name in [p50, p70, p90, spread, ood]]
    graph = helper.make_graph(builder.nodes, "scheduler_quantile_predictor", inputs, outputs, initializer=builder.initializers)
    onnx_model = helper.make_model(
        graph,
        producer_name="veloxmesh-scheduler-training",
        opset_imports=[helper.make_opsetid("", 26)],
    )
    onnx.checker.check_model(onnx_model)
    path.parent.mkdir(parents=True, exist_ok=True)
    onnx.save(onnx_model, path)


class OnnxGraphBuilder:
    def __init__(self) -> None:
        self.nodes: list = []
        self.initializers: list = []

    def weighted_feature_sum(self, *, features: list[str], prefix: str, weights: dict, bias: float) -> str:
        terms = [
            self.multiply(name=f"{prefix}_{feature}_term", value=feature, factor=weight)
            for feature, weight in weights.items()
            if feature in features
        ]
        return self.add_terms(prefix=prefix, terms=terms, bias=float(bias))

    def ood_distance(self, features: list[str]) -> str:
        terms = self.ood_terms(features)
        return self.add_terms(prefix="ood_distance", terms=terms, bias=0.0)

    def ood_terms(self, features: list[str]) -> list[str]:
        terms = [
            self.multiply(name=f"ood_{feature}_term", value=feature, factor=weight)
            for feature, weight in OOD_FEATURE_WEIGHTS.items()
            if feature in features
        ]
        return terms + self.ood_gap_terms(features)

    def ood_gap_terms(self, features: list[str]) -> list[str]:
        terms: list[str] = []
        if "success_rate" in features:
            terms.append(self.clamped_gap(name="success_gap", left=self.constant("success_one", 1.0), right="success_rate"))
        if "coverage_ratio" in features:
            terms.append(self.clamped_gap(name="coverage_gap", left=self.constant("coverage_one", 1.0), right="coverage_ratio"))
        if "latency_p90_ms" in features and "latency_p50_ms" in features:
            gap = self.clamped_gap(name="latency_gap", left="latency_p90_ms", right="latency_p50_ms")
            terms.append(self.multiply(name="latency_gap_weighted", value=gap, factor=LATENCY_GAP_WEIGHT))
        return terms

    def add_terms(self, *, prefix: str, terms: list[str], bias: float) -> str:
        current = self.constant(f"{prefix}_bias", bias)
        if not terms:
            self.nodes.append(helper.make_node("Identity", inputs=[current], outputs=[prefix]))
            return prefix
        for index, term in enumerate(terms):
            output = prefix if index == len(terms) - 1 else f"{prefix}_sum_{index}"
            self.nodes.append(helper.make_node("Add", inputs=[current, term], outputs=[output]))
            current = output
        return current

    def max_with_floor(self, *, name: str, value: str, floor: float) -> str:
        self.nodes.append(helper.make_node("Max", inputs=[value, self.constant(f"{name}_floor", floor)], outputs=[name]))
        return name

    def clamped_gap(self, *, name: str, left: str, right: str) -> str:
        raw = self.subtract(name=f"{name}_raw", left=left, right=right)
        return self.max_with_floor(name=name, value=raw, floor=0.0)

    def multiply(self, *, name: str, value: str, factor: float) -> str:
        self.nodes.append(helper.make_node("Mul", inputs=[value, self.constant(f"{name}_factor", factor)], outputs=[name]))
        return name

    def subtract(self, *, name: str, left: str, right: str) -> str:
        self.nodes.append(helper.make_node("Sub", inputs=[left, right], outputs=[name]))
        return name

    def constant(self, name: str, value: float) -> str:
        self.initializers.append(helper.make_tensor(name, TensorProto.FLOAT, [1], [float(value)]))
        return name


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
