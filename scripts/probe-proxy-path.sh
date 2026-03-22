#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY:-}"
PROXY_PATH="${PROXY_PATH:-/proxy/openai/v1/chat/completions}"
DEFAULT_PROXY_BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello secret sk-abcdefghijklmnopqrstuvwxyz12345"}],"stream":false}'
PROXY_BODY_JSON="${PROXY_BODY_JSON:-$DEFAULT_PROXY_BODY}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"

step() {
  echo
  echo "==> $1"
}

json_post() {
  local url="$1"
  local body="$2"
  local cookie_jar="$3"
  curl -fsS -b "${cookie_jar}" -H "Content-Type: application/json" -d "${body}" "${url}"
}

if [[ -z "${ADMIN_USER}" || -z "${ADMIN_PASSWORD}" ]]; then
  echo "ADMIN_USER and ADMIN_PASSWORD are required." >&2
  exit 1
fi
if [[ -z "${PROXY_VIRTUAL_KEY}" ]]; then
  echo "PROXY_VIRTUAL_KEY is required." >&2
  exit 1
fi

cookie_jar="$(mktemp)"
created_rule_id=""
cleanup() {
  if [[ -n "${created_rule_id}" ]]; then
    curl -fsS -X DELETE -b "${cookie_jar}" "${BASE_URL}/api/v1/policies/${created_rule_id}" >/dev/null || true
  fi
  rm -f "${cookie_jar}"
}
trap cleanup EXIT

step "Logging in"
curl -fsS -c "${cookie_jar}" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASSWORD}\"}" \
  "${BASE_URL}/auth/login" >/dev/null

step "Checking pricing preview"
pricing_preview="$(json_post "${BASE_URL}/api/v1/pricing/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"input_tokens\":120,\"output_tokens\":40}" "${cookie_jar}")"
echo "${pricing_preview}" | grep -q '"total_cost_usd"' || { echo "pricing preview missing total_cost_usd" >&2; exit 1; }

step "Checking policy preview"
policy_preview="$(json_post "${BASE_URL}/api/v1/policies/preview" "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"environment\":\"staging\",\"estimated_tokens\":64,\"request_body\":\"hello secret sk-abcdefghijklmnopqrstuvwxyz12345\",\"response_body\":\"safe response\"}" "${cookie_jar}")"
echo "${policy_preview}" | grep -q '"request_dlp"' || { echo "policy preview missing request_dlp" >&2; exit 1; }

step "Creating temporary request DLP rule"
stamp="$(date +%s)"
created_rule="$(json_post "${BASE_URL}/api/v1/policies" "{\"name\":\"proxy-proof-redact-${stamp}\",\"rule_type\":\"dlp\",\"decision_mode\":\"fast\",\"enabled\":true,\"priority\":3200,\"action\":\"redact\",\"detector\":\"secret\",\"scope\":\"request\",\"description\":\"temporary proxy-path proof rule\"}" "${cookie_jar}")"
created_rule_id="$(printf '%s' "${created_rule}" | sed -n 's/.*"id":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1)"
[[ -n "${created_rule_id}" ]] || { echo "failed to create temporary policy rule" >&2; exit 1; }

step "Sending proxied request"
before_traces="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/traces?framework=proxy&model=gpt-4o-mini&limit=5")"
curl -fsS \
  -H "Authorization: Bearer ${PROXY_VIRTUAL_KEY}" \
  -H "Content-Type: application/json" \
  -d "${PROXY_BODY_JSON}" \
  "${BASE_URL}${PROXY_PATH}" >/dev/null

step "Verifying trace, cost, audit, and policy visibility"
sleep 1
after_traces="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/traces?framework=proxy&model=gpt-4o-mini&limit=5")"
echo "${after_traces}" | grep -q '"items"' || { echo "trace list missing items" >&2; exit 1; }
trace_id="$(printf '%s' "${after_traces}" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p' | head -n1)"
[[ -n "${trace_id}" ]] || { echo "no proxy trace id found after proxied request" >&2; exit 1; }
trace_detail="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/traces/${trace_id}")"
trace_cost="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/traces/${trace_id}/cost")"
control_audit="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/audit/control")"
echo "${trace_detail}" | grep -q '"policy_events"' || { echo "trace detail missing policy_events" >&2; exit 1; }
echo "${trace_cost}" | grep -q '"total_usd"' || { echo "trace cost missing total_usd" >&2; exit 1; }
echo "${control_audit}" | grep -q '"count"' || { echo "control audit missing count" >&2; exit 1; }

echo
echo "Proxy path probe passed."
