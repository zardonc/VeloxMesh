# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/scheduler ./cmd/scheduler

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 veloxmesh

WORKDIR /app

COPY --from=build /out/gateway /app/gateway
COPY --from=build /out/scheduler /app/scheduler

USER veloxmesh

CMD ["/app/gateway"]
