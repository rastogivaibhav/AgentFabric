#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
START_STACK="${START_STACK:-false}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

step() {
  echo
  echo "==> $1"
}

if [[ "${START_STACK}" == "true" ]]; then
  step "Starting local stack"
  "${REPO_ROOT}/scripts/setup-local.sh"
fi

step "Checking health and docs"
curl -fsS "${BASE_URL}/healthz" >/dev/null
curl -fsS "${BASE_URL}/docs/openapi.yaml" >/dev/null
curl -fsS "${BASE_URL}/docs/swagger" >/dev/null

step "Checking public runtime endpoints"
curl -fsS "${BASE_URL}/api/v1/analytics/overview" >/dev/null
curl -fsS "${BASE_URL}/api/v1/environments" >/dev/null

if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" ]]; then
  step "Checking authenticated admin paths"
  cookie_jar="$(mktemp)"
  trap 'rm -f "${cookie_jar}"' EXIT
  curl -fsS -c "${cookie_jar}" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASSWORD}\"}" \
    "${BASE_URL}/auth/login" >/dev/null
  curl -fsS -b "${cookie_jar}" "${BASE_URL}/auth/me" >/dev/null
  curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/pricing" >/dev/null
  curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/policies" >/dev/null
  curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/audit/control" >/dev/null
else
  echo "Skipping authenticated admin validation because ADMIN_USER/ADMIN_PASSWORD were not provided."
fi

echo
echo "Release candidate validation passed."
