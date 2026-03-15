FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrator ./apps/migrator

FROM alpine:3.21
RUN adduser -D -H -u 10001 butler
WORKDIR /app
COPY --from=builder /out/migrator /usr/local/bin/migrator
COPY migrations/ /app/migrations/
USER butler
ENTRYPOINT ["/usr/local/bin/migrator"]
