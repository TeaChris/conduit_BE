.PHONY: help build dev test test-integration lint fmt up down migrate-up migrate-down migrate-create sqlc generate doctor clean

# Default target
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Build & Run ---

build: ## Build the production binary
	CGO_ENABLED=0 go build -ldflags='-w -s' -o bin/server ./cmd/server

dev: ## Run with hot reload (requires air)
	air -c .air.toml || go run ./cmd/server

run: ## Run the server directly
	go run ./cmd/server

# --- Testing ---

test: ## Run unit tests
	go test -race -count=1 ./internal/...

test-integration: ## Run integration tests (requires running DB and Redis)
	go test -race -count=1 -tags=integration ./tests/...

test-coverage: ## Run tests with coverage report
	go test -race -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html

# --- Code Quality ---

lint: ## Run linter
	golangci-lint run ./...

fmt: ## Format code
	gofumpt -l -w .
	goimports -l -w .

# --- Infrastructure ---

up: ## Start Postgres and Redis containers
	docker compose up -d
	@echo "Waiting for services to be healthy..."
	@docker compose exec postgres pg_isready -U conduit > /dev/null 2>&1 || sleep 3
	@echo "Services are ready!"

down: ## Stop infrastructure containers
	docker compose down

down-clean: ## Stop containers and remove volumes
	docker compose down -v

# --- Database Migrations ---

MIGRATE_DSN ?= postgres://conduit:conduit@localhost:5432/conduit?sslmode=disable

migrate-up: ## Run all pending migrations
	migrate -path migrations -database "$(MIGRATE_DSN)" up

migrate-down: ## Rollback the last migration
	migrate -path migrations -database "$(MIGRATE_DSN)" down 1

migrate-create: ## Create a new migration (usage: make migrate-create name=create_users)
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=migration_name"; exit 1; fi
	migrate create -ext sql -dir migrations -seq $(name)

# --- Code Generation ---

sqlc: ## Generate sqlc code
	sqlc generate -f sqlc/sqlc.yaml

generate: sqlc ## Run all code generation
	@echo "Code generation complete."

# --- Developer Setup ---

doctor: ## Check that all required tools are installed
	@echo "Checking prerequisites..."
	@command -v go >/dev/null 2>&1 && echo "  ✓ go" || echo "  ✗ go (install from https://go.dev)"
	@command -v docker >/dev/null 2>&1 && echo "  ✓ docker" || echo "  ✗ docker (install from https://docker.com)"
	@command -v migrate >/dev/null 2>&1 && echo "  ✓ migrate" || echo "  ✗ migrate (go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest)"
	@command -v sqlc >/dev/null 2>&1 && echo "  ✓ sqlc" || echo "  ✗ sqlc (go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest)"
	@command -v golangci-lint >/dev/null 2>&1 && echo "  ✓ golangci-lint" || echo "  ✗ golangci-lint (https://golangci-lint.run/usage/install/)"
	@command -v air >/dev/null 2>&1 && echo "  ✓ air" || echo "  ✗ air (go install github.com/air-verse/air@latest)"
	@command -v gofumpt >/dev/null 2>&1 && echo "  ✓ gofumpt" || echo "  ✗ gofumpt (go install mvdan.cc/gofumpt@latest)"
	@echo "Done."

clean: ## Remove build artifacts
	rm -rf bin/ tmp/ coverage.out coverage.html
