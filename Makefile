.PHONY: build test lint run-api run-worker migrate-up migrate-down docker-up docker-down dev

# Build
build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker

# Run
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

# Test
test:
	go test ./... -cover -count=1

# Repository tests provision a private schema per package, so they are
# parallel-safe across packages and against other concurrent test runs.
test-repo:
	go test ./internal/repository/... -cover -count=1

test-race:
	go test ./... -race -count=1

# List leftover per-package test schemas (dry-run); add -apply to drop them.
test-db-gc:
	go run ./scripts/gc_test_schemas.go

test-db-gc-apply:
	go run ./scripts/gc_test_schemas.go -apply

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

# Database
migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

migrate-create:
	migrate create -ext sql -dir migrations -seq $(NAME)

# Docker
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

# Development
dev-api:
	air --build.cmd "go build -o bin/api ./cmd/api" --build.bin "./bin/api"

dev: docker-up
	$(MAKE) run-api

# Frontend
web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build
