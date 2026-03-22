#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"

step() {
  echo
  echo "==> $1"
}

assert_contains() {
  local body="$1"
  local pattern="$2"
  local message="$3"
  echo "${body}" | grep -q "${pattern}" || { echo "${message}" >&2; exit 1; }
}

step "Probing gateway health"
gateway_health="$(curl -fsS "${BASE_URL}/healthz")"
assert_contains "${gateway_health}" '"status":"ok"' "gateway healthz is not ok"

step "Probing gateway readiness"
gateway_ready="$(curl -fsS "${BASE_URL}/readyz")"
assert_contains "${gateway_ready}" '"status":"ok"' "gateway readyz is not ok"
assert_contains "${gateway_ready}" '"postgres":{"status":"ok"}' "postgres readiness failed"
assert_contains "${gateway_ready}" '"redis":{"status":"ok"}' "redis readiness failed"
assert_contains "${gateway_ready}" '"pricing_rules":{"status":"loaded"' "pricing rules not loaded"
assert_contains "${gateway_ready}" '"policy_engine":{"status":"loaded"' "policy engine not loaded"
assert_contains "${gateway_ready}" '"startup_state":{"status":"healthy"}' "startup state not healthy"

step "Probing collector health"
collector_health="$(curl -fsS "${COLLECTOR_URL}/healthz")"
assert_contains "${collector_health}" '"status":"ok"' "collector healthz is not ok"

step "Probing collector readiness"
collector_ready="$(curl -fsS "${COLLECTOR_URL}/readyz")"
assert_contains "${collector_ready}" '"status":"ok"' "collector readyz is not ok"
assert_contains "${collector_ready}" '"receiver":{"status":"ok"}' "collector receiver check failed"
assert_contains "${collector_ready}" '"gateway_export":{"status":"configured"}' "collector gateway export not configured"
assert_contains "${collector_ready}" '"pricing_config":{"status":"loaded"}' "collector pricing config not loaded"
assert_contains "${collector_ready}" '"gateway_auth_token":{"status":"configured"}' "collector gateway auth token missing"
assert_contains "${collector_ready}" '"gateway_readyz":{"status":"ok"' "collector gateway readiness probe failed"

echo
echo "Stack health probe passed."
