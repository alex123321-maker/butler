# Core stays Go; tensor libraries and weights are a prepared read-only bind.
FROM golang:1.27.2-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/local-models ./apps/local-models
FROM python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1
COPY --from=builder /out/local-models /usr/local/bin/local-models
COPY apps/local-models/model-worker.py /app/model-worker.py
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/local-models"]
