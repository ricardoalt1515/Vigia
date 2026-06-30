.PHONY: dev down logs tools migrate-up migrate-down sqlc test tidy worker seed-dev console-install console-dev

DATABASE_URL ?= postgres://vigia:vigia@localhost:5432/vigia?sslmode=disable
TOOL_BIN := $(CURDIR)/bin
GOOSE := $(TOOL_BIN)/goose
SQLC := $(TOOL_BIN)/sqlc
GOOSE_TOOL := github.com/pressly/goose/v3/cmd/goose
SQLC_TOOL := github.com/sqlc-dev/sqlc/cmd/sqlc

# --- local dev stack ---
dev: ## start Postgres + MinIO
	docker compose up -d postgres minio
	@echo "Postgres :5432  ·  MinIO :9000 (console :9001)"

down: ## stop the stack
	docker compose down

logs: ## stream local dependency logs
	docker compose logs -f postgres minio

# --- tools ---
tools: $(GOOSE) $(SQLC) ## install pinned goose and sqlc into ./bin

$(TOOL_BIN):
	mkdir -p $(TOOL_BIN)

$(GOOSE): go.mod go.sum | $(TOOL_BIN)
	GOBIN=$(TOOL_BIN) go install $(GOOSE_TOOL)

$(SQLC): go.mod go.sum | $(TOOL_BIN)
	GOBIN=$(TOOL_BIN) go install $(SQLC_TOOL)

# --- database ---
migrate-up: $(GOOSE) ## apply goose migrations
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down: $(GOOSE) ## roll back one goose migration
	$(GOOSE) -dir db/migrations postgres "$(DATABASE_URL)" down

sqlc: $(SQLC) ## regenerate type-safe queries
	$(SQLC) generate

# --- quality ---
test:
	go test ./...

tidy:
	go mod tidy

# --- workers and seed ---
worker: ## run the River worker process
	go run ./cmd/worker

seed-dev: ## seed demo tenant, debtor, and three interactions; prints tenant_api_key=<plaintext>
	go run ./cmd/seed dev-data

# --- console ---
console-install: ## install Next.js console dependencies (run once)
	cd apps/console && npm install

console-dev: ## start the Next.js console dev server (set VIGIA_API_KEY in .env.local first)
	cd apps/console && npm run dev
