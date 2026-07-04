from __future__ import annotations

import json
from pathlib import Path

from .artifacts import build_manifest, write_constant_onnx, write_manifest


def publish_artifact(model_path: Path, metrics_path: Path, output_dir: Path, version: str, window: dict) -> Path:
    model = json.loads(model_path.read_text(encoding="utf-8"))
    metrics = json.loads(metrics_path.read_text(encoding="utf-8"))
    artifact_dir = output_dir / version
    artifact_dir.mkdir(parents=True, exist_ok=True)

    onnx_path = artifact_dir / "model.onnx"
    write_constant_onnx(model, onnx_path)
    manifest = build_manifest(model, metrics, onnx_path, version, window)
    write_manifest(manifest, artifact_dir / "manifest.json")
    return artifact_dir
