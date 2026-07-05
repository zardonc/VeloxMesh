from __future__ import annotations

import json
import hashlib
import math
from pathlib import Path

from .export import SEMANTIC_AGGREGATE_FIELDS, read_jsonl

TARGET = "p70_output_tokens"
COVERAGE_LEVEL_ENCODING = {"none": 0.0, "fallback": 0.5, "tenant": 1.0}
BASE_FEATURE_FIELDS = ["estimated_input_tokens", "estimated_output_tokens"]
FEATURE_FIELDS = [*BASE_FEATURE_FIELDS, *SEMANTIC_AGGREGATE_FIELDS]
ANOMALY_MIN_SAMPLES = 20
ANOMALY_STDDEV_K = 3.0
ANOMALY_PERCENTILE = 0.95


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


def anomaly_distance(row: dict) -> float:
    latency_p50 = max(float(row.get("latency_p50_ms") or 0), 1.0)
    latency_p90 = float(row.get("latency_p90_ms") or 0)
    latency_spread = max(latency_p90 - latency_p50, 0.0) / latency_p50
    timeout_rate = float(row.get("timeout_rate") or 0)
    success_gap = max(1.0 - float(row.get("success_rate") or 0), 0.0)
    coverage_gap = max(1.0 - float(row.get("coverage_ratio") or 0), 0.0)
    return latency_spread + timeout_rate + success_gap + coverage_gap


def compute_anomaly_thresholds(rows: list[dict]) -> tuple[dict, dict]:
    evidence = anomaly_evidence(rows)
    thresholds: dict[str, dict[str, dict]] = {}
    success_rows = [row for row in rows if row.get("outcome") == "success"]
    task_types = sorted({task_type(row) for row in success_rows})
    for current_task_type in task_types:
        task_rows = [row for row in success_rows if task_type(row) == current_task_type]
        task_threshold = threshold_entry([anomaly_distance(row) for row in task_rows])
        if task_threshold is None:
            evidence[current_task_type]["unavailable_threshold"] += 1
            continue
        levels = {"all": task_threshold}
        for level in sorted({coverage_level(row) for row in task_rows}):
            entry = threshold_entry([anomaly_distance(row) for row in task_rows if coverage_level(row) == level])
            if entry is not None:
                levels[level] = entry
        thresholds[current_task_type] = levels
    return thresholds, evidence


def anomaly_evidence(rows: list[dict]) -> dict:
    evidence: dict[str, dict[str, int]] = {}
    for row in rows:
        counts = evidence.setdefault(task_type(row), {"success": 0, "failure": 0, "timeout": 0, "unavailable_threshold": 0})
        outcome = str(row.get("outcome") or "")
        if outcome in ("success", "failure", "timeout"):
            counts[outcome] += 1
    return evidence


def threshold_entry(values: list[float]) -> dict | None:
    if len(values) < ANOMALY_MIN_SAMPLES:
        return None
    mean = sum(values) / len(values)
    stddev = math.sqrt(sum((value - mean) ** 2 for value in values) / len(values))
    mean_stddev_threshold = mean + (ANOMALY_STDDEV_K * stddev)
    percentile_threshold = percentile(values, ANOMALY_PERCENTILE)
    return {
        "threshold": max(mean_stddev_threshold, percentile_threshold),
        "sample_count": len(values),
        "mean": mean,
        "stddev": stddev,
    }


def task_type(row: dict) -> str:
    return str(row.get("request_kind") or "unknown")


def coverage_level(row: dict) -> str:
    return str(row.get("coverage_level") or "none")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def train_p70(rows: list[dict]) -> dict:
    features = prepare_features(rows)
    targets = [float(row["output_tokens"]) for row in rows if row.get("outcome") == "success"]
    if not targets:
        raise ValueError("training requires at least one successful sample")
    estimate = percentile(targets, 0.70)
    anomaly_thresholds, evidence = compute_anomaly_thresholds(rows)
    return {
        "target": TARGET,
        "p70_output_tokens": estimate,
        "sample_count": len(targets),
        "feature_schema_version": "scheduler-training-v1",
        "feature_count": len(features[0]) if features else 0,
        "features": FEATURE_FIELDS,
        "semantic_aggregate_features": SEMANTIC_AGGREGATE_FIELDS,
        "semantic_aggregates_supported": True,
        "anomaly_thresholds": anomaly_thresholds,
        "anomaly_evidence": evidence,
    }


def train_file(input_path: Path, model_path: Path) -> dict:
    model = train_p70(read_jsonl(input_path))
    model["training_data_hash"] = sha256_file(input_path)
    model_path.parent.mkdir(parents=True, exist_ok=True)
    model_path.write_text(json.dumps(model, indent=2, sort_keys=True), encoding="utf-8")
    return model
