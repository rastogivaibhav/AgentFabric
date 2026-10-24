#!/usr/bin/env bash
set -euo pipefail

runtime=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$runtime/.." && pwd)
venv=${ENTERPRISE_AGENT_VENV:-"$repo/.venv"}
pid_file="$runtime/state/service.pid"
log_file="$runtime/state/service.log"

mkdir -p "$runtime/state"

ags_keys=$(docker inspect ags-sovereign-ags-control-plane-1 \
  --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | sed -n 's/^AGS_API_KEYS=//p')
export AGS_OPERATOR_KEY=${ags_keys%%,*}
govagn_secret=$(docker inspect govagn-collector-1 \
  --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | sed -n 's/^GV_JWT_SECRET=//p')
export GOVAGN_JWT_SECRET="$govagn_secret"
export SPACE_LOCAL_PASSWORD=${SPACE_LOCAL_PASSWORD:?Set SPACE_LOCAL_PASSWORD for the local SPACE account}
export PUL_ADMIN_TOKEN_FILE=${PUL_ADMIN_TOKEN_FILE:-"$repo/.local/enterprise-stack/pul/admin-token"}
export AGENTFABRIC_PROVIDER_DIR=${AGENTFABRIC_PROVIDER_DIR:-"$repo/integrations/space_provider"}

[[ -x "$venv/bin/uvicorn" ]] || { echo "Install enterprise-agent/requirements.txt into $venv first" >&2; exit 1; }
[[ -s "$PUL_ADMIN_TOKEN_FILE" ]] || { echo "PUL admin token is missing: $PUL_ADMIN_TOKEN_FILE" >&2; exit 1; }

if [[ -f "$pid_file" ]]; then
  old_pid=$(cat "$pid_file")
  if kill -0 "$old_pid" 2>/dev/null; then
    echo "Enterprise agent service already running (PID $old_pid)"
    exit 0
  fi
fi

cd "$runtime"
nohup "$venv/bin/uvicorn" app:app --host 127.0.0.1 --port 8090 \
  >"$log_file" 2>&1 < /dev/null &
pid=$!
echo "$pid" > "$pid_file"

for _ in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8090/health >/dev/null 2>&1; then
    echo "Enterprise agent service running at http://127.0.0.1:8090 (PID $pid)"
    exit 0
  fi
  sleep 1
done

tail -n 80 "$log_file" >&2 || true
exit 1
