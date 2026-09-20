import pytest

from scheduler_training.export import SEMANTIC_AGGREGATE_FIELDS, SAFE_FIELDS, sanitize_row


def safe_row():
    return {
        "task_id": "t1",
        "model_class": "standard",
        "estimated_input_tokens": 10,
        "estimated_output_tokens": 20,
        "output_tokens": 18,
        "outcome": "success",
    }


def test_export_keeps_safe_fields_only():
    row = safe_row() | {"extra": "ignored"}
    exported = sanitize_row(row)
    assert exported["task_id"] == "t1"
    assert "extra" not in exported


def test_export_default_fills_semantic_aggregates():
    exported = sanitize_row(safe_row() | {"coverage_level": None})
    assert all(field in SAFE_FIELDS for field in SEMANTIC_AGGREGATE_FIELDS)
    assert exported["neighbor_count"] == 0
    assert exported["latency_stddev_ms"] == 0.0
    assert exported["coverage_level"] == "none"
    assert exported["coverage_ratio"] == 0.0


def test_export_bounds_coverage_level():
    exported = sanitize_row(safe_row() | {"coverage_level": "tenant-id-123"})
    assert exported["coverage_level"] == "none"


def test_export_rejects_forbidden_field():
    row = safe_row() | {"pro" + "mpt": "do not store"}
    with pytest.raises(ValueError):
        sanitize_row(row)


def test_export_rejects_forbidden_field_variants():
    row = safe_row() | {"raw_" + "pay" + "load": "do not store"}
    with pytest.raises(ValueError):
        sanitize_row(row)
