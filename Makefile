.PHONY: build build-go build-web test test-go test-web lint lint-go lint-web test-orchestrator test-memory test-transport proto infra-up infra-down migrate-up migrate-down up down logs

GO_PACKAGES=./apps/... ./internal/... ./scripts/...

build:
	$(MAKE) build-go

build-go:
	go build $(GO_PACKAGES)

build-web:
	cd web && npm run build

test:
	$(MAKE) test-go

test-go:
	go test $(GO_PACKAGES)

test-web:
	cd web && npm run test:e2e

lint:
	$(MAKE) lint-go

lint-go:
	go vet $(GO_PACKAGES)

lint-web:
	cd web && npm run lint

test-orchestrator:
	go test ./apps/orchestrator/...

test-memory:
	go test ./internal/memory/...

test-transport:
	go test ./internal/transport/...

proto:
	protoc -I proto -I "$(CURDIR)" --go_out=. --go_opt=module=github.com/butler/butler --go-grpc_out=. --go-grpc_opt=module=github.com/butler/butler proto/common/v1/types.proto proto/run/v1/events.proto proto/session/v1/session.proto proto/orchestrator/v1/orchestrator.proto proto/toolbroker/v1/types.proto proto/toolbroker/v1/broker.proto proto/runtime/v1/runtime.proto proto/transport/v1/transport.proto

infra-up:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml up -d postgres redis

infra-down:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml stop postgres redis

up:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml up -d --build

down:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml down

logs:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml logs -f --tail=100

migrate-up:
	go run ./apps/migrator --direction=up

migrate-down:
	go run ./apps/migrator --direction=down
