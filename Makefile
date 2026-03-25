# AgentFabric — root Makefile
# ──────────────────────────────────────────────────────────────────────────────
# Usage:
#   make dev       — build images and start the full dev stack
#   make prod-up   — start stack with production hardening overlay
#   make down      — stop and remove containers (data volumes preserved)
#   make build     — compile Go services + portal production bundle
#   make test      — run all unit / integration tests (no Docker required)
#   make lint      — static analysis for Go and TypeScript
#   make certs     — generate self-signed mTLS certs for local dev
#   make e2e       — full-stack E2E: bring up stack → test → tear down
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: dev prod-up down build test lint certs e2e migrate/up migrate/down migrate/status \
        test/integration integration/up integration/down

COMPOSE       := docker compose -f docker-compose.yml
COMPOSE_PROD  := $(COMPOSE) -f deploy/docker/docker-compose.prod.yml
GO_SERVICES   := ./api-gateway/... ./collector/...
PORTAL_DIR    := portal
TESTS_E2E_DIR := tests/e2e

# ─── Local dev stack ──────────────────────────────────────────────────────────

dev:
	$(COMPOSE) up -d --build
	@echo ""
	@echo "  Portal        → http://localhost:3000"
	@echo "  API Gateway   → http://localhost:8080"
	@echo "  OTLP gRPC     → localhost:4317"
	@echo "  OTLP HTTP     → http://localhost:4318"
	@echo "  Prometheus    → http://localhost:9090"
	@echo "  Grafana       → http://localhost:9091"
	@echo "  Jaeger        → http://localhost:16686"

prod-up:
	@echo "Starting AgentFabric with production hardening overlay..."
	@echo "Required env: AF_JWT_SECRET, AF_ADMIN_PASSWORD, AF_CORS_ORIGINS, DATABASE_URL, REDIS_URL, POSTGRES_PASSWORD"
	$(COMPOSE_PROD) up -d --build

down:
	$(COMPOSE) down

# ─── Build ────────────────────────────────────────────────────────────────────

build: build-go build-portal

build-go:
	cd api-gateway && go build ./cmd/server/...
	cd collector   && go build ./cmd/collector/...

build-portal:
	cd $(PORTAL_DIR) && npm ci --silent && npm run build

# ─── Test ─────────────────────────────────────────────────────────────────────

test: test-go test-portal

test-go:
	cd api-gateway && go test ./... -count=1 -race
	cd collector   && go test ./... -count=1 -race

test-portal:
	cd $(PORTAL_DIR) && npm ci --silent && npm run test -- --run

# ─── Integration tests ────────────────────────────────────────────────────────
# Requires Docker.  Starts a disposable Postgres + Redis, runs the tagged tests,
# then tears everything down.  Data is ephemeral (tmpfs) — safe to run in CI.
#
#   make test/integration
#   make test/integration INTEGRATION_DB_URL="postgres://..." INTEGRATION_REDIS_URL="redis://..."

COMPOSE_TEST   := docker compose -f docker-compose.test.yml
INT_DB_URL     ?= postgres://fabric:fabric_dev_only@localhost:5433/agentfabric?sslmode=disable
INT_REDIS_URL  ?= redis://localhost:6380

integration/up:
	$(COMPOSE_TEST) up -d
	@echo "Waiting for test Postgres to be healthy..."
	@until docker inspect af_test_postgres --format='{{.State.Health.Status}}' 2>/dev/null | grep -q healthy; do sleep 1; done
	@echo "Waiting for test Redis to be healthy..."
	@until docker inspect af_test_redis --format='{{.State.Health.Status}}' 2>/dev/null | grep -q healthy; do sleep 1; done
	@echo "Test infrastructure ready."

integration/down:
	$(COMPOSE_TEST) down -v

test/integration: integration/up
	cd api-gateway && go test -tags=integration ./tests/integration/... \
	    -db-url="$(INT_DB_URL)" \
	    -redis-url="$(INT_REDIS_URL)" \
	    -v -count=1 -timeout=120s; \
	    STATUS=$$?; \
	    $(MAKE) integration/down; \
	    exit $$STATUS

# ─── Lint ─────────────────────────────────────────────────────────────────────

lint: lint-go lint-portal

lint-go:
	@if command -v golangci-lint >/dev/null 2>&1; then \
	  cd api-gateway && golangci-lint run ./...; \
	  cd collector   && golangci-lint run ./...; \
	else \
	  echo "golangci-lint not found — skipping (install: https://golangci-lint.run/usage/install/)"; \
	fi

lint-portal:
	cd $(PORTAL_DIR) && npm ci --silent && npx tsc --noEmit && npm run lint

# ─── Certs ────────────────────────────────────────────────────────────────────

certs:
	bash scripts/generate-dev-certs.sh

# dev-tls: generate certs (if not present) and start the full stack with mTLS enabled.
# Services read AF_TLS_ENABLED=true and load certs from deploy/certs/ at startup.
# Run 'make certs' separately first if you want to inspect the certs before starting.
dev-tls:
	@test -f deploy/certs/server.crt || $(MAKE) certs
	AF_TLS_ENABLED=true $(COMPOSE) up -d --build
	@echo ""
	@echo "  mTLS enabled on collector — OTLP gRPC :4317 / HTTP :4318 now require TLS"
	@echo "  CA cert for clients: deploy/certs/ca.crt"
	@echo "  Client cert:         deploy/certs/client.crt"
	@echo "  Client key:          deploy/certs/client.key"

# ─── E2E ──────────────────────────────────────────────────────────────────────

e2e:
	bash scripts/e2e.sh

# ─── Migrations ───────────────────────────────────────────────────────────────
# Requires DATABASE_URL to be set, e.g.:
#   DATABASE_URL="postgres://fabric:fabric@localhost:5432/agentfabric?sslmode=disable" make migrate/up
#
# go run fetches the migrate CLI from the version pinned in api-gateway/go.mod —
# no separate binary installation is needed.

MIGRATE_DSN ?= $(or $(DATABASE_URL),postgres://fabric:fabric@localhost:5432/agentfabric?sslmode=disable)
MIGRATE_CMD  = cd api-gateway && go run github.com/golang-migrate/migrate/v4/cmd/migrate \
               -path ../deploy/migrations \
               -database "$(MIGRATE_DSN)"

migrate/up:
	@echo "Applying all pending migrations..."
	$(MIGRATE_CMD) up

migrate/down:
	@echo "Rolling back one migration..."
	$(MIGRATE_CMD) down 1

migrate/status:
	@echo "Migration version:"
	$(MIGRATE_CMD) version
