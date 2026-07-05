from __future__ import annotations

import json
from pathlib import Path

from .export import SEMANTIC_AGGREGATE_FIELDS, read_jsonl

TARGET = "p70_output_tokens"
COVERAGE_LEVEL_ENCODING = {"none": 0.0, "fallback": 0.5, "tenant": 1.0}
BASE_FEATURE_FIELDS = ["estimated_input_tokens", "estimated_output_tokens"]
FEATURE_FIELDS = [*BASE_FEATURE_FIELDS, *SEMANTIC_AGGREGATE_FIELDS]


def percentile(values: list[float], q: float) -> float:
    if not values:
        raise ValueError("cannot compute percentile for empty values")
    ordered = sorted(values)
    index = int(round((len(ordered) - 1) * q))
    return ordered[index]


def prepare_features(rows: list[dict]) -> list[list[float]]:
    return [[encode_feature(row, field) for field in FEATURE_FIELDS] for row in rows]


def encode_feature(row: dict, field: str) -> float:
    if field == "coverage_level":
        return COVERAGE_LEVEL_ENCODING.get(str(row.get(field, "none")), 0.0)
    value = row.get(field, 0)
    if isinstance(value, bool):
        return 1.0 if value else 0.0
    return float(value or 0)


def train_p70(rows: list[dict]) -> dict:
    features = prepare_features(rows)
    targets = [float(row["output_tokens"]) for row in rows if row.get("outcome") == "success"]
    if not targets:
        raise ValueError("training requires at least one successful sample")
    estimate = percentile(targets, 0.70)
    return {
        "target": TARGET,
        "p70_output_tokens": estimate,
        "sample_count": len(targets),
        "feature_schema_version": "scheduler-training-v1",
        "feature_count": len(features[0]) if features else 0,
        "features": FEATURE_FIELDS,
        "semantic_aggregate_features": SEMANTIC_AGGREGATE_FIELDS,
        "semantic_aggregates_supported": True,
    }


def train_file(input_path: Path, model_path: Path) -> dict:
    model = train_p70(read_jsonl(input_path))
    model_path.parent.mkdir(parents=True, exist_ok=True)
    model_path.write_text(json.dumps(model, indent=2, sort_keys=True), encoding="utf-8")
    return model
