# syntax=docker/dockerfile:1
#
# Build the ONNX worker image from a Git repository without pre-cloning the code:
# docker build -f docker/onnx-worker.remote-build.Dockerfile \
#   --build-arg VELOXMESH_REPO_URL=https://github.com/your-org/VeloxMesh.git \
#   --build-arg VELOXMESH_BRANCH=main \
#   -t veloxmesh-onnx-worker:main .

FROM alpine:3.22 AS source

ARG VELOXMESH_REPO_URL
ARG VELOXMESH_BRANCH=main

RUN apk add --no-cache git
RUN test -n "$VELOXMESH_REPO_URL"
RUN git clone --depth 1 --branch "$VELOXMESH_BRANCH" "$VELOXMESH_REPO_URL" /src

FROM python:3.12-slim

WORKDIR /app

COPY --from=source /src/tools/scheduler_training/pyproject.toml /src/tools/scheduler_training/uv.lock ./
COPY --from=source /src/tools/scheduler_training/scheduler_training ./scheduler_training

RUN pip install --no-cache-dir .

USER nobody

CMD ["python", "-m", "scheduler_training.onnx_worker", "--artifact-dir=/models/current", "--addr=0.0.0.0:50052"]
