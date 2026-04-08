#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"
PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY:-}"
PROXY_PATH="${PROXY_PATH:-/proxy/openai/v1/chat/completions}"
DEFAULT_PROXY_BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"governance validation"}],"stream":false}'
PROXY_BODY_JSON="${PROXY_BODY_JSON:-$DEFAULT_PROXY_BODY}"

step() {
  echo
  echo "==> $1"
}

json_request() {
  local method="$1"
  local url="$2"
  local body="${3:-}"
  local cookie_jar="${4:-}"
  local extra_args=()
  if [[ -n "${cookie_jar}" ]]; then
    extra_args+=(-b "${cookie_jar}")
  fi
  if [[ -n "${body}" ]]; then
    curl -fsS -X "${method}" "${url}" \
      "${extra_args[@]}" \
      -H "Content-Type: application/json" \
      -d "${body}"
  else
    curl -fsS -X "${method}" "${url}" \
      "${extra_args[@]}"
  fi
}

delete_request() {
  local url="$1"
  local cookie_jar="$2"
  curl -fsS -X DELETE -b "${cookie_jar}" "${url}" >/dev/null
}

extract_id() {
  sed -n 's/.*"id":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1
}

if [[ -z "${ADMIN_USER}" || -z "${ADMIN_PASSWORD}" ]]; then
  echo "ADMIN_USER and ADMIN_PASSWORD are required for governance validation." >&2
  exit 1
fi

cookie_jar="$(mktemp)"
created_pricing_rule_id=""
declare -a created_policy_rule_ids=()

cleanup() {
  if [[ -n "${created_pricing_rule_id}" ]]; then
    delete_request "${BASE_URL}/api/v1/pricing/${created_pricing_rule_id}" "${cookie_jar}" || true
  fi
  for rule_id in "${created_policy_rule_ids[@]:-}"; do
    [[ -n "${rule_id}" ]] || continue
    delete_request "${BASE_URL}/api/v1/policies/${rule_id}" "${cookie_jar}" || true
  done
  rm -f "${cookie_jar}"
}
trap cleanup EXIT

step "Logging in as admin"
curl -fsS -c "${cookie_jar}" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASSWORD}\"}" \
  "${BASE_URL}/auth/login" >/dev/null

step "Creating temporary governance rules"
stamp="$(date +%s)"

allow_rule="$(json_request PUT "${BASE_URL}/api/v1/policies" "{\"name\":\"gv-allow-${stamp}\",\"rule_type\":\"traffic\",\"decision_mode\":\"fast\",\"enabled\":true,\"priority\":2000,\"action\":\"allow\",\"provider\":\"openai\",\"model_pattern\":\"gpt-4o-mini\",\"environment\":\"staging\",\"description\":\"temporary governance allow rule\"}" "${cookie_jar}")"
created_policy_rule_ids+=("$(printf '%s' "${allow_rule}" | extract_id)")

deny_rule="$(json_request PUT "${BASE_URL}/api/v1/policies" "{\"name\":\"gv-deny-${stamp}\",\"rule_type\":\"traffic\",\"decision_mode\":\"rego\",\"enabled\":true,\"priority\":2500,\"action\":\"deny\",\"provider\":\"openai\",\"model_pattern\":\"gpt-4o\",\"environment\":\"staging\",\"rego_module\":\"deny if input.environment == \\\"staging\\\" && input.estimated_tokens > 100\",\"description\":\"temporary governance deny rule\"}" "${cookie_jar}")"
created_policy_rule_ids+=("$(printf '%s' "${deny_rule}" | extract_id)")

redact_rule="$(json_request PUT "${BASE_URL}/api/v1/policies" "{\"name\":\"gv-redact-${stamp}\",\"rule_type\":\"dlp\",\"decision_mode\":\"fast\",\"enabled\":true,\"priority\":2600,\"action\":\"redact\",\"detector\":\"secret\",\"scope\":\"request\",\"description\":\"temporary governance redact rule\"}" "${cookie_jar}")"
created_policy_rule_ids+=("$(printf '%s' "${redact_rule}" | extract_id)")

warn_rule="$(json_request PUT "${BASE_URL}/api/v1/policies" "{\"name\":\"gv-warn-${stamp}\",\"rule_type\":\"dlp\",\"decision_mode\":\"rego\",\"enabled\":true,\"priority\":2550,\"action\":\"warn\",\"scope\":\"response\",\"rego_module\":\"warn if input.scope == \\\"response\\\" && input.response_body contains \\\"@\\\"\",\"description\":\"temporary governance warn rule\"}" "${cookie_jar}")"
created_policy_rule_ids+=("$(printf '%s' "${warn_rule}" | extract_id)")

