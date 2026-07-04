from __future__ import annotations

import json
from pathlib import Path

from .export import read_jsonl

TARGET = "p70_output_tokens"


def percentile(values: list[float], q: float) -> float:
    if not values:
        raise ValueError("cannot compute percentile for empty values")
    ordered = sorted(values)
    index = int(round((len(ordered) - 1) * q))
    return ordered[index]


def train_p70(rows: list[dict]) -> dict:
    targets = [float(row["output_tokens"]) for row in rows if row.get("outcome") == "success"]
    if not targets:
        raise ValueError("training requires at least one successful sample")
    estimate = percentile(targets, 0.70)
    return {
        "target": TARGET,
        "p70_output_tokens": estimate,
        "sample_count": len(targets),
        "feature_schema_version": "scheduler-training-v1",
    }


def train_file(input_path: Path, model_path: Path) -> dict:
    model = train_p70(read_jsonl(input_path))
    model_path.parent.mkdir(parents=True, exist_ok=True)
    model_path.write_text(json.dumps(model, indent=2, sort_keys=True), encoding="utf-8")
    return model
