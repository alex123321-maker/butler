.PHONY: build test lint proto infra-up infra-down

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

proto:
	@powershell -NoProfile -Command "if (Get-Command protoc -ErrorAction SilentlyContinue) { Write-Host 'protoc is available; proto generation wiring will be added with Sprint 0 contract tasks.' } else { Write-Host 'protoc is not installed; proto generation wiring will be added with Sprint 0 contract tasks.' }"

infra-up:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml up -d

infra-down:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml down
