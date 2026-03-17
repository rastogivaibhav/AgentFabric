# AgentFabric — root Makefile
# ──────────────────────────────────────────────────────────────────────────────
# Usage:
#   make dev       — build images and start the full dev stack
#   make down      — stop and remove containers (data volumes preserved)
#   make build     — compile Go services + portal production bundle
#   make test      — run all unit / integration tests (no Docker required)
#   make lint      — static analysis for Go and TypeScript
#   make certs     — generate self-signed mTLS certs for local dev
#   make e2e       — full-stack E2E: bring up stack → test → tear down
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: dev down build test lint certs e2e

COMPOSE       := docker compose -f docker-compose.yml
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

# ─── E2E ──────────────────────────────────────────────────────────────────────

e2e:
	bash scripts/e2e.sh
