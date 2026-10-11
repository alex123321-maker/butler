# syntax=docker/dockerfile:1.7
FROM golang:1.27.2-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/tool-converter ./apps/tool-converter

FROM alpine:3.21
RUN adduser -D -H -u 10001 butler && mkdir /run/butler-converter && chown butler:butler /run/butler-converter && chmod 0700 /run/butler-converter
WORKDIR /app
COPY --from=builder /out/tool-converter /usr/local/bin/tool-converter
USER butler
ENTRYPOINT ["/usr/local/bin/tool-converter"]
