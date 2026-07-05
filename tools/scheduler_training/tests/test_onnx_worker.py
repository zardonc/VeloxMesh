import json

import grpc

from scheduler_training.onnx_worker import predictor_pb2, predictor_pb2_grpc, start_server
from scheduler_training.publish import publish_artifact
from scheduler_training.train import train_file


def write_samples(path):
    rows = [
        {"task_id": "t1", "output_tokens": 10, "outcome": "success"},
        {"task_id": "t2", "output_tokens": 20, "outcome": "success"},
        {"task_id": "t3", "output_tokens": 30, "outcome": "success"},
    ]
    path.write_text("\n".join(json.dumps(row) for row in rows), encoding="utf-8")


def published_artifact(tmp_path):
    samples = tmp_path / "samples.jsonl"
    model = tmp_path / "build" / "model.json"
    metrics = tmp_path / "build" / "metrics.json"
    write_samples(samples)
    train_file(samples, model)
    metrics.write_text(json.dumps({"sample_count": 3}), encoding="utf-8")
    return publish_artifact(model, metrics, tmp_path / "artifacts", "scheduler-predictor-v1", {})


def test_worker_calls_onnxruntime_session_and_returns_quantiles(tmp_path):
    artifact = published_artifact(tmp_path)
    server, port = start_server(artifact, "127.0.0.1:0")
    try:
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        stub = predictor_pb2_grpc.OutputTokenPredictorStub(channel)

        health = stub.Health(predictor_pb2.HealthRequest())
        response = stub.BatchPredict(
            predictor_pb2.BatchPredictRequest(
                tasks=[
                    predictor_pb2.TaskFeature(task_id="small", estimated_output_tokens=1),
                    predictor_pb2.TaskFeature(task_id="large", estimated_input_tokens=20, estimated_output_tokens=40),
                ]
            )
        )

        assert health.ready is True
        assert response.predictions[1].quantiles[70] > response.predictions[0].quantiles[70]
        assert response.predictions[0].quantiles[50] < response.predictions[0].quantiles[70]
        assert response.predictions[0].quantiles[90] > response.predictions[0].quantiles[70]
        assert response.predictions[0].signals["quantile_spread"] > 0
    finally:
        server.stop(0)


def test_worker_reports_malformed_task_without_blocking_siblings(tmp_path):
    artifact = published_artifact(tmp_path)
    server, port = start_server(artifact, "127.0.0.1:0")
    try:
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        stub = predictor_pb2_grpc.OutputTokenPredictorStub(channel)
        request = predictor_pb2.BatchPredictRequest(
            tasks=[
                predictor_pb2.TaskFeature(task_id="bad", estimated_input_tokens=-1),
                predictor_pb2.TaskFeature(task_id="ok"),
            ]
        )

        response = stub.BatchPredict(request)

        assert response.predictions[0].error == "invalid_task"
        assert response.predictions[1].quantiles[70] > 0
    finally:
        server.stop(0)
