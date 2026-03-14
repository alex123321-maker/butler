FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/tool-browser ./apps/tool-browser

FROM alpine:3.21
RUN adduser -D -H -u 10001 butler
WORKDIR /app
COPY --from=builder /out/tool-browser /usr/local/bin/tool-browser
USER butler
ENTRYPOINT ["/usr/local/bin/tool-browser"]
