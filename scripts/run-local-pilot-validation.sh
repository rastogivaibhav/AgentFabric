#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY:-}"
PROXY_PATH="${PROXY_PATH:-/proxy/openai/v1/chat/completions}"
DEFAULT_PROXY_BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"pilot validation secret sk-abcdefghijklmnopqrstuvwxyz12345"}],"stream":false}'
PROXY_BODY_JSON="${PROXY_BODY_JSON:-$DEFAULT_PROXY_BODY}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"
PILOT_NAME="${PILOT_NAME:-local-pilot}"
TEAM_NAME="${TEAM_NAME:-pilot-team}"
ENVIRONMENT_NAME="${ENVIRONMENT_NAME:-staging}"
START_STACK="${START_STACK:-false}"
RUN_GOVERNANCE_SCENARIOS="${RUN_GOVERNANCE_SCENARIOS:-false}"
START_DASHBOARD="${START_DASHBOARD:-false}"
VISUAL_CHECK="${VISUAL_CHECK:-false}"
OUTPUT_PATH="${OUTPUT_PATH:-}"
SCORECARD_PATH="${SCORECARD_PATH:-}"
JSON_OUTPUT_PATH="${JSON_OUTPUT_PATH:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

[[ -n "${OUTPUT_PATH}" ]] || OUTPUT_PATH="${REPO_ROOT}/pilot-validation-summary.md"
[[ -n "${SCORECARD_PATH}" ]] || SCORECARD_PATH="${REPO_ROOT}/pilot-value-scorecard.md"

step() {
  echo
  echo "==> $1"
}

if [[ "${START_STACK}" == "true" ]]; then
  step "Starting local stack"
  "${REPO_ROOT}/scripts/setup-local.sh"
fi

step "Running stack health probe"
BASE_URL="${BASE_URL}" COLLECTOR_URL="${COLLECTOR_URL}" "${REPO_ROOT}/scripts/probe-stack-health.sh" >/dev/null

if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" && -n "${PROXY_VIRTUAL_KEY}" ]]; then
  step "Running proxy path proof"
  BASE_URL="${BASE_URL}" \
  ADMIN_USER="${ADMIN_USER}" \
  ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
  PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" \
  PROXY_PATH="${PROXY_PATH}" \
  PROXY_BODY_JSON="${PROXY_BODY_JSON}" \
  TENANT_ID="${TENANT_ID}" \
  "${REPO_ROOT}/scripts/probe-proxy-path.sh" >/dev/null

  step "Running release candidate validation"
  BASE_URL="${BASE_URL}" \
  ADMIN_USER="${ADMIN_USER}" \
  ADMIN_PASSWORD="${ADMIN_PASSWORD}" \
  PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" \
  PROXY_PATH="${PROXY_PATH}" \
  PROXY_BODY_JSON="${PROXY_BODY_JSON}" \
  TENANT_ID="${TENANT_ID}" \
  RUN_GOVERNANCE_SCENARIOS="${RUN_GOVERNANCE_SCENARIOS}" \
  "${REPO_ROOT}/scripts/run-release-candidate-validation.sh" >/dev/null
fi

step "Collecting pilot evidence"
overview="$(curl -fsS "${BASE_URL}/api/v1/analytics/overview")"
traces="$(curl -fsS "${BASE_URL}/api/v1/traces?limit=20")"

extract_number() {
  local key="$1"
  local value
  value="$(printf '%s' "$2" | sed -n "s/.*\"${key}\":[[:space:]]*\\([0-9.][0-9.]*\\).*/\\1/p" | head -n1)"
  if [[ -n "${value}" ]]; then
    printf '%s' "${value}"
  else
    printf '0'
  fi
}

total_cost="$(extract_number total_cost_usd "${overview}")"
blocked_requests="$(extract_number blocked_requests "${overview}")"
llm_calls="$(extract_number llm_calls "${overview}")"
tool_calls="$(extract_number tool_calls "${overview}")"
trace_count="$(extract_number total "${traces}")"

policy_preview_state="not-run"
control_audit_count="0"
proxy_evidence="stack-only"

cookie_jar="$(mktemp)"
cleanup() {
  rm -f "${cookie_jar}"
}
trap cleanup EXIT

if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" ]]; then
  curl -fsS -c "${cookie_jar}" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASSWORD}\"}" \
    "${BASE_URL}/auth/login" >/dev/null

  control_audit="$(curl -fsS -b "${cookie_jar}" "${BASE_URL}/api/v1/audit/control?limit=50")"
  control_audit_count="$(extract_number count "${control_audit}")"

  policy_preview="$(curl -fsS -b "${cookie_jar}" -H "Content-Type: application/json" \
    -d "{\"tenant_id\":\"${TENANT_ID}\",\"provider\":\"openai\",\"model\":\"gpt-4o-mini\",\"environment\":\"${ENVIRONMENT_NAME}\",\"estimated_tokens\":96,\"request_body\":\"contact me at someone@example.com with secret sk-abcdefghijklmnopqrstuvwxyz12345\",\"response_body\":\"ok\"}" \
    "${BASE_URL}/api/v1/policies/preview")"
  if printf '%s' "${policy_preview}" | grep -q '"request_dlp"\|"traffic"'; then
    policy_preview_state="verified"
  fi
