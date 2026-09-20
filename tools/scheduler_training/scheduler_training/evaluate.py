from __future__ import annotations

import json
from pathlib import Path

from .export import read_jsonl


def evaluate_model(model: dict, rows: list[dict]) -> dict:
    prediction = float(model["p70_output_tokens"])
    actuals = [float(row["output_tokens"]) for row in rows if row.get("outcome") == "success"]
    if not actuals:
        raise ValueError("evaluation requires at least one successful sample")
    errors = [abs(prediction - actual) for actual in actuals]
    return {
        "target": model["target"],
        "sample_count": len(actuals),
        "mae": sum(errors) / len(errors),
        "max_error": max(errors),
        "p70_output_tokens": prediction,
    }


def evaluate_file(model_path: Path, input_path: Path, metrics_path: Path) -> dict:
    model = json.loads(model_path.read_text(encoding="utf-8"))
    metrics = evaluate_model(model, read_jsonl(input_path))
    metrics_path.parent.mkdir(parents=True, exist_ok=True)
    metrics_path.write_text(json.dumps(metrics, indent=2, sort_keys=True), encoding="utf-8")
    return metrics
