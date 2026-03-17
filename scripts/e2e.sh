#!/usr/bin/env bash
# AgentFabric — full-stack E2E test runner
#
# Usage: bash scripts/e2e.sh
#   or:  make e2e
#
# What it does:
#   1. Brings up the full docker-compose stack
#   2. Polls each critical service until healthy (or times out)
#   3. Exports AGENTFABRIC_API_URL so tests can talk to the live stack
#   4. Runs pytest -m integration against tests/e2e/
#   5. Always tears the stack down — even on failure (trap EXIT)
#
# Exit code mirrors pytest's exit code (0 = all pass, non-zero = failure).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE="docker compose -f ${REPO_ROOT}/docker-compose.yml"
E2E_DIR="${REPO_ROOT}/tests/e2e"

# Services whose health we wait for, and their probe URLs.
declare -A HEALTH_URLS=(
  ["api-gateway"]="http://localhost:8080/healthz"
  ["collector"]="http://localhost:4318/healthz"
)
WAIT_TIMEOUT=120   # seconds before giving up
WAIT_INTERVAL=3    # poll interval

# ─── Cleanup on exit ─────────────────────────────────────────────────────────
_teardown() {
  local code=$?
  echo ""
  echo "──────────────────────────────────────────"
  echo "  Tearing down docker-compose stack..."
  ${COMPOSE} down --remove-orphans >/dev/null 2>&1 || true
  if [ "$code" -eq 0 ]; then
    echo "  E2E result: PASS"
  else
    echo "  E2E result: FAIL (exit ${code})"
  fi
  echo "──────────────────────────────────────────"
  exit "$code"
}
trap _teardown EXIT

# ─── Start the stack ─────────────────────────────────────────────────────────
echo "──────────────────────────────────────────"
echo "  AgentFabric E2E — starting stack"
echo "──────────────────────────────────────────"
cd "${REPO_ROOT}"
${COMPOSE} up -d --build --quiet-pull

# ─── Wait for services ───────────────────────────────────────────────────────
echo ""
echo "  Waiting for services to become healthy (timeout: ${WAIT_TIMEOUT}s)..."

for svc in "${!HEALTH_URLS[@]}"; do
  url="${HEALTH_URLS[$svc]}"
  elapsed=0
  printf "  %-20s" "${svc}:"
  until curl -sf "$url" >/dev/null 2>&1; do
    if [ "$elapsed" -ge "$WAIT_TIMEOUT" ]; then
      echo " TIMEOUT"
      echo "ERROR: ${svc} did not become healthy within ${WAIT_TIMEOUT}s" >&2
      exit 1
    fi
    sleep "$WAIT_INTERVAL"
    elapsed=$((elapsed + WAIT_INTERVAL))
    printf "."
  done
  echo " ready (${elapsed}s)"
done

# ─── Install Python test dependencies ────────────────────────────────────────
echo ""
echo "  Installing Python test dependencies..."
if [ -f "${REPO_ROOT}/requirements-test.txt" ]; then
  pip install -q -r "${REPO_ROOT}/requirements-test.txt"
else
  pip install -q pytest requests
fi

# ─── Run integration tests ───────────────────────────────────────────────────
echo ""
echo "  Running integration tests..."
echo "──────────────────────────────────────────"
export AGENTFABRIC_API_URL="http://localhost:8080"
export AGENTFABRIC_OTLP_URL="http://localhost:4318"

cd "${E2E_DIR}"
python -m pytest -m integration -v --tb=short "$@"