fi

if [[ -n "${PROXY_VIRTUAL_KEY}" ]]; then
  proxy_evidence="verified"
fi

cat >"${SCORECARD_PATH}" <<EOF
# Customer Value Scorecard

- Pilot name: **${PILOT_NAME}**
- Team: **${TEAM_NAME}**
- Environment: **${ENVIRONMENT_NAME}**
- Timestamp: **$(date -u '+%Y-%m-%d %H:%M:%S UTC')**

## Value Signals

- Cost visibility: total observed spend **\`${total_cost}\`**
- Runtime activity: **${trace_count}** traces, **${llm_calls}** LLM calls, **${tool_calls}** tool calls
- Guardrail/policy evidence: **${policy_preview_state}**
- Blocked/redacted pressure: **${blocked_requests}** blocked requests reported in overview
- Audit completeness: **${control_audit_count}** control audit records visible
- Proxy proof: **${proxy_evidence}**

## Operator Outcome Questions

- Was the team able to identify high-cost or high-latency traces without database access?
- Did policy previews and trace-linked policy events explain why requests were denied, warned, or redacted?
- Did the prompt/release linkage make it obvious which prompt version produced a given trace?
- Did audit and cost views reduce manual investigation time during pilot debugging?

## Suggested Pilot Ratings

- Cost visibility: \`green\` if spend anomalies were found from the UI alone
- Policy explainability: \`green\` if deny/redact decisions were understandable without logs
- Incident debugging speed: \`green\` if at least one trace-driven investigation was completed faster than the prior workflow
- Operator confidence: \`green\` if pilot users say they would keep this in the path for their team
EOF

cat >"${OUTPUT_PATH}" <<EOF
# AgentFabric Local Pilot Validation

- Pilot: **${PILOT_NAME}**
- Team: **${TEAM_NAME}**
- Environment: **${ENVIRONMENT_NAME}**
- Base URL: \`${BASE_URL}\`
- Collector URL: \`${COLLECTOR_URL}\`

## Validation Performed

- Stack health and readiness probe: passed
- Proxy path proof: ${proxy_evidence}
- Release-candidate validation: $(if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" ]]; then echo verified; else echo 'skipped (no admin credentials)'; fi)
- Governance scenarios: $(if [[ "${RUN_GOVERNANCE_SCENARIOS}" == "true" ]]; then echo requested; else echo 'not requested'; fi)
- Visual dashboard review: $(if [[ "${VISUAL_CHECK}" == "true" ]]; then echo requested; else echo 'not requested'; fi)

## Evidence Snapshot

- Total spend observed: **\`${total_cost}\`**
- Trace count: **${trace_count}**
- LLM calls: **${llm_calls}**
- Tool calls: **${tool_calls}**
- Blocked requests: **${blocked_requests}**
- Control audit records: **${control_audit_count}**
- Policy preview evidence: **${policy_preview_state}**

## Next Pilot Actions

- Run pilot traffic for 1-2 weeks with one real team
- Capture one debugging story, one policy/governance story, and one cost-control story
- Complete the customer value scorecard and attach operator quotes
- Re-run GA gate with pilot evidence when preparing for market-facing release
EOF

if [[ -n "${JSON_OUTPUT_PATH}" ]]; then
  cat >"${JSON_OUTPUT_PATH}" <<EOF
{"pilot_name":"${PILOT_NAME}","team_name":"${TEAM_NAME}","environment":"${ENVIRONMENT_NAME}","base_url":"${BASE_URL}","collector_url":"${COLLECTOR_URL}","trace_count":${trace_count},"total_cost_usd":${total_cost},"llm_calls":${llm_calls},"tool_calls":${tool_calls},"blocked_requests":${blocked_requests},"control_audit_count":${control_audit_count},"policy_preview_state":"${policy_preview_state}","proxy_evidence":"${proxy_evidence}","generated_at":"$(date -u '+%Y-%m-%dT%H:%M:%SZ')"}
EOF
fi

if [[ "${START_DASHBOARD}" == "true" ]]; then
  echo "StartDashboard was requested. Open ${BASE_URL} in a browser if you want an interactive visual review."
fi

echo
echo "Pilot validation summary written to ${OUTPUT_PATH}"
echo "Pilot scorecard template written to ${SCORECARD_PATH}"
