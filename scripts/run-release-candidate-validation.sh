#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
START_STACK="${START_STACK:-false}"
RUN_GOVERNANCE_SCENARIOS="${RUN_GOVERNANCE_SCENARIOS:-false}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"
PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY:-}"
PROXY_PATH="${PROXY_PATH:-/proxy/openai/v1/chat/completions}"
DEFAULT_PROXY_BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"governance validation"}],"stream":false}'
PROXY_BODY_JSON="${PROXY_BODY_JSON:-$DEFAULT_PROXY_BODY}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

step() {
  echo
  echo "==> $1"
}

json_post() {
  local url="$1"
  local body="$2"
  local cookie_jar="${3:-}"
  if [[ -n "${cookie_jar}" ]]; then
    curl -fsS -b "${cookie_jar}" -H "Content-Type: application/json" -d "${body}" "${url}"
  else
    curl -fsS -H "Content-Type: application/json" -d "${body}" "${url}"
  fi
}

if [[ "${START_STACK}" == "true" ]]; then
  step "Starting local stack"
  "${REPO_ROOT}/scripts/setup-local.sh"
fi

step "Checking readiness, health, and docs"
curl -fsS "${BASE_URL}/healthz" >/dev/null
curl -fsS "${BASE_URL}/readyz" >/dev/null
curl -fsS "${BASE_URL}/docs/openapi.yaml" >/dev/null
curl -fsS "${BASE_URL}/docs/swagger" >/dev/null

step "Checking public runtime endpoints"
curl -fsS "${BASE_URL}/api/v1/analytics/overview" >/dev/null
curl -fsS "${BASE_URL}/api/v1/environments" >/dev/null

if [[ -z "${ADMIN_USER}" || -z "${ADMIN_PASSWORD}" ]]; then
  echo "Skipping authenticated admin validation because ADMIN_USER/ADMIN_PASSWORD were not provided."
  echo
  echo "Release candidate validation passed."
  exit 0
fi

cookie_jar="$(mktemp)"
temp_rule_id=""
cleanup() {
  if [[ -n "${temp_rule_id}" ]]; then
    curl -fsS -X DELETE -b "${cookie_jar}" "${BASE_URL}/api/v1/policies/${temp_rule_id}" >/dev/null || true
  fi
  rm -f "${cookie_jar}"
}
trap cleanup EXIT

step "Checking authenticated admin paths"
curl -fsS -c "${cookie_jar}" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASSWORD}\"}" \
  "${BASE_URL}/auth/login" >/dev/null
curl -fsS -b "${cookie_jar}" "${BASE_URL}/auth/me" >/dev/null
pricing_rules="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/pricing")"
policy_rules="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/policies")"
control_audit="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/audit/control")"

echo "${pricing_rules}" | grep -q '"items"' || { echo "pricing API returned an unexpected shape" >&2; exit 1; }
echo "${policy_rules}" | grep -q '"items"' || { echo "policy API returned an unexpected shape" >&2; exit 1; }
echo "${control_audit}" | grep -q '"items"' || { echo "control audit API returned an unexpected shape" >&2; exit 1; }

step "Checking pricing preview"
pricing_preview="$(json_post "${BASE_URL}/api/v1/pricing/preview" '{"provider":"openai","model":"gpt-4o","input_tokens":120,"output_tokens":40}' "${cookie_jar}")"
echo "${pricing_preview}" | grep -q '"matched"' || { echo "pricing preview response was missing expected fields" >&2; exit 1; }
echo "${pricing_preview}" | grep -q '"total_cost_usd"' || { echo "pricing preview response was missing total_cost_usd" >&2; exit 1; }

step "Checking policy preview with a staged deny rule"
timestamp="$(date +%s)"
created_rule="$(json_post "${BASE_URL}/api/v1/policies" "{\"name\":\"rc-preview-${timestamp}\",\"rule_type\":\"traffic\",\"enabled\":true,\"priority\":9999,\"action\":\"deny\",\"provider\":\"openai\",\"model_pattern\":\"gpt-4o\",\"environment\":\"staging\",\"max_tokens\":10,\"description\":\"temporary release validation rule\"}" "${cookie_jar}")"
temp_rule_id="$(printf '%s' "${created_rule}" | sed -n 's/.*"id":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1)"
[[ -n "${temp_rule_id}" ]] || { echo "policy upsert did not return an id" >&2; exit 1; }

policy_preview="$(json_post "${BASE_URL}/api/v1/policies/preview" '{"provider":"openai","model":"gpt-4o","environment":"staging","estimated_tokens":128,"request_body":"contact me at someone@example.com","response_body":"safe response"}' "${cookie_jar}")"
echo "${policy_preview}" | grep -q '"traffic"' || { echo "policy preview response was missing traffic decision" >&2; exit 1; }
echo "${policy_preview}" | grep -q '"matched":[[:space:]]*true' || { echo "policy preview did not match the staged deny rule" >&2; exit 1; }
echo "${policy_preview}" | grep -q '"action":"deny"' || { echo "policy preview action was expected to be deny" >&2; exit 1; }

step "Checking DLP preview path"
dlp_preview="$(json_post "${BASE_URL}/api/v1/policies/preview" '{"provider":"openai","model":"gpt-4o","environment":"production","estimated_tokens":32,"request_body":"secret token sk-1234567890abcdefghijklmnop","response_body":"user email someone@example.com"}' "${cookie_jar}")"
echo "${dlp_preview}" | grep -q '"request_dlp"' || { echo "DLP preview was missing request_dlp" >&2; exit 1; }
echo "${dlp_preview}" | grep -q '"response_dlp"' || { echo "DLP preview was missing response_dlp" >&2; exit 1; }

step "Checking control-plane audit after temporary mutation"
control_audit_after="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/audit/control")"
echo "${control_audit_after}" | grep -q '"count"' || { echo "control audit response was missing count" >&2; exit 1; }

if [[ "${RUN_GOVERNANCE_SCENARIOS}" == "true" ]]; then
  step "Running governance battle-testing scenarios"
  BASE_URL="${BASE_URL}" \
  ADMIN_USER="${ADMIN_USER}" \
  ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
  TENANT_ID="${TENANT_ID}" \
  PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" \
  PROXY_PATH="${PROXY_PATH}" \
  PROXY_BODY_JSON="${PROXY_BODY_JSON}" \
  "${REPO_ROOT}/scripts/run-staging-governance-validation.sh"
fi

echo
echo "Release candidate validation passed."
