#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
COLLECTOR_URL="${COLLECTOR_URL:-http://localhost:4318}"
ADMIN_USER="${ADMIN_USER:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY:-}"
PROXY_PATH="${PROXY_PATH:-/proxy/openai/v1/chat/completions}"
DEFAULT_PROXY_BODY='{"model":"gpt-4o-mini","messages":[{"role":"user","content":"production deployment validation"}],"stream":false}'
PROXY_BODY_JSON="${PROXY_BODY_JSON:-$DEFAULT_PROXY_BODY}"
TENANT_ID="${TENANT_ID:-00000000-0000-0000-0000-000000000001}"
DATABASE_URL="${DATABASE_URL:-}"
BACKUP_OUTPUT_DIR="${BACKUP_OUTPUT_DIR:-./backups/production-validation}"
NETPROXY_CA_CERT_FILE="${NETPROXY_CA_CERT_FILE:-}"
NETPROXY_CA_KEY_FILE="${NETPROXY_CA_KEY_FILE:-}"
LIVE_STREAM_SINGLE_REPLICA="${LIVE_STREAM_SINGLE_REPLICA:-false}"
LIVE_STREAM_FANOUT_READY="${LIVE_STREAM_FANOUT_READY:-false}"
SKIP_PACKAGING_SMOKE="${SKIP_PACKAGING_SMOKE:-false}"
SKIP_STACK_PROBE="${SKIP_STACK_PROBE:-false}"
SKIP_PROXY_PROBE="${SKIP_PROXY_PROBE:-false}"
SKIP_CANDIDATE_VALIDATION="${SKIP_CANDIDATE_VALIDATION:-false}"
SKIP_BACKUP_DRILL="${SKIP_BACKUP_DRILL:-false}"
SKIP_NETPROXY_CA_DRILL="${SKIP_NETPROXY_CA_DRILL:-false}"
OUTPUT_PATH="${OUTPUT_PATH:-}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKER_REPO_ROOT="${REPO_ROOT}"
if command -v pwd >/dev/null 2>&1 && [[ -n "${MSYSTEM:-}" ]]; then
  DOCKER_REPO_ROOT="$(cd "${REPO_ROOT}" && pwd -W)"
fi

env_true() {
  [[ "$1" =~ ^(1|true|yes|success)$ ]]
}

if [[ -n "${MSYSTEM:-}" ]]; then
  ps_args=(-NoProfile -ExecutionPolicy Bypass -File "${REPO_ROOT}/scripts/run_production_deployment_validation.ps1" -BaseUrl "${BASE_URL}" -CollectorUrl "${COLLECTOR_URL}" -AdminUser "${ADMIN_USER}" -AdminPassword "${ADMIN_PASSWORD}" -ProxyVirtualKey "${PROXY_VIRTUAL_KEY}" -ProxyPath "${PROXY_PATH}" -ProxyBodyJson "${PROXY_BODY_JSON}" -TenantId "${TENANT_ID}" -DatabaseUrl "${DATABASE_URL}" -BackupOutputDir "${BACKUP_OUTPUT_DIR}" -NetProxyCaCertFile "${NETPROXY_CA_CERT_FILE}" -NetProxyCaKeyFile "${NETPROXY_CA_KEY_FILE}" -OutputPath "${OUTPUT_PATH}")
  env_true "${LIVE_STREAM_SINGLE_REPLICA}" && ps_args+=(-LiveStreamSingleReplica)
  env_true "${LIVE_STREAM_FANOUT_READY}" && ps_args+=(-LiveStreamFanoutReady)
  env_true "${SKIP_PACKAGING_SMOKE}" && ps_args+=(-SkipPackagingSmoke)
  env_true "${SKIP_STACK_PROBE}" && ps_args+=(-SkipStackProbe)
  env_true "${SKIP_PROXY_PROBE}" && ps_args+=(-SkipProxyProbe)
  env_true "${SKIP_CANDIDATE_VALIDATION}" && ps_args+=(-SkipCandidateValidation)
  env_true "${SKIP_BACKUP_DRILL}" && ps_args+=(-SkipBackupDrill)
  env_true "${SKIP_NETPROXY_CA_DRILL}" && ps_args+=(-SkipNetProxyCaDrill)
  powershell.exe "${ps_args[@]}"
  exit $?
fi

[[ -n "${OUTPUT_PATH}" ]] || OUTPUT_PATH="${REPO_ROOT}/production-deployment-validation.md"

declare -a RESULTS=()

record_result() {
  local passed="$1"
  local name="$2"
  local detail="$3"
  RESULTS+=("${passed}|${name}|${detail}")
}

