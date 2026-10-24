#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd "$script_dir/../.." && pwd)
venv=${ENTERPRISE_AGENT_VENV:-"$repo/.venv"}

if [[ ! -x "$venv/bin/python" ]]; then
  python3 -m venv "$venv"
fi
"$venv/bin/pip" install -r "$repo/enterprise-agent/requirements.txt"

export ENTERPRISE_AGENT_VENV="$venv"
export PUL_ADMIN_TOKEN_FILE="$repo/.local/enterprise-stack/pul/admin-token"
exec "$repo/enterprise-agent/start.sh"
