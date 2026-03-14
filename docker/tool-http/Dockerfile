FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tool-http ./apps/tool-http

FROM alpine:3.21
RUN adduser -D -H -u 10001 butler
WORKDIR /app
COPY --from=builder /out/tool-http /usr/local/bin/tool-http
USER butler
ENTRYPOINT ["/usr/local/bin/tool-http"]
