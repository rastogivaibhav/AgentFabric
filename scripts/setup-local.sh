#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE="docker compose -f ${REPO_ROOT}/docker-compose.yml"
ENV_FILE="${REPO_ROOT}/.env.local"
WAIT_SECONDS="${WAIT_SECONDS:-120}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

wait_for_http() {
  local name="$1"
  local url="$2"
  local elapsed=0
  printf "Waiting for %-12s" "${name}"
  until curl -fsS "${url}" >/dev/null 2>&1; do
    if [ "${elapsed}" -ge "${WAIT_SECONDS}" ]; then
      echo " timeout"
      echo "Timed out waiting for ${name} at ${url}" >&2
      exit 1
    fi
    sleep 2
    elapsed=$((elapsed + 2))
    printf "."
  done
  echo " ready"
}

require_cmd docker
require_cmd curl

if [ ! -f "${ENV_FILE}" ]; then
  cat > "${ENV_FILE}" <<'EOF'
AF_ENV=development
AF_AUTH_DISABLED=true
AF_JWT_SECRET=dev-secret-change-in-production
AF_ADMIN_PASSWORD=admin
AF_VAULT_KEY=0000000000000000000000000000000000000000000000000000000000000000
AF_CORS_ORIGINS=http://localhost:3000,http://localhost:5173
EOF
  echo "Created ${ENV_FILE}"
fi

echo "Starting AgentFabric local stack..."
cd "${REPO_ROOT}"
${COMPOSE} --env-file "${ENV_FILE}" up -d --build

wait_for_http "gateway" "http://localhost:8080/healthz"
wait_for_http "collector" "http://localhost:4318/healthz"

echo "Applying demo pricing and policy seeds..."
"${REPO_ROOT}/scripts/seed-demo-data.sh"

cat <<'EOF'

AgentFabric local stack is ready.

Gateway:      http://localhost:8080
Portal:       http://localhost:3000
Collector:    http://localhost:4318
Swagger UI:   http://localhost:8080/docs/swagger
Prometheus:   http://localhost:9090
Grafana:      http://localhost:9091
Jaeger:       http://localhost:16686

Stop the stack:
  docker compose -f docker-compose.yml down
EOF
