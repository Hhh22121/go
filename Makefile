GOLANGCI_LINT_VERSION ?= v2.14.0
LOAD_ENV = set -a; [ -f .env ] && . ./.env; set +a;

.PHONY: run build test test-integration lint sqlc migrate-up migrate-down migrate-new docker-up docker-down

run:
	@$(LOAD_ENV) go run ./cmd/api

build:
	CGO_ENABLED=0 go build -trimpath -o bin/api ./cmd/api

test:
	GIN_MODE=test go test -race ./...

test-integration:
	GIN_MODE=test go test -race -tags integration ./...

lint:
	docker run --rm -v $(CURDIR):/app -v $(shell go env GOMODCACHE):/go/pkg/mod -w /app golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) golangci-lint run

sqlc:
	go tool sqlc generate

migrate-up:
	@$(LOAD_ENV) go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@$(LOAD_ENV) go tool goose -dir migrations postgres "$$DATABASE_URL" down

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=add_things" && exit 1)
	go tool goose -dir migrations -s create $(name) sql

docker-up:
	docker compose up -d postgres

docker-down:
	docker compose down
