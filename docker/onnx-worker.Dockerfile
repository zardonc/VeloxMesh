# syntax=docker/dockerfile:1

FROM python:3.12-slim

WORKDIR /app

# Install uv for reproducible, lock-file-driven dependency installation
RUN pip install --no-cache-dir uv

COPY tools/scheduler_training/pyproject.toml tools/scheduler_training/uv.lock ./
COPY tools/scheduler_training/scheduler_training ./scheduler_training

# uv sync installs into .venv; set PATH so the venv python is used at runtime
RUN uv sync --frozen --no-dev
ENV PATH="/app/.venv/bin:$PATH"

USER nobody

CMD ["python", "-m", "scheduler_training.onnx_worker", "--artifact-dir=/models/current", "--addr=0.0.0.0:50052"]
