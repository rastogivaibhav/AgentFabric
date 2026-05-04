#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${1:-${REPO_ROOT}/docker-compose.yml}"
ENV_FILE="${REPO_ROOT}/.env.local"
LOG_DIR="${REPO_ROOT}/artifacts"
LOG_FILE="${LOG_DIR}/autostart.log"
DOCKER_WAIT_SECONDS="${DOCKER_WAIT_SECONDS:-180}"

mkdir -p "${LOG_DIR}"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "${LOG_FILE}"
}

wait_for_docker() {
  local elapsed=0
  until docker info >/dev/null 2>&1; do
    if (( elapsed >= DOCKER_WAIT_SECONDS )); then
      log "Docker daemon not ready after ${DOCKER_WAIT_SECONDS}s."
      return 1
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done
  return 0
}

log "Govagn autostart begin"
log "Using compose file: ${COMPOSE_FILE}"

if ! wait_for_docker; then
  if [[ "$(uname -s)" == "Darwin" ]]; then
    open -a Docker || true
    wait_for_docker
  else
    exit 1
  fi
fi

cd "${REPO_ROOT}"
if [[ -f "${ENV_FILE}" ]]; then
  docker compose -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" up -d --remove-orphans >> "${LOG_FILE}" 2>&1
else
  log ".env.local missing; running compose without explicit env file."
  docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans >> "${LOG_FILE}" 2>&1
fi

log "Govagn stack started successfully."
