.PHONY: build test lint proto infra-up infra-down migrate-up migrate-down

build:
	go build ./...

test:
	go test ./...

lint:
	go vet ./...

proto:
	protoc -I proto -I "$(CURDIR)" --go_out=. --go_opt=module=github.com/butler/butler --go-grpc_out=. --go-grpc_opt=module=github.com/butler/butler proto/common/v1/types.proto proto/run/v1/events.proto proto/session/v1/session.proto proto/orchestrator/v1/orchestrator.proto proto/toolbroker/v1/types.proto proto/toolbroker/v1/broker.proto proto/transport/v1/transport.proto

infra-up:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml up -d

infra-down:
	docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.dev.yml down

migrate-up:
	go run ./apps/migrator --direction=up

migrate-down:
	go run ./apps/migrator --direction=down
