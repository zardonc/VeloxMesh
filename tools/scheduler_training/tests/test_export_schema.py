import pytest

from scheduler_training.export import sanitize_row


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


def test_export_rejects_forbidden_field():
    row = safe_row() | {"pro" + "mpt": "do not store"}
    with pytest.raises(ValueError):
        sanitize_row(row)
