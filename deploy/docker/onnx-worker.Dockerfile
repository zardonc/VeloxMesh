# syntax=docker/dockerfile:1

FROM python:3.12-slim

WORKDIR /app

RUN pip install --no-cache-dir uv

COPY tools/scheduler_training/pyproject.toml tools/scheduler_training/uv.lock ./
COPY tools/scheduler_training/scheduler_training ./scheduler_training

RUN uv sync --frozen --no-dev
ENV PATH="/app/.venv/bin:$PATH"

USER nobody

CMD ["python", "-m", "scheduler_training.onnx_worker", "--artifact-dir=/models/current", "--addr=0.0.0.0:50052"]
