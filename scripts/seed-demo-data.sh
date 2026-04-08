#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE="docker compose -f ${REPO_ROOT}/docker-compose.yml"
ENV_FILE="${REPO_ROOT}/.env.local"

cd "${REPO_ROOT}"
${COMPOSE} --env-file "${ENV_FILE}" exec -T postgres \
  psql -U fabric -d govagn -f /seed/demo_seed.sql >/dev/null

echo "Demo pricing and policy rules seeded."
