# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/tool-browser ./apps/tool-browser

FROM mcr.microsoft.com/playwright:v1.52.0-jammy
RUN useradd -m -u 10001 butler
WORKDIR /app
COPY apps/tool-browser/package.json /app/apps/tool-browser/package.json
RUN --mount=type=cache,target=/root/.npm cd /app/apps/tool-browser && npm install --omit=dev
COPY apps/tool-browser/scripts /app/apps/tool-browser/scripts
COPY --from=builder /out/tool-browser /usr/local/bin/tool-browser
USER butler
ENTRYPOINT ["/usr/local/bin/tool-browser"]
