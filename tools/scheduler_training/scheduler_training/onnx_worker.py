from __future__ import annotations

import argparse
import json
import sys
from concurrent import futures
from pathlib import Path

import grpc
import numpy as np
import onnxruntime as ort

_BINDINGS_DIR = Path(__file__).with_name("predictorv1")
if str(_BINDINGS_DIR) not in sys.path:
    sys.path.insert(0, str(_BINDINGS_DIR))

import predictor_pb2  # noqa: E402
import predictor_pb2_grpc  # noqa: E402

COVERAGE_LEVEL_ENCODING = {"none": 0.0, "fallback": 0.5, "tenant": 1.0, "all": 1.0}


class ONNXWorker(predictor_pb2_grpc.OutputTokenPredictorServicer):
    def __init__(self, artifact_dir: Path):
        self.artifact_dir = artifact_dir
        self.manifest = json.loads((artifact_dir / "manifest.json").read_text(encoding="utf-8"))
        self.session = ort.InferenceSession(str(artifact_dir / "model.onnx"), providers=["CPUExecutionProvider"])

    def Health(self, request, context):
        return predictor_pb2.HealthResponse(ready=True, model_version=self.manifest.get("model_version", ""))

    def BatchPredict(self, request, context):
        return predictor_pb2.BatchPredictResponse(predictions=[self._prediction(task) for task in request.tasks])

    def _run_model(self, task) -> dict[str, float]:
        names = [output.name for output in self.session.get_outputs()]
        feeds = {input.name: tensor(feature_value(task, input.name)) for input in self.session.get_inputs()}
        values = self.session.run(names, feeds)
        return {name: scalar(value) for name, value in zip(names, values)}

    def _prediction(self, task):
        if task.estimated_input_tokens < 0 or task.estimated_output_tokens < 0:
            return predictor_pb2.Prediction(model_version=self.manifest.get("model_version", ""), error="invalid_task")
        outputs = self._run_model(task)
        quantiles = {50: outputs.get("p50_output_tokens", 0), 70: outputs.get("p70_output_tokens", 0), 90: outputs.get("p90_output_tokens", 0)}
        spread = outputs.get("quantile_spread", max(quantiles[90] - quantiles[50], 0))
        signals = {
            "quantile_spread": spread,
            "ood_distance": outputs.get("ood_distance", 0),
            "feature_coverage": task.coverage_ratio,
        }
        return predictor_pb2.Prediction(model_version=self.manifest.get("model_version", ""), quantiles=quantiles, signals=signals)


def feature_value(task, name: str) -> float:
    if name == "coverage_level":
        return COVERAGE_LEVEL_ENCODING.get(str(task.coverage_level or "none"), 0.0)
    return float(getattr(task, name, 0) or 0)


def tensor(value: float):
    return np.array([value], dtype=np.float32)


def scalar(value) -> float:
    item = value
    while hasattr(item, "__len__") and not isinstance(item, (bytes, str)):
        item = item[0]
    return float(item)


def start_server(artifact_dir: Path, address: str):
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    predictor_pb2_grpc.add_OutputTokenPredictorServicer_to_server(ONNXWorker(artifact_dir), server)
    port = server.add_insecure_port(address)
    if port == 0:
        raise RuntimeError(f"could not bind predictor worker address {address}")
    server.start()
    return server, port


def main() -> None:
    parser = argparse.ArgumentParser(prog="scheduler-training-onnx-worker")
    parser.add_argument("--artifact-dir", required=True)
    parser.add_argument("--addr", default="127.0.0.1:50052")
    args = parser.parse_args()
    server, port = start_server(Path(args.artifact_dir), args.addr)
    print(port, flush=True)
    server.wait_for_termination()


if __name__ == "__main__":
    main()
