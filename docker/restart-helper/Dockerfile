# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/restart-helper ./apps/restart-helper

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/restart-helper /usr/local/bin/restart-helper
ENTRYPOINT ["/usr/local/bin/restart-helper"]
