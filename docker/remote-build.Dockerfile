# syntax=docker/dockerfile:1
#
# Build the Go runtime image from a Git repository without pre-cloning the code:
# docker build -f docker/remote-build.Dockerfile \
#   --build-arg VELOXMESH_REPO_URL=https://github.com/your-org/VeloxMesh.git \
#   --build-arg VELOXMESH_BRANCH=main \
#   -t veloxmesh-go:main .

FROM alpine:3.22 AS source

ARG VELOXMESH_REPO_URL
ARG VELOXMESH_BRANCH=main

RUN apk add --no-cache git
RUN test -n "$VELOXMESH_REPO_URL"
RUN git clone --depth 1 --branch "$VELOXMESH_BRANCH" "$VELOXMESH_REPO_URL" /src

FROM golang:1.26.1-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY --from=source /src/go.mod /src/go.sum ./
RUN go mod download

COPY --from=source /src .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scheduler ./cmd/scheduler

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && adduser -D -H veloxmesh

WORKDIR /app

COPY --from=build /out/gateway /app/gateway
COPY --from=build /out/scheduler /app/scheduler

USER veloxmesh

CMD ["/app/gateway"]
