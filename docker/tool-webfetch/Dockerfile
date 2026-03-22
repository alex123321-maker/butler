# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/tool-webfetch ./apps/tool-webfetch

FROM alpine:3.21
RUN adduser -D -H -u 10001 butler
WORKDIR /app
COPY --from=builder /out/tool-webfetch /usr/local/bin/tool-webfetch
USER butler
ENTRYPOINT ["/usr/local/bin/tool-webfetch"]
