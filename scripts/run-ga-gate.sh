#!/usr/bin/env bash
set -euo pipefail

MODE="${GA_GATE_MODE:-ga}"
BASE_URL="${BASE_URL:-http://localhost:8080}"
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY:-}"
PROXY_PATH="${PROXY_PATH:-/proxy/openai/v1/chat/completions}"
DEFAULT_PROXY_BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ga gate validation"}],"stream":false}'
PROXY_BODY_JSON="${PROXY_BODY_JSON:-$DEFAULT_PROXY_BODY}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"
OPEN_P0_COUNT="${OPEN_P0_COUNT:--1}"
OPEN_P1_COUNT="${OPEN_P1_COUNT:--1}"
OUTPUT_PATH="${OUTPUT_PATH:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

declare -a CHECKS=()
declare -a SUMMARY=()

add_check() {
  local name="$1"
  local passed="$2"
  local detail="$3"
  CHECKS+=("${passed}|${name}|${detail}")
}

invoke_required() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    add_check "${name}" "true" "passed"
  else
    add_check "${name}" "false" "failed"
  fi
}

docs_alignment() {
  local checklist="${REPO_ROOT}/docs/PRODUCTION_CHECKLIST.md"
  local boundaries="${REPO_ROOT}/docs/RELEASE_BOUNDARIES.md"
  [[ -f "${checklist}" ]] || { echo "missing ${checklist}" >&2; return 1; }
  [[ -f "${boundaries}" ]] || { echo "missing ${boundaries}" >&2; return 1; }
  local combined
  combined="$(cat "${checklist}" "${boundaries}")"
  for provider in openai anthropic google; do
    echo "${combined}" | grep -qi "${provider}" || { echo "missing provider ${provider} in docs" >&2; return 1; }
  done
  for stale in af-core clickhouse-svc kafka-svc CLICKHOUSE_URL KAFKA_; do
    if echo "${combined}" | grep -q "${stale}"; then
      echo "stale runtime reference ${stale} still present in docs" >&2
      return 1
    fi
  done
}

render_summary() {
  local decision="$1"
  local timestamp
  timestamp="$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  SUMMARY+=("# AgentFabric GA Gate")
  SUMMARY+=("")
  SUMMARY+=("- Decision: **${decision}**")
  SUMMARY+=("- Mode: \`${MODE}\`")
  SUMMARY+=("- Timestamp: \`${timestamp}\`")
  SUMMARY+=("")
  SUMMARY+=("## Evidence")
  for entry in "${CHECKS[@]}"; do
    IFS="|" read -r passed name detail <<<"${entry}"
    if [[ "${passed}" == "true" ]]; then
      SUMMARY+=("- [PASS] ${name}: ${detail}")
    else
      SUMMARY+=("- [FAIL] ${name}: ${detail}")
    fi
  done
  if [[ "${MODE}" == "ga" ]]; then
    SUMMARY+=("")
    SUMMARY+=("## Blockers")
    SUMMARY+=("- Open P0 blockers: \`${OPEN_P0_COUNT}\`")
    SUMMARY+=("- Open P1 blockers: \`${OPEN_P1_COUNT}\`")
  fi
  printf '%s\n' "${SUMMARY[@]}"
  if [[ -n "${OUTPUT_PATH}" ]]; then
    printf '%s\n' "${SUMMARY[@]}" >"${OUTPUT_PATH}"
  fi
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    printf '%s\n' "${SUMMARY[@]}" >>"${GITHUB_STEP_SUMMARY}"
  fi
}

invoke_required "Docs alignment" docs_alignment

if [[ "${MODE}" == "ci" ]]; then
  declare -A job_results=(
    ["collector tests"]="${GA_COLLECTOR_RESULT:-}"
    ["api-gateway tests"]="${GA_GATEWAY_RESULT:-}"
    ["portal tests/build"]="${GA_PORTAL_RESULT:-}"
    ["agent-sdk tests"]="${GA_SDK_RESULT:-}"
    ["helm smoke"]="${GA_HELM_RESULT:-}"
    ["packaging smoke"]="${GA_PACKAGING_RESULT:-}"
    ["secret scan"]="${GA_SECRET_SCAN_RESULT:-}"
  )
  failed=0
  for name in "${!job_results[@]}"; do
    result="${job_results[$name]}"
    if [[ "${result}" == "success" ]]; then
      add_check "${name}" "true" "result=${result}"
    else
      add_check "${name}" "false" "result=${result}"
      failed=1
    fi
  done
  if [[ "${failed}" -eq 0 ]]; then
    render_summary "CI PASS - staging evidence still required for GA"
    exit 0
  fi
  render_summary "CI NO-GO"
  exit 1
fi

