.DEFAULT_GOAL := help
GO_ENV := CGO_ENABLED=0

.PHONY: help
help: ## List the available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# --- local development -------------------------------------------------------
.PHONY: install dev-api dev-web gendata analyze
install: ## Install frontend dependencies and download Go modules
	cd backend && go mod download
	cd frontend && npm ci

dev-api: ## Run the API with the in-memory store (http://localhost:8080)
	cd backend && DATA_DIR=data go run ./cmd/api

dev-web: ## Run the SPA with Vite, proxying /api to :8080 (http://localhost:5173)
	cd frontend && npm run dev

gendata: ## Write the synthetic test dataset to backend/data-synthetic
	cd backend && go run ./cmd/gendata -out data-synthetic

analyze: ## Run the engine over backend/data and print the findings (challenge JSON format)
	cd backend && go run ./cmd/analyze -data data -v

# --- quality -------------------------------------------------------------------
.PHONY: test test-backend test-frontend lint
test: test-backend test-frontend ## Run every test suite

test-backend: ## Go tests with the race detector (set TEST_DATABASE_URL to include Postgres)
	cd backend && go test -race -count=1 ./...

test-frontend: ## Vitest + Testing Library
	cd frontend && npm test

lint: ## go vet + gofmt check + oxlint + TypeScript
	cd backend && go vet ./... && test -z "$$(gofmt -l .)"
	cd frontend && npm run lint && npm run typecheck

# --- containers ----------------------------------------------------------------
.PHONY: up down logs
up: ## Build and start PostgreSQL + API + web (http://localhost:8080)
	@test -f .env || (cp .env.example .env && echo "created .env from .env.example")
	docker compose up --build -d

down: ## Stop the stack
	docker compose down

logs: ## Follow the stack logs
	docker compose logs -f
