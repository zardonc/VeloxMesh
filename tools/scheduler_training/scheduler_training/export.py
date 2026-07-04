from __future__ import annotations

import csv
import json
from pathlib import Path
from typing import Iterable

FORBIDDEN_FIELDS = {
    "prompt",
    "raw_prompt",
    "messages",
    "payload",
    "payload_hash",
    "authorization",
    "api_key",
    "secret",
}

SAFE_FIELDS = [
    "task_id",
    "model_class",
    "estimated_input_tokens",
    "estimated_output_tokens",
    "stream",
    "priority",
    "timeout_class",
    "enqueue_time_ms",
    "request_kind",
    "route_hint",
    "has_tool_calls",
    "tool_call_depth",
    "turn_count",
    "multimodal",
    "question_count",
    "code_block_count",
    "enumeration_hint",
    "instruction_verb_count",
    "max_sentence_length_bucket",
    "vocabulary_richness_bucket",
    "confidence_hint",
    "uncertainty_hint",
    "actual_latency_ms",
    "input_tokens",
    "output_tokens",
    "outcome",
    "provider_class",
    "scheduler_version",
    "completed_at",
]


def sanitize_row(row: dict) -> dict:
    forbidden = forbidden_fields(row)
    if forbidden:
        raise ValueError(f"forbidden scheduler export fields: {', '.join(sorted(forbidden))}")
    return {field: row.get(field) for field in SAFE_FIELDS}


def forbidden_fields(row: dict) -> set[str]:
    return {field for field in row if any(token in field.lower() for token in FORBIDDEN_FIELDS)}


def read_jsonl(path: Path) -> list[dict]:
    rows = []
    with path.open("r", encoding="utf-8") as handle:
        for line in handle:
            if line.strip():
                rows.append(sanitize_row(json.loads(line)))
    return rows


def write_csv(rows: Iterable[dict], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=SAFE_FIELDS)
        writer.writeheader()
        for row in rows:
            writer.writerow(sanitize_row(row))
