# syntax=docker/dockerfile:1

FROM python:3.12-slim

WORKDIR /app

COPY tools/scheduler_training/pyproject.toml tools/scheduler_training/uv.lock ./
COPY tools/scheduler_training/scheduler_training ./scheduler_training

RUN pip install --no-cache-dir .

USER nobody

CMD ["python", "-m", "scheduler_training.onnx_worker", "--artifact-dir=/models/current", "--addr=0.0.0.0:50052"]
