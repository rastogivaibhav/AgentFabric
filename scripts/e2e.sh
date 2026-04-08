#!/usr/bin/env bash
# Govagn â€” full-stack E2E test runner
#
# Usage: bash scripts/e2e.sh
#   or:  make e2e
#
# What it does:
#   1. Brings up the full docker-compose stack
#   2. Polls each critical service until healthy (or times out)
#   3. Exports GOVAGN_API_URL so tests can talk to the live stack
#   4. Runs pytest -m integration against tests/e2e/
#   5. Always tears the stack down â€” even on failure (trap EXIT)
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

# â”€â”€â”€ Cleanup on exit â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
_teardown() {
  local code=$?
  echo ""
  echo "â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€"
  echo "  Tearing down docker-compose stack..."
  ${COMPOSE} down --remove-orphans >/dev/null 2>&1 || true
  if [ "$code" -eq 0 ]; then
    echo "  E2E result: PASS"
  else
    echo "  E2E result: FAIL (exit ${code})"
  fi
  echo "â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€"
  exit "$code"
}
trap _teardown EXIT

# â”€â”€â”€ Start the stack â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
echo "â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€"
echo "  Govagn E2E â€” starting stack"
echo "â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€"
cd "${REPO_ROOT}"
${COMPOSE} up -d --build --quiet-pull

# â”€â”€â”€ Wait for services â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
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

# â”€â”€â”€ Install Python test dependencies â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
echo ""
echo "  Installing Python test dependencies..."
if [ -f "${REPO_ROOT}/requirements-test.txt" ]; then
  pip install -q -r "${REPO_ROOT}/requirements-test.txt"
else
  pip install -q pytest requests
fi

# â”€â”€â”€ Run integration tests â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
echo ""
echo "  Running integration tests..."
echo "â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€"
export GOVAGN_API_URL="http://localhost:8080"
export GOVAGN_OTLP_URL="http://localhost:4318"

cd "${E2E_DIR}"
python -m pytest -m integration -v --tb=short "$@"