run_check() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    record_result "true" "${name}" "passed"
  else
    record_result "false" "${name}" "failed"
  fi
}

topology_detail=""
if env_true "${LIVE_STREAM_FANOUT_READY}"; then
  topology_detail="fan-out ready"
  record_result "true" "Live stream topology" "${topology_detail}"
elif env_true "${LIVE_STREAM_SINGLE_REPLICA}"; then
  topology_detail="single-replica acknowledged"
  record_result "true" "Live stream topology" "${topology_detail}"
else
  topology_detail="set LIVE_STREAM_SINGLE_REPLICA=true or LIVE_STREAM_FANOUT_READY=true"
  record_result "false" "Live stream topology" "${topology_detail}"
fi

if ! env_true "${SKIP_PACKAGING_SMOKE}"; then
  run_check "Compose render (local)" docker compose -f "${REPO_ROOT}/docker-compose.yml" config
  run_check "Compose render (production overlay)" docker compose -f "${REPO_ROOT}/docker-compose.yml" -f "${REPO_ROOT}/deploy/docker/docker-compose.prod.yml" --env-file "${REPO_ROOT}/deploy/docker/.env.production.example" config
  if [[ -n "${MSYSTEM:-}" ]]; then
    run_check "Helm lint" powershell.exe -NoProfile -Command "docker run --rm -v '${DOCKER_REPO_ROOT}:/work' -w /work alpine/helm:3.14.0 lint deploy/helm"
    run_check "Helm template" powershell.exe -NoProfile -Command "docker run --rm -v '${DOCKER_REPO_ROOT}:/work' -w /work alpine/helm:3.14.0 template govagn deploy/helm --set collector.image.tag=ga --set api.image.tag=ga --set portal.image.tag=ga"
    if powershell.exe -NoProfile -Command "docker run --rm -v '${DOCKER_REPO_ROOT}:/work' -w /work alpine/helm:3.14.0 template govagn deploy/helm --set api.replicas=2" >/dev/null 2>&1; then
      record_result "false" "Helm live-stream topology guard" "api.replicas=2 rendered successfully when it should fail"
    else
      record_result "true" "Helm live-stream topology guard" "api.replicas=2 render blocked as expected"
    fi
  else
    run_check "Helm lint" docker run --rm -v "${DOCKER_REPO_ROOT}:/work" -w /work alpine/helm:3.14.0 lint deploy/helm
    run_check "Helm template" docker run --rm -v "${DOCKER_REPO_ROOT}:/work" -w /work alpine/helm:3.14.0 template govagn deploy/helm --set collector.image.tag=ga --set api.image.tag=ga --set portal.image.tag=ga
    if docker run --rm -v "${DOCKER_REPO_ROOT}:/work" -w /work alpine/helm:3.14.0 template govagn deploy/helm --set api.replicas=2 >/dev/null 2>&1; then
      record_result "false" "Helm live-stream topology guard" "api.replicas=2 rendered successfully when it should fail"
    else
      record_result "true" "Helm live-stream topology guard" "api.replicas=2 render blocked as expected"
    fi
  fi
fi

if ! env_true "${SKIP_STACK_PROBE}"; then
  run_check "Stack probe" env BASE_URL="${BASE_URL}" COLLECTOR_URL="${COLLECTOR_URL}" "${REPO_ROOT}/scripts/probe-stack-health.sh"
fi

if ! env_true "${SKIP_PROXY_PROBE}"; then
  if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" && -n "${PROXY_VIRTUAL_KEY}" ]]; then
    run_check "Proxy path proof" env BASE_URL="${BASE_URL}" ADMIN_USER="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASSWORD}" PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" PROXY_PATH="${PROXY_PATH}" PROXY_BODY_JSON="${PROXY_BODY_JSON}" TENANT_ID="${TENANT_ID}" "${REPO_ROOT}/scripts/probe-proxy-path.sh"
  else
    record_result "false" "Proxy path proof" "ADMIN_USER, ADMIN_PASSWORD, and PROXY_VIRTUAL_KEY are required"
  fi
fi

if ! env_true "${SKIP_CANDIDATE_VALIDATION}"; then
  if [[ -n "${ADMIN_USER}" && -n "${ADMIN_PASSWORD}" && -n "${PROXY_VIRTUAL_KEY}" ]]; then
    run_check "Release candidate validation" env BASE_URL="${BASE_URL}" ADMIN_USER="${ADMIN_USER}" ADMIN_PASSWORD="${ADMIN_PASSWORD}" PROXY_VIRTUAL_KEY="${PROXY_VIRTUAL_KEY}" PROXY_PATH="${PROXY_PATH}" PROXY_BODY_JSON="${PROXY_BODY_JSON}" TENANT_ID="${TENANT_ID}" RUN_GOVERNANCE_SCENARIOS=true "${REPO_ROOT}/scripts/run-release-candidate-validation.sh"
  else
    record_result "false" "Release candidate validation" "ADMIN_USER, ADMIN_PASSWORD, and PROXY_VIRTUAL_KEY are required"
  fi
