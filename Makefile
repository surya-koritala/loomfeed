.PHONY: all build clean test lint fmt run-api run-gateway run-provenance run-search migrate-up migrate-down docker-up docker-down help

.DEFAULT_GOAL := help

BINARY_DIR := bin
GO := go
GOFLAGS := -v

# Services
SERVICES := api gateway provenance search

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

all: build

build: $(addprefix build-,$(SERVICES)) ## Build all services

build-%:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/$* ./cmd/$*

clean: ## Remove built binaries
	rm -rf $(BINARY_DIR)

test: ## Run all tests
	$(GO) test ./internal/... ./tests/... -race -count=1

test-coverage: ## Run tests with coverage report
	$(GO) test ./internal/... ./tests/... -race -coverprofile=coverage.out -covermode=atomic
	$(GO) tool cover -html=coverage.out -o coverage.html

lint: ## Run golangci-lint
	golangci-lint run ./...

fmt: ## Format Go code
	gofmt -s -w .
	goimports -w .

vet: ## Run go vet
	$(GO) vet ./...

run-api: ## Run API server (port 8080)
	$(GO) run ./cmd/api

run-gateway: ## Run Gateway server (port 8081)
	$(GO) run ./cmd/gateway

run-provenance: ## Run Provenance service
	$(GO) run ./cmd/provenance

run-search: ## Run Search service
	$(GO) run ./cmd/search

migrate-up: ## Run database migrations
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down: ## Rollback last migration
	migrate -path migrations -database "$(DATABASE_URL)" down 1

migrate-create: ## Create new migration (name=migration_name)
	migrate create -ext sql -dir migrations -seq $(name)

docker-up: ## Start all services via Docker Compose
	docker compose -f deployments/docker-compose.yml up -d

docker-down: ## Stop Docker Compose services
	docker compose -f deployments/docker-compose.yml down

docker-build: ## Build Docker images
	docker compose -f deployments/docker-compose.yml build

seed: ## Seed database with demo data (humans, agents, communities, posts)
	$(GO) run ./cmd/seed

dev: ## Start Postgres + Redis, run migrations, start API + frontend
	@echo "Starting Postgres and Redis..."
	docker compose -f deployments/docker-compose.yml up -d postgres redis
	@echo "Waiting for Postgres..."
	@sleep 3
	@echo "Running migrations..."
	DATABASE_URL="postgres://loomfeed:loomfeed@localhost:5432/loomfeed?sslmode=disable" migrate -path migrations -database "postgres://loomfeed:loomfeed@localhost:5432/loomfeed?sslmode=disable" up || true
	@echo ""
	@echo "=== Services ready ==="
	@echo "  Postgres: localhost:5432"
	@echo "  Redis:    localhost:6379"
	@echo ""
	@echo "Run in separate terminals:"
	@echo "  make run-api       # API on :8080"
	@echo "  cd web && npm run dev  # Frontend on :5173"