if [[ "${GA_CI_GREEN:-false}" =~ ^(true|1|yes|success)$ ]]; then
  add_check "CI evidence" "true" "latest CI reported green"
else
  add_check "CI evidence" "false" "set GA_CI_GREEN=true after confirming latest CI is green"
fi

if [[ "${GA_PACKAGING_GREEN:-false}" =~ ^(true|1|yes|success)$ ]]; then
  add_check "Packaging evidence" "true" "external packaging evidence marked green"
else
  if docker compose -f "${REPO_ROOT}/docker-compose.yml" config >/dev/null 2>&1; then
    add_check "Compose render (local)" "true" "passed"
  else
    add_check "Compose render (local)" "false" "failed"
  fi
  if docker compose -f "${REPO_ROOT}/docker-compose.yml" -f "${REPO_ROOT}/deploy/docker/docker-compose.prod.yml" --env-file "${REPO_ROOT}/deploy/docker/.env.production.example" config >/dev/null 2>&1; then
    add_check "Compose render (production overlay)" "true" "passed"
  else
    add_check "Compose render (production overlay)" "false" "failed"
  fi
  if helm lint "${REPO_ROOT}/deploy/helm" >/dev/null 2>&1; then
    add_check "Helm lint" "true" "passed"
  else
    add_check "Helm lint" "false" "failed"
  fi
  if helm template agentfabric "${REPO_ROOT}/deploy/helm" --set collector.image.tag=ga --set api.image.tag=ga --set portal.image.tag=ga >/dev/null 2>&1; then
    add_check "Helm template" "true" "passed"
  else
    add_check "Helm template" "false" "failed"
  fi
fi

if bash "${REPO_ROOT}/scripts/probe-stack-health.sh" >/dev/null 2>&1; then
  add_check "Stack probe" "true" "passed"
else
  BASE_URL="${BASE_URL}" COLLECTOR_URL="${COLLECTOR_URL}" bash "${REPO_ROOT}/scripts/probe-stack-health.sh" >/dev/null 2>&1 \
    && add_check "Stack probe" "true" "passed" \
    || add_check "Stack probe" "false" "failed"
fi

if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" ]]; then
  add_check "Admin credentials" "true" "provided"
else
  add_check "Admin credentials" "false" "ADMIN_USER and ADMIN_PASSWORD are required for GA mode"
fi

if [[ -n "${PROXY_VIRTUAL_KEY}" ]]; then
  add_check "Proxy virtual key" "true" "provided"
else
  add_check "Proxy virtual key" "false" "PROXY_VIRTUAL_KEY is required for proxy proof and governance validation"
fi

if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" && -n "${PROXY_VIRTUAL_KEY}" ]]; then
  if BASE_URL="${BASE_URL}" ADMIN_USER="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASSWORD}" PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" PROXY_PATH="${PROXY_PATH}" PROXY_BODY_JSON="${PROXY_BODY_JSON}" TENANT_ID="${TENANT_ID}" bash "${REPO_ROOT}/scripts/probe-proxy-path.sh" >/dev/null 2>&1; then
    add_check "Proxy path probe" "true" "passed"
  else
    add_check "Proxy path probe" "false" "failed"
  fi

  if BASE_URL="${BASE_URL}" ADMIN_USER="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASSWORD}" PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" PROXY_PATH="${PROXY_PATH}" PROXY_BODY_JSON="${PROXY_BODY_JSON}" TENANT_ID="${TENANT_ID}" RUN_GOVERNANCE_SCENARIOS=true bash "${REPO_ROOT}/scripts/run-release-candidate-validation.sh" >/dev/null 2>&1; then
    add_check "Release candidate validation" "true" "passed"
  else
    add_check "Release candidate validation" "false" "failed"
  fi
fi

if [[ "${OPEN_P0_COUNT}" =~ ^[0-9]+$ && "${OPEN_P1_COUNT}" =~ ^[0-9]+$ ]]; then
  if [[ "${OPEN_P0_COUNT}" == "0" && "${OPEN_P1_COUNT}" == "0" ]]; then
    add_check "Release blockers declared" "true" "P0=${OPEN_P0_COUNT} P1=${OPEN_P1_COUNT}"
  else
    add_check "Release blockers declared" "false" "P0=${OPEN_P0_COUNT} P1=${OPEN_P1_COUNT}"
  fi
else
  add_check "Release blockers declared" "false" "OPEN_P0_COUNT and OPEN_P1_COUNT must be provided in GA mode"
fi

failed=0
for entry in "${CHECKS[@]}"; do
  IFS="|" read -r passed _ _ <<<"${entry}"
  if [[ "${passed}" != "true" ]]; then
    failed=1
  fi
done

if [[ "${failed}" -eq 0 ]]; then
  render_summary "GO"
  exit 0
fi

render_summary "NO-GO"
exit 1
