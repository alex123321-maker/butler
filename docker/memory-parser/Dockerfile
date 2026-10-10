FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /memory-parser ./apps/memory-parser

FROM alpine:3.23
RUN apk add --no-cache poppler-utils tesseract-ocr tesseract-ocr-data-rus tesseract-ocr-data-eng
COPY --from=build /memory-parser /usr/local/bin/memory-parser
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/memory-parser"]