pricing_rule="$(json_request PUT "${BASE_URL}/api/v1/pricing" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model_pattern\":\"gpt-4o-mini\",\"input_per_million\":8.0,\"output_per_million\":16.0,\"active\":true,\"priority\":999,\"description\":\"temporary governance tenant pricing override\"}" "${cookie_jar}")"
created_pricing_rule_id="$(printf '%s' "${pricing_rule}" | extract_id)"
[[ -n "${created_pricing_rule_id}" ]] || { echo "pricing rule creation did not return an id" >&2; exit 1; }

step "Scenario allow"
allow_preview="$(json_request POST "${BASE_URL}/api/v1/policies/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"environment\":\"staging\",\"estimated_tokens\":20,\"request_body\":\"safe request\",\"response_body\":\"safe response\"}" "${cookie_jar}")"
echo "${allow_preview}" | grep -q '"action":"allow"' || { echo "scenario_allow failed" >&2; exit 1; }

step "Scenario deny"
deny_preview="$(json_request POST "${BASE_URL}/api/v1/policies/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o\",\"environment\":\"staging\",\"estimated_tokens\":200,\"request_body\":\"safe request\",\"response_body\":\"safe response\"}" "${cookie_jar}")"
echo "${deny_preview}" | grep -q '"action":"deny"' || { echo "scenario_deny_model failed" >&2; exit 1; }

step "Scenario redact"
redact_preview="$(json_request POST "${BASE_URL}/api/v1/policies/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"environment\":\"staging\",\"estimated_tokens\":32,\"request_body\":\"secret sk-abcdefghijklmnopqrstuvwxyz12345\",\"response_body\":\"safe response\"}" "${cookie_jar}")"
echo "${redact_preview}" | grep -q '"request_dlp"' || { echo "scenario_redact_secret missing request_dlp" >&2; exit 1; }
echo "${redact_preview}" | grep -q '"action":"redact"' || { echo "scenario_redact_secret failed" >&2; exit 1; }

step "Scenario warn"
warn_preview="$(json_request POST "${BASE_URL}/api/v1/policies/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"environment\":\"staging\",\"estimated_tokens\":32,\"request_body\":\"safe request\",\"response_body\":\"contact me at analyst@example.com\"}" "${cookie_jar}")"
echo "${warn_preview}" | grep -q '"response_dlp"' || { echo "scenario_warn_pii missing response_dlp" >&2; exit 1; }
echo "${warn_preview}" | grep -q '"action":"warn"' || { echo "scenario_warn_pii failed" >&2; exit 1; }

step "Scenario tenant override pricing"
pricing_preview="$(json_request POST "${BASE_URL}/api/v1/pricing/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"input_tokens\":1000,\"output_tokens\":1000}" "${cookie_jar}")"
echo "${pricing_preview}" | grep -q "\"rule_id\":${created_pricing_rule_id}" || { echo "scenario_tenant_override_pricing failed" >&2; exit 1; }

step "Scenario budget limit"
json_request PUT "${BASE_URL}/api/v1/budgets/${TENANT_ID}" "{\"monthly_tokens\":1,\"monthly_cost_usd\":0.000001,\"alert_threshold\":0.5,\"hard_limit\":true,\"reset_day\":1}" "${cookie_jar}" >/dev/null
budget_usage="$(json_request GET "${BASE_URL}/api/v1/budgets/${TENANT_ID}/usage" "" "${cookie_jar}")"
echo "${budget_usage}" | grep -q '"tokens_used"' || { echo "scenario_budget_limit usage lookup failed" >&2; exit 1; }

if [[ -n "${PROXY_VIRTUAL_KEY}" ]]; then
  step "Live proxy budget check"
  proxy_status="$(curl -sS -o /tmp/govagn-governance-proxy.out -w "%{http_code}" \
    -H "Authorization: Bearer ${PROXY_VIRTUAL_KEY}" \
    -H "Content-Type: application/json" \
    -d "${PROXY_BODY_JSON}" \
    "${BASE_URL}${PROXY_PATH}")"
  if [[ "${proxy_status}" != "429" && "${proxy_status}" != "200" ]]; then
    echo "unexpected proxy status during budget validation: ${proxy_status}" >&2
    exit 1
  fi
else
  echo "Skipping live 429 proxy validation because PROXY_VIRTUAL_KEY was not provided."
fi

step "Control-plane audit visibility"
control_audit="$(json_request GET "${BASE_URL}/api/v1/audit/control" "" "${cookie_jar}")"
echo "${control_audit}" | grep -q '"count"' || { echo "control-plane audit did not show governance mutations" >&2; exit 1; }

echo
echo "Staging governance validation passed."
