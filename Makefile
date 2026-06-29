.PHONY: dev down logs migrate-up migrate-down sqlc test seed api worker tidy

# --- local dev stack ---
dev: ## start Postgres + MinIO
	docker compose up -d
	@echo "Postgres :5432  ·  MinIO :9000 (console :9001)"

down: ## stop the stack
	docker compose down

logs:
	docker compose logs -f

# --- database ---
migrate-up: ## apply goose migrations
	goose -dir db/migrations postgres "$$DATABASE_URL" up

migrate-down: ## roll back one goose migration
	goose -dir db/migrations postgres "$$DATABASE_URL" down

sqlc: ## regenerate type-safe queries
	sqlc generate

# --- app ---
seed: ## load synthetic es-MX dataset
	go run ./cmd/seed

api: ## run the HTTP API
	go run ./cmd/api

worker: ## run the River worker
	go run ./cmd/worker

# --- quality ---
test:
	go test ./...

tidy:
	go mod tidy