fi

backup_artifact="not-run"
if ! env_true "${SKIP_BACKUP_DRILL}"; then
  if [[ -n "${DATABASE_URL}" ]]; then
    mkdir -p "${BACKUP_OUTPUT_DIR}"
    if DATABASE_URL="${DATABASE_URL}" OUTPUT_DIR="${BACKUP_OUTPUT_DIR}" BACKUP_FORMAT=custom RETENTION_DAYS=7 "${REPO_ROOT}/scripts/backup-postgres.sh" >/dev/null 2>&1; then
      backup_artifact="$(find "${BACKUP_OUTPUT_DIR}" -maxdepth 1 -type f | sort | tail -n1)"
      if [[ -n "${backup_artifact}" ]]; then
        record_result "true" "Backup drill" "backup created at ${backup_artifact}"
      else
        record_result "false" "Backup drill" "backup script completed without creating an artifact"
      fi
    else
      record_result "false" "Backup drill" "backup script failed"
    fi
  else
    record_result "false" "Backup drill" "DATABASE_URL is required"
  fi
fi

netproxy_report="${REPO_ROOT}/netproxy-ca-drill.md"
if ! env_true "${SKIP_NETPROXY_CA_DRILL}"; then
  if [[ -n "${NETPROXY_CA_CERT_FILE}" && -n "${NETPROXY_CA_KEY_FILE}" ]]; then
    if NETPROXY_CA_CERT_FILE="${NETPROXY_CA_CERT_FILE}" NETPROXY_CA_KEY_FILE="${NETPROXY_CA_KEY_FILE}" OUTPUT_PATH="${netproxy_report}" "${REPO_ROOT}/scripts/exercise-netproxy-ca-backup-restore.sh" >/dev/null 2>&1; then
      record_result "true" "NetProxy CA drill" "summary written to ${netproxy_report}"
    else
      record_result "false" "NetProxy CA drill" "backup/restore drill failed"
    fi
  else
    record_result "false" "NetProxy CA drill" "NETPROXY_CA_CERT_FILE and NETPROXY_CA_KEY_FILE are required"
  fi
fi

failed=0
for entry in "${RESULTS[@]}"; do
  IFS="|" read -r passed _ _ <<<"${entry}"
  if [[ "${passed}" != "true" ]]; then
    failed=1
  fi
done

validation_result="PASS"
if [[ "${failed}" -ne 0 ]]; then
  validation_result="FAIL"
fi

{
  echo "# Govagn Production Deployment Validation"
  echo
  echo "- Validation result: ${validation_result}"
  echo "- Generated at: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
  echo "- Base URL: \`${BASE_URL}\`"
  echo "- Collector URL: \`${COLLECTOR_URL}\`"
  echo
  echo "## Packaging and Topology"
  for entry in "${RESULTS[@]}"; do
    IFS="|" read -r passed name detail <<<"${entry}"
    if [[ "${name}" =~ ^(Live\ stream\ topology|Compose render \(local\)|Compose render \(production overlay\)|Helm lint|Helm template|Helm live-stream topology guard)$ ]]; then
      icon="[FAIL]"
      [[ "${passed}" == "true" ]] && icon="[PASS]"
      echo "- ${icon} ${name}: ${detail}"
    fi
  done
  echo
  echo "## Candidate Environment"
  for entry in "${RESULTS[@]}"; do
    IFS="|" read -r passed name detail <<<"${entry}"
    if [[ "${name}" =~ ^(Stack probe|Proxy path proof|Release candidate validation|Backup drill|NetProxy CA drill)$ ]]; then
      icon="[FAIL]"
      [[ "${passed}" == "true" ]] && icon="[PASS]"
      echo "- ${icon} ${name}: ${detail}"
    fi
  done
  echo
  echo "## Operator Notes"
  echo "- Live stream topology: ${topology_detail}"
  if [[ "${backup_artifact}" != "not-run" ]]; then
    echo "- Backup artifact: \`${backup_artifact}\`"
  fi
  if [[ -f "${netproxy_report}" ]]; then
    echo "- NetProxy CA drill report: \`${netproxy_report}\`"
  fi
} >"${OUTPUT_PATH}"

echo "Production deployment validation summary written to ${OUTPUT_PATH}"
if [[ "${validation_result}" != "PASS" ]]; then
  exit 1
fi
